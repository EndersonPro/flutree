package mcpinstall

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tidwall/jsonc"
)

// opencodeMerger implements Merger for the OpenCode AI client.
// It manages ~/.config/opencode/opencode.json → mcp.flutree key.
type opencodeMerger struct {
	baseDir  string // injectable for tests
	lookPath func(string) (string, error)
}

// NewOpenCodeMerger returns a Merger for the OpenCode client.
// baseDir is the home-directory root used in tests; pass "" for real home dir.
// When baseDir is non-empty (test mode), binary detection via PATH is disabled.
func NewOpenCodeMerger(baseDir string) Merger {
	lp := exec.LookPath
	if baseDir != "" {
		lp = func(string) (string, error) { return "", fmt.Errorf("not found") }
	}
	return &opencodeMerger{baseDir: baseDir, lookPath: lp}
}

func (o *opencodeMerger) Name() string { return "opencode" }

// Detect returns true when `opencode` binary is on PATH OR ~/.config/opencode/ exists.
func (o *opencodeMerger) Detect() bool {
	if _, err := o.lookPath("opencode"); err == nil {
		return true
	}
	base := o.resolveBase()
	if _, err := os.Stat(filepath.Join(base, ".config", "opencode")); err == nil {
		return true
	}
	return false
}

// Merge writes the flutree entry to ~/.config/opencode/opencode.json non-destructively.
// JSONC handling: if the file contains comments and force is false, returns OutcomeError.
// If force is true, comments are stripped before merging (comments are not preserved).
func (o *opencodeMerger) Merge(absCmd string, force bool) (Outcome, error) {
	configDir := filepath.Join(o.resolveBase(), ".config", "opencode")
	path := filepath.Join(configDir, "opencode.json")

	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return OutcomeError, fmt.Errorf("opencode: read config: %w", err)
	}

	var root map[string]json.RawMessage

	if os.IsNotExist(err) || len(raw) == 0 {
		// File missing or empty: start fresh.
		root = map[string]json.RawMessage{}
	} else {
		// Detect JSONC: if stripping produces different bytes, comments are present.
		stripped := jsonc.ToJSONInPlace(append([]byte(nil), raw...))
		hasComments := string(stripped) != string(raw)

		if hasComments && !force {
			return OutcomeError, fmt.Errorf(
				"opencode: config contains JSONC comments; use --force to strip comments and proceed",
			)
		}

		// Use stripped bytes for decoding (either comments removed via force, or no comments).
		if err := json.Unmarshal(stripped, &root); err != nil {
			return OutcomeError, fmt.Errorf("opencode: malformed JSON in %s: %w", filepath.Base(path), err)
		}
	}

	// Navigate or create the mcp sub-object.
	mcpMap, err := getOrCreateRawMap(root, "mcp")
	if err != nil {
		return OutcomeError, fmt.Errorf("opencode: mcp key: %w", err)
	}

	// Check if flutree entry already exists.
	if _, exists := mcpMap["flutree"]; exists && !force {
		return OutcomeAlreadyExists, nil
	}

	// Build the entry.
	entry := openCodeEntry{
		Type:    "local",
		Command: []string{absCmd, "mcp", "serve"},
		Enabled: true,
	}
	entryRaw, err := json.Marshal(entry)
	if err != nil {
		return OutcomeError, fmt.Errorf("opencode: marshal entry: %w", err)
	}
	mcpMap["flutree"] = json.RawMessage(entryRaw)

	// Persist mcp back into root.
	serialised, err := json.Marshal(mcpMap)
	if err != nil {
		return OutcomeError, fmt.Errorf("opencode: marshal mcp: %w", err)
	}
	root["mcp"] = json.RawMessage(serialised)

	if err := writeJSONMap(path, root); err != nil {
		return OutcomeError, fmt.Errorf("opencode: write config: %w", err)
	}
	return OutcomeConfigured, nil
}

func (o *opencodeMerger) resolveBase() string {
	if o.baseDir != "" {
		return o.baseDir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

// openCodeEntry is the JSON shape expected by OpenCode for a local MCP server.
type openCodeEntry struct {
	Type    string   `json:"type"`
	Command []string `json:"command"`
	Enabled bool     `json:"enabled"`
}
