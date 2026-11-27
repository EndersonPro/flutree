package mcpinstall

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJSONClientsScopeAndRemove(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	c, ok := NewCursorMerger(home).WithScope(ScopeProject, cwd)
	if !ok {
		t.Fatal("cursor must support project scope")
	}
	if want := filepath.Join(cwd, ".cursor", "mcp.json"); c.ConfigPath() != want {
		t.Fatalf("ConfigPath = %s, want %s", c.ConfigPath(), want)
	}
	if _, ok := NewClaudeDesktopMerger(home).WithScope(ScopeProject, cwd); ok {
		t.Fatal("claude-desktop must not support project scope")
	}
	if _, ok := NewWindsurfMerger(home).WithScope(ScopeProject, cwd); ok {
		t.Fatal("windsurf must not support project scope")
	}
	if err := os.MkdirAll(filepath.Dir(c.ConfigPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c.ConfigPath(), []byte(`{"mcpServers":{"other":{"command":"x"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := c.Merge("/bin/flutree", false); err != nil || out != OutcomeConfigured {
		t.Fatalf("Merge = %v, %v", out, err)
	}
	if cmd, found, _ := c.EntryCommand(); !found || cmd != "/bin/flutree" {
		t.Fatalf("EntryCommand = %q, %v", cmd, found)
	}
	if out, err := c.Remove(); err != nil || out != OutcomeRemoved {
		t.Fatalf("Remove = %v, %v", out, err)
	}
	if out, _ := c.Remove(); out != OutcomeNotFound {
		t.Fatalf("second Remove = %v, want not_found", out)
	}
	b, _ := os.ReadFile(c.ConfigPath())
	if !strings.Contains(string(b), "other") || strings.Contains(string(b), "flutree") {
		t.Fatalf("remove must keep only other servers: %s", b)
	}
}

func TestInstallerDryRunWritesNothing(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".gemini"), 0o755); err != nil {
		t.Fatal(err)
	}
	inst := NewInstaller([]Merger{NewGeminiMerger(home)}, ExecResolver("/bin/flutree"))
	res, err := inst.Install(Options{DryRun: true, Scope: ScopeUser})
	if err != nil || len(res) != 1 || res[0].Status != OutcomeConfigured || !res[0].DryRun || !strings.Contains(res[0].Entry, "/bin/flutree") {
		t.Fatalf("unexpected dry-run result: %+v, %v", res, err)
	}
	if _, err := os.Stat(res[0].ConfigPath); err == nil {
		t.Fatal("dry-run wrote the config")
	}
}
