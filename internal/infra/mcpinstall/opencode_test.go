package mcpinstall_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/EndersonPro/flutree/internal/infra/mcpinstall"
)

func TestOpenCodeMerger_Name(t *testing.T) {
	m := mcpinstall.NewOpenCodeMerger("")
	if m.Name() != "opencode" {
		t.Fatalf("want %q got %q", "opencode", m.Name())
	}
}

func TestOpenCodeMerger_Detect_ConfigDirExists(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".config", "opencode"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := mcpinstall.NewOpenCodeMerger(dir)
	if !m.Detect() {
		t.Fatal("expected Detect() == true when .config/opencode/ exists")
	}
}

func TestOpenCodeMerger_Detect_NeitherPresent(t *testing.T) {
	dir := t.TempDir()
	m := mcpinstall.NewOpenCodeMerger(dir)
	if m.Detect() {
		t.Fatal("expected Detect() == false when .config/opencode/ absent")
	}
}

func TestOpenCodeMerger_Merge_AbsentEntry_Creates(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := map[string]any{
		"theme": "dark",
		"mcp": map[string]any{
			"other": map[string]any{"type": "local", "command": []any{"/bin/other"}, "enabled": true},
		},
	}
	writeJSON(t, filepath.Join(configDir, "opencode.json"), existing)

	m := mcpinstall.NewOpenCodeMerger(dir)
	outcome, err := m.Merge("/usr/local/bin/flutree", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome != mcpinstall.OutcomeConfigured {
		t.Fatalf("want %q got %q", mcpinstall.OutcomeConfigured, outcome)
	}

	data := readJSON(t, filepath.Join(configDir, "opencode.json"))
	mcp := data["mcp"].(map[string]any)
	flutree := mcp["flutree"].(map[string]any)

	if flutree["type"] != "local" {
		t.Errorf("want type=local got %v", flutree["type"])
	}
	cmds := flutree["command"].([]any)
	if len(cmds) != 3 || cmds[0] != "/usr/local/bin/flutree" || cmds[1] != "mcp" || cmds[2] != "serve" {
		t.Errorf("want command=[/usr/local/bin/flutree mcp serve] got %v", cmds)
	}
	if flutree["enabled"] != true {
		t.Errorf("want enabled=true got %v", flutree["enabled"])
	}

	// Unrelated keys preserved
	if data["theme"] != "dark" {
		t.Error("theme key was removed")
	}
	if _, ok := mcp["other"]; !ok {
		t.Error("other mcp entry was removed")
	}
}

func TestOpenCodeMerger_Merge_EntryPresent_NoForce_Skips(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(configDir, "opencode.json")
	existing := map[string]any{
		"mcp": map[string]any{
			"flutree": map[string]any{"type": "local", "command": []any{"/old/flutree", "mcp", "serve"}, "enabled": true},
		},
	}
	writeJSON(t, path, existing)
	before := readFile(t, path)

	m := mcpinstall.NewOpenCodeMerger(dir)
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

func TestOpenCodeMerger_Merge_EntryPresent_Force_Overwrites(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := map[string]any{
		"mcp": map[string]any{
			"flutree": map[string]any{"type": "local", "command": []any{"/old/flutree", "mcp", "serve"}, "enabled": true},
		},
	}
	writeJSON(t, filepath.Join(configDir, "opencode.json"), existing)

	m := mcpinstall.NewOpenCodeMerger(dir)
	outcome, err := m.Merge("/new/flutree", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome != mcpinstall.OutcomeConfigured {
		t.Fatalf("want %q got %q", mcpinstall.OutcomeConfigured, outcome)
	}

	data := readJSON(t, filepath.Join(configDir, "opencode.json"))
	mcp := data["mcp"].(map[string]any)
	flutree := mcp["flutree"].(map[string]any)
	cmds := flutree["command"].([]any)
	if cmds[0] != "/new/flutree" {
		t.Errorf("expected command to be overwritten to /new/flutree got %v", cmds[0])
	}
}

func TestOpenCodeMerger_Merge_FileMissing_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	// No .config/opencode/ dir at all

	m := mcpinstall.NewOpenCodeMerger(dir)
	outcome, err := m.Merge("/usr/local/bin/flutree", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome != mcpinstall.OutcomeConfigured {
		t.Fatalf("want %q got %q", mcpinstall.OutcomeConfigured, outcome)
	}

	configPath := filepath.Join(dir, ".config", "opencode", "opencode.json")
	data := readJSON(t, configPath)
	mcp := data["mcp"].(map[string]any)
	if _, ok := mcp["flutree"]; !ok {
		t.Error("flutree entry was not created")
	}
}

func TestOpenCodeMerger_Merge_MalformedJSON_Error(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "opencode.json"), []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := mcpinstall.NewOpenCodeMerger(dir)
	outcome, err := m.Merge("/usr/local/bin/flutree", false)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if outcome != mcpinstall.OutcomeError {
		t.Fatalf("want %q got %q", mcpinstall.OutcomeError, outcome)
	}
}

func TestOpenCodeMerger_Merge_JSONC_NoForce_Error(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	jsoncContent := `{
  // this is a comment
  "theme": "dark"
}`
	if err := os.WriteFile(filepath.Join(configDir, "opencode.json"), []byte(jsoncContent), 0o644); err != nil {
		t.Fatal(err)
	}
	before := jsoncContent

	m := mcpinstall.NewOpenCodeMerger(dir)
	outcome, err := m.Merge("/usr/local/bin/flutree", false)
	if err == nil {
		t.Fatal("expected error for JSONC without --force, got nil")
	}
	if outcome != mcpinstall.OutcomeError {
		t.Fatalf("want %q got %q", mcpinstall.OutcomeError, outcome)
	}

	after := readFile(t, filepath.Join(configDir, "opencode.json"))
	if after != before {
		t.Error("file was modified but should not have been")
	}
}

func TestOpenCodeMerger_Merge_JSONC_Force_StripsAndWrites(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	jsoncContent := `{
  // theme setting
  "theme": "dark"
}`
	if err := os.WriteFile(filepath.Join(configDir, "opencode.json"), []byte(jsoncContent), 0o644); err != nil {
		t.Fatal(err)
	}

	m := mcpinstall.NewOpenCodeMerger(dir)
	outcome, err := m.Merge("/usr/local/bin/flutree", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome != mcpinstall.OutcomeConfigured {
		t.Fatalf("want %q got %q", mcpinstall.OutcomeConfigured, outcome)
	}

	data := readJSON(t, filepath.Join(configDir, "opencode.json"))
	if data["theme"] != "dark" {
		t.Error("theme key was lost after JSONC strip")
	}
	mcp := data["mcp"].(map[string]any)
	if _, ok := mcp["flutree"]; !ok {
		t.Error("flutree entry was not created after JSONC strip")
	}
}
