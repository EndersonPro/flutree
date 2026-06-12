package mcpinstall

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
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
// It returns a non-nil error only on genuine I/O failure (e.g. permission denied
// resolving the home directory).
func (o *opencodeMerger) Detect() (bool, error) {
	if _, err := o.lookPath("opencode"); err == nil {
		return true, nil
	}
	base, err := o.resolveBase()
	if err != nil {
		return false, fmt.Errorf("opencode: resolve home dir: %w", err)
	}
	if _, err := os.Stat(filepath.Join(base, ".config", "opencode")); err == nil {
		return true, nil
	}
	return false, nil
}

// Merge writes the flutree entry to ~/.config/opencode/opencode.json non-destructively.
// JSONC handling: if the file contains comments and force is false, returns OutcomeError.
// If force is true, comments are stripped before merging (comments are not preserved).
// An empty (0-byte) file is treated as a fresh start.
func (o *opencodeMerger) Merge(absCmd string, force bool) (Outcome, error) {
	base, err := o.resolveBase()
	if err != nil {
		return OutcomeError, fmt.Errorf("opencode: resolve home dir: %w", err)
	}
	configDir := filepath.Join(base, ".config", "opencode")
	path := filepath.Join(configDir, "opencode.json")

	raw, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return OutcomeError, fmt.Errorf("opencode: read config: %w", err)
	}

	var root map[string]json.RawMessage

	if errors.Is(err, fs.ErrNotExist) || len(raw) == 0 {
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

// resolveBase returns the home directory for this merger.
// It returns an error when os.UserHomeDir fails, rather than silently falling
// back to "." (which would write config files into the process working directory).
func (o *opencodeMerger) resolveBase() (string, error) {
	if o.baseDir != "" {
		return o.baseDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return home, nil
}

// ConfigPath returns the absolute path of the config file this merger manages.
// It satisfies MergerWithConfigPath; empty string is returned if the home
// directory cannot be resolved.
func (o *opencodeMerger) ConfigPath() string {
	base, err := o.resolveBase()
	if err != nil {
		return ""
	}
	return filepath.Join(base, ".config", "opencode", "opencode.json")
}

// openCodeEntry is the JSON shape expected by OpenCode for a local MCP server.
type openCodeEntry struct {
	Type    string   `json:"type"`
	Command []string `json:"command"`
	Enabled bool     `json:"enabled"`
}
