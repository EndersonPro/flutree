package mcpinstall_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/EndersonPro/flutree/internal/infra/mcpinstall"
)

func TestClaudeMerger_Name(t *testing.T) {
	m := mcpinstall.NewClaudeMerger("")
	if m.Name() != "claude-code" {
		t.Fatalf("want %q got %q", "claude-code", m.Name())
	}
}

func TestClaudeMerger_Detect_ConfigExists(t *testing.T) {
	dir := t.TempDir()
	// Create ~/.claude.json
	if err := os.WriteFile(filepath.Join(dir, ".claude.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := mcpinstall.NewClaudeMerger(dir)
	if !m.Detect() {
		t.Fatal("expected Detect() == true when .claude.json exists")
	}
}

func TestClaudeMerger_Detect_DirExists(t *testing.T) {
	dir := t.TempDir()
	// Create ~/.claude/ directory
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := mcpinstall.NewClaudeMerger(dir)
	if !m.Detect() {
		t.Fatal("expected Detect() == true when .claude/ dir exists")
	}
}

func TestClaudeMerger_Detect_NeitherPresent(t *testing.T) {
	dir := t.TempDir()
	m := mcpinstall.NewClaudeMerger(dir)
	if m.Detect() {
		t.Fatal("expected Detect() == false when neither .claude.json nor .claude/ exists")
	}
}

func TestClaudeMerger_Merge_AbsentEntry_Creates(t *testing.T) {
	dir := t.TempDir()
	existing := map[string]any{
		"projects": []string{"proj1"},
		"auth":     map[string]any{"token": "abc"},
		"mcpServers": map[string]any{
			"other": map[string]any{"type": "stdio", "command": "other-bin"},
		},
	}
	writeJSON(t, filepath.Join(dir, ".claude.json"), existing)

	m := mcpinstall.NewClaudeMerger(dir)
	outcome, err := m.Merge("/usr/local/bin/flutree", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome != mcpinstall.OutcomeConfigured {
		t.Fatalf("want %q got %q", mcpinstall.OutcomeConfigured, outcome)
	}

	// Read back and validate shape
	data := readJSON(t, filepath.Join(dir, ".claude.json"))
	mcpServers := data["mcpServers"].(map[string]any)
	flutree := mcpServers["flutree"].(map[string]any)
	if flutree["type"] != "stdio" {
		t.Errorf("want type=stdio got %v", flutree["type"])
	}
	if flutree["command"] != "/usr/local/bin/flutree" {
		t.Errorf("want command=/usr/local/bin/flutree got %v", flutree["command"])
	}
	args := flutree["args"].([]any)
	if len(args) != 2 || args[0] != "mcp" || args[1] != "serve" {
		t.Errorf("want args=[mcp serve] got %v", args)
	}

	// Existing unrelated keys must be preserved
	if _, ok := data["projects"]; !ok {
		t.Error("projects key was removed")
	}
	if _, ok := data["auth"]; !ok {
		t.Error("auth key was removed")
	}
	// Other mcpServers entries preserved
	if _, ok := mcpServers["other"]; !ok {
		t.Error("other mcpServers entry was removed")
	}
}

func TestClaudeMerger_Merge_EntryPresent_NoForce_Skips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")
	original := map[string]any{
		"mcpServers": map[string]any{
			"flutree": map[string]any{"type": "stdio", "command": "/old/flutree", "args": []any{"mcp", "serve"}},
		},
	}
	writeJSON(t, path, original)
	before := readFile(t, path)

	m := mcpinstall.NewClaudeMerger(dir)
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

func TestClaudeMerger_Merge_EntryPresent_Force_Overwrites(t *testing.T) {
	dir := t.TempDir()
	existing := map[string]any{
		"mcpServers": map[string]any{
			"flutree": map[string]any{"type": "stdio", "command": "/old/flutree", "args": []any{"mcp", "serve"}},
		},
	}
	writeJSON(t, filepath.Join(dir, ".claude.json"), existing)

	m := mcpinstall.NewClaudeMerger(dir)
	outcome, err := m.Merge("/new/flutree", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome != mcpinstall.OutcomeConfigured {
		t.Fatalf("want %q got %q", mcpinstall.OutcomeConfigured, outcome)
	}

	data := readJSON(t, filepath.Join(dir, ".claude.json"))
	mcpServers := data["mcpServers"].(map[string]any)
	flutree := mcpServers["flutree"].(map[string]any)
	if flutree["command"] != "/new/flutree" {
		t.Errorf("expected command to be overwritten to /new/flutree got %v", flutree["command"])
	}
}

func TestClaudeMerger_Merge_FileMissing_Creates(t *testing.T) {
	dir := t.TempDir()
	// No .claude.json present

	m := mcpinstall.NewClaudeMerger(dir)
	outcome, err := m.Merge("/usr/local/bin/flutree", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome != mcpinstall.OutcomeConfigured {
		t.Fatalf("want %q got %q", mcpinstall.OutcomeConfigured, outcome)
	}

	data := readJSON(t, filepath.Join(dir, ".claude.json"))
	mcpServers := data["mcpServers"].(map[string]any)
	flutree := mcpServers["flutree"].(map[string]any)
	if flutree["type"] != "stdio" {
		t.Errorf("want type=stdio got %v", flutree["type"])
	}
}

func TestClaudeMerger_Merge_MalformedJSON_Error(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".claude.json"), []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := mcpinstall.NewClaudeMerger(dir)
	outcome, err := m.Merge("/usr/local/bin/flutree", false)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if outcome != mcpinstall.OutcomeError {
		t.Fatalf("want %q got %q", mcpinstall.OutcomeError, outcome)
	}
}

// --- helpers ---

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("readJSON: %v", err)
	}
	return m
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
