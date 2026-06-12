package mcpinstall

import (
	"errors"
	"fmt"
	"io/fs"
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
// It returns a non-nil error only on genuine I/O failure (e.g. permission denied
// resolving the home directory).
func (c *codexMerger) Detect() (bool, error) {
	if _, err := c.lookPath("codex"); err == nil {
		return true, nil
	}
	base, err := c.resolveBase()
	if err != nil {
		return false, fmt.Errorf("codex: resolve home dir: %w", err)
	}
	if _, err := os.Stat(filepath.Join(base, ".codex")); err == nil {
		return true, nil
	}
	return false, nil
}

// Merge writes the flutree entry to ~/.codex/config.toml non-destructively.
// TOML comment handling (consistent with opencode JSONC policy): if the raw
// TOML bytes contain comments and force is false, returns OutcomeError to
// prevent silent comment loss. With force, comments are stripped on the
// marshal round-trip (go-toml/v2 map round-trip behaviour).
// It returns OutcomeConfigured on success, OutcomeAlreadyExists if the entry
// exists (and force is false), or OutcomeError on failure.
func (c *codexMerger) Merge(absCmd string, force bool) (Outcome, error) {
	base, err := c.resolveBase()
	if err != nil {
		return OutcomeError, fmt.Errorf("codex: resolve home dir: %w", err)
	}
	codexDir := filepath.Join(base, ".codex")
	path := filepath.Join(codexDir, "config.toml")

	// Load or initialize as a generic map to preserve all unrelated keys.
	raw, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return OutcomeError, fmt.Errorf("codex: read config: %w", err)
	}

	var root map[string]any

	if errors.Is(err, fs.ErrNotExist) || len(raw) == 0 {
		// File missing or empty: start fresh.
		root = map[string]any{}
	} else {
		// Detect TOML comments: conservatively scan for '#' characters.
		// This is acceptable because '#' inside TOML strings is rare in
		// machine-generated config files; a false positive with force=false
		// is safe (user can re-run with --force).
		if tomlHasComments(raw) && !force {
			return OutcomeError, fmt.Errorf(
				"codex: config contains TOML comments; use --force to strip comments and proceed",
			)
		}

		if err := toml.Unmarshal(raw, &root); err != nil {
			return OutcomeError, fmt.Errorf("codex: read config: %w",
				fmt.Errorf("malformed TOML in %s: %w", filepath.Base(path), err))
		}
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

// resolveBase returns the home directory for this merger.
// It returns an error when os.UserHomeDir fails, rather than silently falling
// back to "." (which would write config files into the process working directory).
func (c *codexMerger) resolveBase() (string, error) {
	if c.baseDir != "" {
		return c.baseDir, nil
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
func (c *codexMerger) ConfigPath() string {
	base, err := c.resolveBase()
	if err != nil {
		return ""
	}
	return filepath.Join(base, ".codex", "config.toml")
}

// tomlHasComments returns true when raw TOML bytes contain a '#' character
// that is outside a quoted string.  The check is conservative: it returns
// true on any '#' that is not preceded on the same line by an odd number of
// unescaped double-quote characters, which covers the common machine-generated
// config format without a full TOML parser.
func tomlHasComments(data []byte) bool {
	inString := false
	for i := 0; i < len(data); i++ {
		b := data[i]
		switch {
		case b == '"' && (i == 0 || data[i-1] != '\\'):
			inString = !inString
		case b == '#' && !inString:
			return true
		case b == '\n':
			inString = false // reset on newline (TOML strings don't span lines unescaped)
		}
	}
	return false
}

// loadTOMLMap reads a TOML file as map[string]any.
// Returns an empty map when the file does not exist or is empty (0 bytes).
func loadTOMLMap(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	// Empty file: treat as fresh start.
	if len(data) == 0 {
		return map[string]any{}, nil
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
	// Preserve existing file permissions; default to 0o600 for new files.
	perm := fs.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}
	return atomicWrite(path, data, perm)
}
