package mcpinstall

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// codexMerger implements Merger for the Codex AI client.
// It manages ~/.codex/config.toml → [mcp_servers.flutree] table.
type codexMerger struct {
	baseDir  string // injectable for tests
	lookPath func(string) (string, error)
}

// NewCodexMerger returns a Merger for the Codex client.
// baseDir is the home-directory root used in tests; pass "" for real home dir.
// When baseDir is non-empty (test mode), binary detection via PATH is disabled.
func NewCodexMerger(baseDir string) Merger {
	lp := exec.LookPath
	if baseDir != "" {
		lp = func(string) (string, error) { return "", fmt.Errorf("not found") }
	}
	return &codexMerger{baseDir: baseDir, lookPath: lp}
}

func (c *codexMerger) Name() string { return "codex" }

// Detect returns true when `codex` binary is on PATH OR ~/.codex/ directory exists.
func (c *codexMerger) Detect() bool {
	if _, err := c.lookPath("codex"); err == nil {
		return true
	}
	base := c.resolveBase()
	if _, err := os.Stat(filepath.Join(base, ".codex")); err == nil {
		return true
	}
	return false
}

// Merge writes the flutree entry to ~/.codex/config.toml non-destructively.
// It returns OutcomeConfigured on success, OutcomeAlreadyExists if the entry
// exists (and force is false), or OutcomeError on failure.
func (c *codexMerger) Merge(absCmd string, force bool) (Outcome, error) {
	codexDir := filepath.Join(c.resolveBase(), ".codex")
	path := filepath.Join(codexDir, "config.toml")

	// Load or initialize as a generic map to preserve all unrelated keys.
	root, err := loadTOMLMap(path)
	if err != nil {
		return OutcomeError, fmt.Errorf("codex: read config: %w", err)
	}

	// Navigate or create mcp_servers table.
	mcpServers, err := getOrCreateAnyMap(root, "mcp_servers")
	if err != nil {
		return OutcomeError, fmt.Errorf("codex: mcp_servers key: %w", err)
	}

	// Check if flutree entry already exists.
	if _, exists := mcpServers["flutree"]; exists && !force {
		return OutcomeAlreadyExists, nil
	}

	// Build the entry.
	mcpServers["flutree"] = map[string]any{
		"command": absCmd,
		"args":    []string{"mcp", "serve"},
	}
	root["mcp_servers"] = mcpServers

	if err := writeTOMLMap(path, root); err != nil {
		return OutcomeError, fmt.Errorf("codex: write config: %w", err)
	}
	return OutcomeConfigured, nil
}

func (c *codexMerger) resolveBase() string {
	if c.baseDir != "" {
		return c.baseDir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

// loadTOMLMap reads a TOML file as map[string]any.
// Returns an empty map when the file does not exist.
func loadTOMLMap(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := toml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("malformed TOML in %s: %w", filepath.Base(path), err)
	}
	return m, nil
}

// getOrCreateAnyMap returns the sub-map stored under key in root, creating an
// empty map if the key is absent. Returns an error if key is not a map.
func getOrCreateAnyMap(root map[string]any, key string) (map[string]any, error) {
	val, exists := root[key]
	if !exists {
		return map[string]any{}, nil
	}
	sub, ok := val.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("key %q is not a TOML table (got %T)", key, val)
	}
	return sub, nil
}

// writeTOMLMap atomically writes a map[string]any as TOML.
func writeTOMLMap(path string, m map[string]any) error {
	data, err := toml.Marshal(m)
	if err != nil {
		return err
	}
	return atomicWrite(path, data, 0o644)
}
