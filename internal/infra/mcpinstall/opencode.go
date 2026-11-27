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
// It manages ~/.config/opencode/opencode.json → mcp.servers.flutree (V2) or
// mcp.flutree (legacy), whichever layout the file already uses.
type opencodeMerger struct {
	baseDir  string // injectable for tests
	lookPath func(string) (string, error)
	scope    Scope
	cwd      string
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
	path := o.configFile(base)

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

	// Write in the layout the file already uses; see openCodeLayoutOf.
	layout := openCodeLayoutOf(mcpMap)
	servers, err := openCodeServers(mcpMap, layout)
	if err != nil {
		return OutcomeError, fmt.Errorf("opencode: mcp.servers key: %w", err)
	}

	// Check if flutree entry already exists in either layout.
	_, inServers := servers[entryName]
	_, inLegacy := mcpMap[entryName]
	if (inServers || (layout == openCodeV2 && inLegacy)) && !force {
		return OutcomeAlreadyExists, nil
	}

	entryRaw, err := json.Marshal(openCodeEntryFor(layout, absCmd))
	if err != nil {
		return OutcomeError, fmt.Errorf("opencode: marshal entry: %w", err)
	}
	servers[entryName] = json.RawMessage(entryRaw)
	if layout == openCodeV2 {
		// Forced re-install: drop a stray legacy entry the V2 layout cannot read.
		delete(mcpMap, entryName)
		ser, err := json.Marshal(servers)
		if err != nil {
			return OutcomeError, fmt.Errorf("opencode: marshal mcp.servers: %w", err)
		}
		mcpMap["servers"] = json.RawMessage(ser)
	}

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

// openCodeLayout is where OpenCode reads server definitions inside "mcp".
type openCodeLayout int

const (
	// openCodeV2 nests servers under mcp.servers.<name>. The current docs
	// prescribe it: "give it a unique name under `mcp.servers`. V2 does not
	// place server names directly under `mcp`."
	// https://opencode.ai/docs/mcp-servers/ (examples use "$schema":
	// "https://opencode.ai/config.json"; there is no other version marker, and
	// https://opencode.ai/docs/config/ shows the same mcp -> servers nesting).
	openCodeV2 openCodeLayout = iota
	// openCodeLegacy puts servers directly under mcp.<name>, the layout older
	// OpenCode releases read.
	openCodeLegacy
)

// openCodeLayoutOf detects the layout an existing mcp object already uses: an
// object under "servers" that is not itself a server entry means V2; any other
// mcp.<name> object means legacy. Anything else (missing mcp, empty mcp, only
// scalar settings such as timeout) is a fresh config and gets V2. The docs
// (URLs above) give no version cutoff showing that current OpenCode releases
// cannot read mcp.servers, so new files follow the docs rather than the old
// layout.
func openCodeLayoutOf(mcpMap map[string]json.RawMessage) openCodeLayout {
	if raw, ok := mcpMap["servers"]; ok && !looksLikeServerEntry(raw) {
		var sub map[string]json.RawMessage
		if json.Unmarshal(raw, &sub) == nil {
			return openCodeV2
		}
	}
	for _, raw := range mcpMap {
		if looksLikeServerEntry(raw) {
			return openCodeLegacy
		}
	}
	return openCodeV2
}

// looksLikeServerEntry reports whether raw is a server definition (has a
// "type" or "command") rather than a container of servers.
func looksLikeServerEntry(raw json.RawMessage) bool {
	var e map[string]json.RawMessage
	if json.Unmarshal(raw, &e) != nil {
		return false
	}
	_, hasType := e["type"]
	_, hasCmd := e["command"]
	return hasType || hasCmd
}

// openCodeServers returns the map holding server entries for the layout.
func openCodeServers(mcpMap map[string]json.RawMessage, layout openCodeLayout) (map[string]json.RawMessage, error) {
	if layout == openCodeLegacy {
		return mcpMap, nil
	}
	return getOrCreateRawMap(mcpMap, "servers")
}

// openCodeEntryFor builds the entry for a layout. V2 has no "enabled" field
// (servers connect automatically; "disabled" turns one off), the legacy layout
// keeps the original shape.
func openCodeEntryFor(layout openCodeLayout, absCmd string) any {
	if layout == openCodeV2 {
		return openCodeEntryV2{Type: "local", Command: []string{absCmd, "mcp", "serve"}}
	}
	return openCodeEntry{Type: "local", Command: []string{absCmd, "mcp", "serve"}, Enabled: true}
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
	return o.configFile(base)
}

// configFile is ~/.config/opencode/opencode.json for user scope and
// <cwd>/opencode.json for project scope.
func (o *opencodeMerger) configFile(base string) string {
	if o.scope == ScopeProject {
		return filepath.Join(o.cwd, "opencode.json")
	}
	return filepath.Join(base, ".config", "opencode", "opencode.json")
}

func (o *opencodeMerger) WithScope(scope Scope, cwd string) (Client, bool) {
	cp := *o
	cp.scope, cp.cwd = scope, cwd
	return &cp, true
}

func (o *opencodeMerger) Preview(absCmd string) (string, error) {
	layout := openCodeV2
	if root, _, err := o.readStripped(); err == nil {
		if m, err := getOrCreateRawMap(root, "mcp"); err == nil {
			layout = openCodeLayoutOf(m)
		}
	}
	return previewJSON(openCodeEntryFor(layout, absCmd))
}

// readStripped loads the config with JSONC comments removed (read-only use).
func (o *opencodeMerger) readStripped() (map[string]json.RawMessage, []byte, error) {
	raw, err := os.ReadFile(o.ConfigPath())
	if errors.Is(err, fs.ErrNotExist) || (err == nil && len(raw) == 0) {
		return map[string]json.RawMessage{}, raw, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(jsonc.ToJSONInPlace(append([]byte(nil), raw...)), &root); err != nil {
		return nil, nil, fmt.Errorf("malformed JSON in %s: %w", filepath.Base(o.ConfigPath()), err)
	}
	return root, raw, nil
}

// findFlutree looks for the flutree entry in both layouts and returns the
// entry plus the mcp map and the servers map it was found in.
func findFlutree(mcpMap map[string]json.RawMessage) (entry json.RawMessage, servers map[string]json.RawMessage, found bool) {
	if raw, ok := mcpMap["servers"]; ok && !looksLikeServerEntry(raw) {
		var sub map[string]json.RawMessage
		if json.Unmarshal(raw, &sub) == nil {
			if e, ok := sub[entryName]; ok {
				return e, sub, true
			}
		}
	}
	if e, ok := mcpMap[entryName]; ok {
		return e, nil, true
	}
	return nil, nil, false
}

// Remove deletes only the flutree entry, from mcp.servers and/or mcp.<name>.
// A config containing JSONC comments is refused (rewriting would drop them);
// remove the entry by hand in that case.
func (o *opencodeMerger) Remove() (Outcome, error) {
	root, raw, err := o.readStripped()
	if err != nil {
		return OutcomeError, fmt.Errorf("opencode: %w", err)
	}
	mcpMap, err := getOrCreateRawMap(root, "mcp")
	if err != nil {
		return OutcomeError, fmt.Errorf("opencode: mcp key: %w", err)
	}
	if _, _, ok := findFlutree(mcpMap); !ok {
		return OutcomeNotFound, nil
	}
	if string(jsonc.ToJSONInPlace(append([]byte(nil), raw...))) != string(raw) {
		return OutcomeError, fmt.Errorf("opencode: config contains JSONC comments; remove the %q entry from %s by hand", entryName, o.ConfigPath())
	}
	if raw, ok := mcpMap["servers"]; ok && !looksLikeServerEntry(raw) {
		var sub map[string]json.RawMessage
		if json.Unmarshal(raw, &sub) == nil {
			if _, ok := sub[entryName]; ok {
				delete(sub, entryName)
				ser, err := json.Marshal(sub)
				if err != nil {
					return OutcomeError, fmt.Errorf("opencode: marshal mcp.servers: %w", err)
				}
				mcpMap["servers"] = ser
			}
		}
	}
	delete(mcpMap, entryName)
	ser, err := json.Marshal(mcpMap)
	if err != nil {
		return OutcomeError, fmt.Errorf("opencode: marshal mcp: %w", err)
	}
	root["mcp"] = ser
	if err := writeJSONMap(o.ConfigPath(), root); err != nil {
		return OutcomeError, fmt.Errorf("opencode: write config: %w", err)
	}
	return OutcomeRemoved, nil
}

func (o *opencodeMerger) EntryCommand() (string, bool, error) {
	root, _, err := o.readStripped()
	if err != nil {
		return "", false, fmt.Errorf("opencode: %w", err)
	}
	mcpMap, err := getOrCreateRawMap(root, "mcp")
	if err != nil {
		return "", false, err
	}
	entry, _, ok := findFlutree(mcpMap)
	if !ok {
		return "", false, nil
	}
	return commandOf(entry), true, nil
}

// openCodeEntryV2 is the local MCP server shape of the V2 layout.
type openCodeEntryV2 struct {
	Type    string   `json:"type"`
	Command []string `json:"command"`
}

// openCodeEntry is the JSON shape expected by OpenCode for a local MCP server.
type openCodeEntry struct {
	Type    string   `json:"type"`
	Command []string `json:"command"`
	Enabled bool     `json:"enabled"`
}
