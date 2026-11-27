//go:build !windows

package integration_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

const flutterPubspec = "dependencies:\n  flutter:\n    sdk: flutter\n"

func TestE2EPubGetRunsDartPubGetInRootWorktree(t *testing.T) {
	ws := createWorkspace(t, "feature-pub", nil, "")
	tc := newFakeToolchain(t, 0)

	res := runE2E(t, t.TempDir(), tc.env(ws.home), "pubget", ws.name)
	res.requireCode(t, 0)

	want := []string{"dart pub get @ " + realPath(t, ws.rootWT)}
	if got := tc.calls(t); !equalLines(got, want) {
		t.Fatalf("toolchain calls = %q, want %q", got, want)
	}
}

func TestE2EPubGetRunsFlutterInPackagesAndRoot(t *testing.T) {
	ws := createWorkspace(t, "feature-pub", []string{"core-pkg"}, flutterPubspec)
	tc := newFakeToolchain(t, 0)

	res := runE2E(t, t.TempDir(), tc.env(ws.home), "pubget", ws.name)
	res.requireCode(t, 0)

	want := []string{
		"flutter pub get @ " + realPath(t, ws.pkgWTs["core-pkg"]),
		"flutter pub get @ " + realPath(t, ws.rootWT),
	}
	if got := tc.calls(t); !equalLines(got, want) {
		t.Fatalf("toolchain calls = %q, want %q (packages first, then root)", got, want)
	}
}

func TestE2EPubGetForceCleansAndRemovesLockBeforeGet(t *testing.T) {
	ws := createWorkspace(t, "feature-pub", nil, flutterPubspec)
	lock := filepath.Join(ws.rootWT, "pubspec.lock")
	writeFile(t, lock, "packages: {}\n")
	tc := newFakeToolchain(t, 0)

	res := runE2E(t, t.TempDir(), tc.env(ws.home), "pubget", ws.name, "--force")
	res.requireCode(t, 0)

	wt := realPath(t, ws.rootWT)
	want := []string{"flutter clean @ " + wt, "flutter pub get @ " + wt}
	if got := tc.calls(t); !equalLines(got, want) {
		t.Fatalf("toolchain calls = %q, want %q", got, want)
	}
	if exists(lock) {
		t.Fatalf("expected %s to be removed by --force", lock)
	}
}

func TestE2EPubGetJSONWritesOnlyJSONToStdout(t *testing.T) {
	ws := createWorkspace(t, "feature-pub", []string{"core-pkg"}, "")
	tc := newFakeToolchain(t, 0)

	res := runE2E(t, t.TempDir(), tc.env(ws.home), "pubget", ws.name, "--json")
	res.requireCode(t, 0)

	var out struct {
		WorkspaceName string `json:"workspace_name"`
		Root          struct {
			Path string `json:"path"`
			Tool string `json:"tool"`
			Role string `json:"role"`
		} `json:"root"`
		Packages []struct {
			Path string `json:"path"`
			Role string `json:"role"`
		} `json:"packages"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &out); err != nil {
		t.Fatalf("stdout is not a single JSON document: %v\nstdout:\n%s", err, res.stdout)
	}
	if out.WorkspaceName != ws.name || out.Root.Path != ws.rootWT || out.Root.Tool != "dart" || out.Root.Role != "root" {
		t.Fatalf("unexpected root result: %+v", out)
	}
	if len(out.Packages) != 1 || out.Packages[0].Path != ws.pkgWTs["core-pkg"] || out.Packages[0].Role != "package" {
		t.Fatalf("unexpected packages: %+v", out.Packages)
	}
}

func TestE2EPubGetUnknownWorkspaceIsPreconditionError(t *testing.T) {
	home := t.TempDir()
	tc := newFakeToolchain(t, 0)

	res := runE2E(t, t.TempDir(), tc.env(home), "pubget", "ghost")
	res.requireCode(t, 1)
	if !strings.Contains(res.stderr, "[precondition] Managed workspace 'ghost' was not found in registry.") {
		t.Fatalf("unexpected stderr: %s", res.stderr)
	}
	if calls := tc.calls(t); len(calls) != 0 {
		t.Fatalf("expected no toolchain calls, got %q", calls)
	}
}

func TestE2EPubGetToolFailureExitsOne(t *testing.T) {
	ws := createWorkspace(t, "feature-pub", nil, "")
	tc := newFakeToolchain(t, 3)

	res := runE2E(t, t.TempDir(), tc.env(ws.home), "pubget", ws.name)
	res.requireCode(t, 1)
	if !strings.Contains(res.stderr, "Failed to run pub get for root repository.") {
		t.Fatalf("unexpected stderr: %s", res.stderr)
	}
}

func TestE2ECleanDartRemovesDartToolFromCurrentWorktree(t *testing.T) {
	ws := createWorkspace(t, "feature-clean", nil, "")
	dartTool := filepath.Join(ws.rootWT, ".dart_tool", "package_config.json")
	writeFile(t, dartTool, "{}")
	lock := filepath.Join(ws.rootWT, "pubspec.lock")
	writeFile(t, lock, "packages: {}\n")
	tc := newFakeToolchain(t, 0)

	res := runE2E(t, ws.rootWT, tc.env(ws.home), "clean")
	res.requireCode(t, 0)

	if exists(filepath.Dir(dartTool)) {
		t.Fatalf("expected .dart_tool to be removed")
	}
	if !exists(lock) {
		t.Fatalf("expected pubspec.lock to be kept without --force")
	}
	if calls := tc.calls(t); len(calls) != 0 {
		t.Fatalf("dart clean must not shell out, got %q", calls)
	}
}

func TestE2ECleanFlutterForceRunsFlutterCleanAndRemovesLock(t *testing.T) {
	ws := createWorkspace(t, "feature-clean", nil, flutterPubspec)
	lock := filepath.Join(ws.rootWT, "pubspec.lock")
	writeFile(t, lock, "packages: {}\n")
	tc := newFakeToolchain(t, 0)

	res := runE2E(t, ws.rootWT, tc.env(ws.home), "clean", "--force", "--json")
	res.requireCode(t, 0)

	want := []string{"flutter clean @ " + realPath(t, ws.rootWT)}
	if got := tc.calls(t); !equalLines(got, want) {
		t.Fatalf("toolchain calls = %q, want %q", got, want)
	}
	if exists(lock) {
		t.Fatalf("expected pubspec.lock to be removed by --force")
	}
	var out struct {
		Tool        string `json:"tool"`
		Force       bool   `json:"force"`
		LockRemoved bool   `json:"lock_removed"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &out); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, res.stdout)
	}
	if out.Tool != "flutter" || !out.Force || !out.LockRemoved {
		t.Fatalf("unexpected clean result: %+v", out)
	}
}

func TestE2ECleanOutsideManagedWorktreeIsPreconditionError(t *testing.T) {
	ws := createWorkspace(t, "feature-clean", nil, "")

	// The source repository is a Git repo but not a managed worktree.
	res := runE2E(t, ws.rootRepo, testEnv(ws.home), "clean")
	res.requireCode(t, 1)
	if !strings.Contains(res.stderr, "[precondition] Current repository is not a managed flutree worktree.") {
		t.Fatalf("unexpected stderr: %s", res.stderr)
	}
}

func TestE2ECleanOutsideGitRepositoryFails(t *testing.T) {
	// flutree recognizes git's English "not a git repository" message, so force
	// git's C locale to keep the test independent of the machine's language.
	env := append(testEnv(t.TempDir()), "LC_ALL=C", "LANG=C", "LANGUAGE=")
	res := runE2E(t, t.TempDir(), env, "clean")
	res.requireCode(t, 1)
	if !strings.Contains(res.stderr, "[precondition] Current directory is not inside a Git repository.") {
		t.Fatalf("unexpected stderr: %s", res.stderr)
	}
}

func equalLines(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
