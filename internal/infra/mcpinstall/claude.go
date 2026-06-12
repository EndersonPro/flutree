package mcpinstall

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// claudeMerger implements Merger for the Claude Code AI client.
// It manages ~/.claude.json → mcpServers.flutree key.
type claudeMerger struct {
	baseDir  string // injectable for tests; real callers pass os.UserHomeDir()
	lookPath func(string) (string, error)
}

// NewClaudeMerger returns a Merger for the Claude Code client.
// baseDir is the home-directory root used in tests; pass "" for the real home dir
// (the real home dir is resolved lazily in Detect/Merge).
// When baseDir is non-empty (test mode), binary detection via PATH is disabled
// so tests don't depend on what is installed on the host machine.
func NewClaudeMerger(baseDir string) Merger {
	lp := exec.LookPath
	if baseDir != "" {
		// In test mode, never find the binary via PATH so Detect is deterministic.
		lp = func(string) (string, error) { return "", fmt.Errorf("not found") }
	}
	return &claudeMerger{baseDir: baseDir, lookPath: lp}
}

func (c *claudeMerger) Name() string { return "claude-code" }

// Detect returns true when:
//   - `claude` binary is on PATH, OR
//   - ~/.claude.json exists, OR
//   - ~/.claude/ directory exists.
func (c *claudeMerger) Detect() bool {
	if _, err := c.lookPath("claude"); err == nil {
		return true
	}
	base := c.resolveBase()
	if _, err := os.Stat(filepath.Join(base, ".claude.json")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(base, ".claude")); err == nil {
		return true
	}
	return false
}

// Merge writes the flutree entry to ~/.claude.json non-destructively.
// It returns OutcomeConfigured on success, OutcomeAlreadyExists if the
// entry already exists (and force is false), or OutcomeError on failure.
func (c *claudeMerger) Merge(absCmd string, force bool) (Outcome, error) {
	path := filepath.Join(c.resolveBase(), ".claude.json")

	// Load or initialize config as a map of raw JSON values so unrelated keys
	// are preserved byte-for-byte.
	root, err := loadJSONMap(path)
	if err != nil {
		return OutcomeError, fmt.Errorf("claude-code: read config: %w", err)
	}

	// Navigate or create the mcpServers sub-object.
	mcpServers, err := getOrCreateRawMap(root, "mcpServers")
	if err != nil {
		return OutcomeError, fmt.Errorf("claude-code: mcpServers key: %w", err)
	}

	// Check if flutree entry already exists.
	if _, exists := mcpServers["flutree"]; exists && !force {
		return OutcomeAlreadyExists, nil
	}

	// Build the entry.
	entry := claudeEntry{
		Type:    "stdio",
		Command: absCmd,
		Args:    []string{"mcp", "serve"},
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return OutcomeError, fmt.Errorf("claude-code: marshal entry: %w", err)
	}
	mcpServers["flutree"] = json.RawMessage(raw)

	// Persist mcpServers back into root.
	serialised, err := json.Marshal(mcpServers)
	if err != nil {
		return OutcomeError, fmt.Errorf("claude-code: marshal mcpServers: %w", err)
	}
	root["mcpServers"] = json.RawMessage(serialised)

	if err := writeJSONMap(path, root); err != nil {
		return OutcomeError, fmt.Errorf("claude-code: write config: %w", err)
	}
	return OutcomeConfigured, nil
}

func (c *claudeMerger) resolveBase() string {
	if c.baseDir != "" {
		return c.baseDir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

// claudeEntry is the JSON shape expected by Claude Code for a stdio MCP server.
type claudeEntry struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}
