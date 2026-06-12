package mcpinstall_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"

	"github.com/EndersonPro/flutree/internal/infra/mcpinstall"
)

func TestCodexMerger_Name(t *testing.T) {
	m := mcpinstall.NewCodexMerger("")
	if m.Name() != "codex" {
		t.Fatalf("want %q got %q", "codex", m.Name())
	}
}

func TestCodexMerger_Detect_DirExists(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := mcpinstall.NewCodexMerger(dir)
	if !m.Detect() {
		t.Fatal("expected Detect() == true when .codex/ exists")
	}
}

func TestCodexMerger_Detect_NeitherPresent(t *testing.T) {
	dir := t.TempDir()
	m := mcpinstall.NewCodexMerger(dir)
	if m.Detect() {
		t.Fatal("expected Detect() == false when .codex/ absent")
	}
}

func TestCodexMerger_Merge_AbsentEntry_Creates(t *testing.T) {
	dir := t.TempDir()
	codexDir := filepath.Join(dir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Existing TOML with unrelated keys
	existing := map[string]any{
		"model": "gpt-4o",
		"mcp_servers": map[string]any{
			"other": map[string]any{"command": "/bin/other", "args": []any{"serve"}},
		},
	}
	writeTOML(t, filepath.Join(codexDir, "config.toml"), existing)

	m := mcpinstall.NewCodexMerger(dir)
	outcome, err := m.Merge("/usr/local/bin/flutree", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome != mcpinstall.OutcomeConfigured {
		t.Fatalf("want %q got %q", mcpinstall.OutcomeConfigured, outcome)
	}

	data := readTOML(t, filepath.Join(codexDir, "config.toml"))
	mcpServers := data["mcp_servers"].(map[string]any)
	flutree := mcpServers["flutree"].(map[string]any)

	if flutree["command"] != "/usr/local/bin/flutree" {
		t.Errorf("want command=/usr/local/bin/flutree got %v", flutree["command"])
	}

	// Args should be ["mcp", "serve"]
	args, ok := flutree["args"]
	if !ok {
		t.Fatal("args key missing from flutree entry")
	}
	switch v := args.(type) {
	case []any:
		if len(v) != 2 || v[0] != "mcp" || v[1] != "serve" {
			t.Errorf("want args=[mcp serve] got %v", v)
		}
	case []string:
		if len(v) != 2 || v[0] != "mcp" || v[1] != "serve" {
			t.Errorf("want args=[mcp serve] got %v", v)
		}
	default:
		t.Errorf("unexpected args type %T: %v", args, args)
	}

	// Unrelated keys preserved
	if data["model"] != "gpt-4o" {
		t.Error("model key was removed")
	}
	if _, ok := mcpServers["other"]; !ok {
		t.Error("other mcp_servers entry was removed")
	}
}

func TestCodexMerger_Merge_EntryPresent_NoForce_Skips(t *testing.T) {
	dir := t.TempDir()
	codexDir := filepath.Join(dir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(codexDir, "config.toml")
	existing := map[string]any{
		"mcp_servers": map[string]any{
			"flutree": map[string]any{"command": "/old/flutree", "args": []any{"mcp", "serve"}},
		},
	}
	writeTOML(t, path, existing)
	before := readFile(t, path)

	m := mcpinstall.NewCodexMerger(dir)
	outcome, err := m.Merge("/new/flutree", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome != mcpinstall.OutcomeAlreadyExists {
		t.Fatalf("want %q got %q", mcpinstall.OutcomeAlreadyExists, outcome)
	}
	after := readFile(t, path)
	if before != after {
		t.Error("file was modified but should not have been")
	}
}

func TestCodexMerger_Merge_EntryPresent_Force_Overwrites(t *testing.T) {
	dir := t.TempDir()
	codexDir := filepath.Join(dir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := map[string]any{
		"mcp_servers": map[string]any{
			"flutree": map[string]any{"command": "/old/flutree", "args": []any{"mcp", "serve"}},
		},
	}
	writeTOML(t, filepath.Join(codexDir, "config.toml"), existing)

	m := mcpinstall.NewCodexMerger(dir)
	outcome, err := m.Merge("/new/flutree", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome != mcpinstall.OutcomeConfigured {
		t.Fatalf("want %q got %q", mcpinstall.OutcomeConfigured, outcome)
	}

	data := readTOML(t, filepath.Join(codexDir, "config.toml"))
	mcpServers := data["mcp_servers"].(map[string]any)
	flutree := mcpServers["flutree"].(map[string]any)
	if flutree["command"] != "/new/flutree" {
		t.Errorf("expected command to be overwritten to /new/flutree got %v", flutree["command"])
	}
}

func TestCodexMerger_Merge_FileMissing_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	// No .codex/ dir at all

	m := mcpinstall.NewCodexMerger(dir)
	outcome, err := m.Merge("/usr/local/bin/flutree", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome != mcpinstall.OutcomeConfigured {
		t.Fatalf("want %q got %q", mcpinstall.OutcomeConfigured, outcome)
	}

	configPath := filepath.Join(dir, ".codex", "config.toml")
	data := readTOML(t, configPath)
	mcpServers := data["mcp_servers"].(map[string]any)
	if _, ok := mcpServers["flutree"]; !ok {
		t.Error("flutree entry was not created")
	}
}

func TestCodexMerger_Merge_MalformedTOML_Error(t *testing.T) {
	dir := t.TempDir()
	codexDir := filepath.Join(dir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte("[not valid\ntoml = {"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := mcpinstall.NewCodexMerger(dir)
	outcome, err := m.Merge("/usr/local/bin/flutree", false)
	if err == nil {
		t.Fatal("expected error for malformed TOML, got nil")
	}
	if outcome != mcpinstall.OutcomeError {
		t.Fatalf("want %q got %q", mcpinstall.OutcomeError, outcome)
	}
}

// --- helpers ---

func writeTOML(t *testing.T, path string, v any) {
	t.Helper()
	data, err := toml.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTOML(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := toml.Unmarshal(data, &m); err != nil {
		t.Fatalf("readTOML: %v", err)
	}
	return m
}
