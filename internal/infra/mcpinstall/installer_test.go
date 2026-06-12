package mcpinstall_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/EndersonPro/flutree/internal/infra/mcpinstall"
)

// fakeMerger is a controllable Merger for orchestrator tests.
type fakeMerger struct {
	name        string
	detects     bool
	detectErr   error
	outcome     mcpinstall.Outcome
	mergeErr    error
	mergeForce  bool
	mergeCalled bool
	mergeCmd    string
}

func (f *fakeMerger) Name() string { return f.name }
func (f *fakeMerger) Detect() (bool, error) {
	return f.detects, f.detectErr
}
func (f *fakeMerger) Merge(absCmd string, force bool) (mcpinstall.Outcome, error) {
	f.mergeCalled = true
	f.mergeCmd = absCmd
	f.mergeForce = force
	return f.outcome, f.mergeErr
}

func TestInstaller_Run_AllDetected_AllConfigured(t *testing.T) {
	fakes := []*fakeMerger{
		{name: "claude-code", detects: true, outcome: mcpinstall.OutcomeConfigured},
		{name: "opencode", detects: true, outcome: mcpinstall.OutcomeConfigured},
		{name: "codex", detects: true, outcome: mcpinstall.OutcomeConfigured},
	}
	mergers := toMergers(fakes)

	installer := mcpinstall.NewInstaller(mergers, mcpinstall.ExecResolver("/abs/flutree"))
	results, err := installer.Run([]string{}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("want 3 results got %d", len(results))
	}
	for _, r := range results {
		if r.Status != mcpinstall.OutcomeConfigured {
			t.Errorf("client %q: want %q got %q", r.Client, mcpinstall.OutcomeConfigured, r.Status)
		}
	}
}

func TestInstaller_Run_NoneDetected_AllNotInstalled(t *testing.T) {
	fakes := []*fakeMerger{
		{name: "claude-code", detects: false},
		{name: "opencode", detects: false},
		{name: "codex", detects: false},
	}

	installer := mcpinstall.NewInstaller(toMergers(fakes), mcpinstall.ExecResolver("/abs/flutree"))
	results, err := installer.Run([]string{}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("want 3 results got %d", len(results))
	}
	for _, r := range results {
		if r.Status != mcpinstall.OutcomeNotInstalled {
			t.Errorf("client %q: want %q got %q", r.Client, mcpinstall.OutcomeNotInstalled, r.Status)
		}
		if r.Message == "" {
			t.Errorf("client %q: message should not be empty for not_installed", r.Client)
		}
	}
	// Merge should not have been called on any
	for _, f := range fakes {
		if f.mergeCalled {
			t.Errorf("Merge() should not be called when client not detected")
		}
	}
}

func TestInstaller_Run_ClientFilter_OnlyTargeted(t *testing.T) {
	fakes := []*fakeMerger{
		{name: "claude-code", detects: true, outcome: mcpinstall.OutcomeConfigured},
		{name: "opencode", detects: true, outcome: mcpinstall.OutcomeConfigured},
		{name: "codex", detects: true, outcome: mcpinstall.OutcomeConfigured},
	}

	installer := mcpinstall.NewInstaller(toMergers(fakes), mcpinstall.ExecResolver("/abs/flutree"))
	results, err := installer.Run([]string{"claude-code"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result got %d", len(results))
	}
	if results[0].Client != "claude-code" {
		t.Errorf("want claude-code got %q", results[0].Client)
	}
	// opencode and codex should not have been called
	if fakes[1].mergeCalled || fakes[2].mergeCalled {
		t.Error("non-targeted clients should not be merged")
	}
}

func TestInstaller_Run_UnknownClientFilter_Error(t *testing.T) {
	fakes := []*fakeMerger{
		{name: "claude-code", detects: true, outcome: mcpinstall.OutcomeConfigured},
	}

	installer := mcpinstall.NewInstaller(toMergers(fakes), mcpinstall.ExecResolver("/abs/flutree"))
	_, err := installer.Run([]string{"foobar"}, false)
	if err == nil {
		t.Fatal("expected error for unknown client name, got nil")
	}
}

func TestInstaller_Run_ForceFlag_PassedToMerge(t *testing.T) {
	fake := &fakeMerger{name: "claude-code", detects: true, outcome: mcpinstall.OutcomeConfigured}
	installer := mcpinstall.NewInstaller([]mcpinstall.Merger{fake}, mcpinstall.ExecResolver("/abs/flutree"))
	_, err := installer.Run([]string{}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fake.mergeForce {
		t.Error("force should be passed to Merge()")
	}
}

func TestInstaller_Run_AbsCmdPassedToMerge(t *testing.T) {
	fake := &fakeMerger{name: "claude-code", detects: true, outcome: mcpinstall.OutcomeConfigured}
	installer := mcpinstall.NewInstaller([]mcpinstall.Merger{fake}, mcpinstall.ExecResolver("/custom/path/flutree"))
	_, err := installer.Run([]string{}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.mergeCmd != "/custom/path/flutree" {
		t.Errorf("want absCmd=/custom/path/flutree got %q", fake.mergeCmd)
	}
}

func TestInstaller_Run_MergeError_StatusError(t *testing.T) {
	fake := &fakeMerger{
		name:     "opencode",
		detects:  true,
		outcome:  mcpinstall.OutcomeError,
		mergeErr: errors.New("disk full"),
	}
	installer := mcpinstall.NewInstaller([]mcpinstall.Merger{fake}, mcpinstall.ExecResolver("/abs/flutree"))
	results, err := installer.Run([]string{}, false)
	if err != nil {
		t.Fatalf("orchestrator error should not propagate: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result got %d", len(results))
	}
	if results[0].Status != mcpinstall.OutcomeError {
		t.Errorf("want %q got %q", mcpinstall.OutcomeError, results[0].Status)
	}
	if results[0].Message == "" {
		t.Error("message should not be empty when status is error")
	}
}

// TestInstaller_Run_ResultHasConfigPath verifies that a merger implementing
// MergerWithConfigPath has its path included in the InstallResult.
func TestInstaller_Run_ResultHasConfigPath(t *testing.T) {
	dir := t.TempDir()
	// Pre-create .claude.json so Detect() returns true.
	if err := os.WriteFile(filepath.Join(dir, ".claude.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Use a real claude merger so ConfigPath() is populated.
	m := mcpinstall.NewClaudeMerger(dir)
	installer := mcpinstall.NewInstaller([]mcpinstall.Merger{m}, mcpinstall.ExecResolver("/abs/flutree"))
	results, err := installer.Run([]string{}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result got %d", len(results))
	}
	want := filepath.Join(dir, ".claude.json")
	if results[0].ConfigPath != want {
		t.Errorf("want ConfigPath=%q got %q", want, results[0].ConfigPath)
	}
}

// TestInstaller_Run_DetectError_StatusError verifies that a Detect() I/O
// error is surfaced as OutcomeError, not silently treated as not_installed.
func TestInstaller_Run_DetectError_StatusError(t *testing.T) {
	ioErr := errors.New("permission denied")
	fake := &fakeMerger{
		name:      "claude-code",
		detects:   false,
		detectErr: ioErr,
	}
	installer := mcpinstall.NewInstaller([]mcpinstall.Merger{fake}, mcpinstall.ExecResolver("/abs/flutree"))
	results, err := installer.Run([]string{}, false)
	if err != nil {
		t.Fatalf("orchestrator error should not propagate: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result got %d", len(results))
	}
	if results[0].Status != mcpinstall.OutcomeError {
		t.Errorf("want OutcomeError got %q", results[0].Status)
	}
	if results[0].Message == "" {
		t.Error("message should not be empty when detect returns an error")
	}
	if fake.mergeCalled {
		t.Error("Merge() should not be called when Detect() returns an error")
	}
}

// --- helpers ---

func toMergers(fakes []*fakeMerger) []mcpinstall.Merger {
	out := make([]mcpinstall.Merger, len(fakes))
	for i, f := range fakes {
		out[i] = f
	}
	return out
}

// writePerm writes data to path with the given file mode (for permission tests).
func writePerm(t *testing.T, path string, data []byte, perm os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, perm); err != nil {
		t.Fatal(err)
	}
}
