//go:build !windows

package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	keyEnter = "\r"
	keyDown  = "\x1b[B"
	keyUp    = "\x1b[A"
	keyTab   = "\t"
	keyEsc   = "\x1b"
	keyCtrlO = "\x0f"
)

// navigatorWorkspace builds <root>/ws with two Flutter repos (root-app and
// core, each with a develop branch) and an empty sibling directory <root>/empty.
type navigatorWorkspace struct {
	root  string
	ws    string
	empty string
	home  string
}

func newNavigatorWorkspace(t *testing.T) navigatorWorkspace {
	t.Helper()
	root := t.TempDir()
	w := navigatorWorkspace{
		root:  root,
		ws:    filepath.Join(root, "ws"),
		empty: filepath.Join(root, "empty"),
		home:  t.TempDir(),
	}
	if err := os.MkdirAll(w.empty, 0o755); err != nil {
		t.Fatal(err)
	}
	initRepoWithPackageName(t, filepath.Join(w.ws, "root-app"), "root_app")
	initRepoWithPackageName(t, filepath.Join(w.ws, "core"), "core")
	runGit(t, filepath.Join(w.ws, "core"), "branch", "develop")
	return w
}

func configScopeRoot(t *testing.T, bin, home string) string {
	t.Helper()
	res := runCLI(t, bin, home, testEnv(home), "", "config", "get", "scope.root")
	if res.code != 0 {
		t.Fatalf("config get scope.root failed: %d %s", res.code, res.stderr)
	}
	return strings.TrimSpace(res.stdout)
}

func sameDir(t *testing.T, got, want string) bool {
	t.Helper()
	g, err1 := filepath.EvalSymlinks(got)
	w, err2 := filepath.EvalSymlinks(want)
	if err1 != nil || err2 != nil {
		return got == want
	}
	return g == w
}

// navigateToWorkspace walks from <root>/empty up to <root>, into ws and selects it.
func navigateToWorkspace() []string {
	return []string{
		"Current folder::" + keyEnter,                 // ".." -> <root>
		"ws/::" + keyDown + keyEnter,                  // empty/ -> ws/ and open it
		"core/::" + keyTab,                            // select ws as scope
		"Step 1 - Workspace name::" + keyEnter,        // keep the default name
		"Step 2 - Choose root repository::root",       // filter by typing
		"Filter: root::" + keyEnter,                   // root-app
		"Step 3 - Select packages::" + keyEnter,       // core is preselected
		"Workspace branch::" + keyEnter,               // accept branch
		"Root base branch::" + keyEnter,               // accept base
		"Generate .code-workspace file::" + keyEnter,  // accept workspace toggle
		"Base branch for package 'core'::" + keyEnter, // accept develop
	}
}

func TestNavigatorCreateFromEmptyDirectorySavesScope(t *testing.T) {
	bin := buildCLI(t)
	w := newNavigatorWorkspace(t)

	script := append(navigateToWorkspace(),
		"Step 5 - Final review::"+keyEnter,
		"Update local branches from origin::n\n",
	)
	res := runCLIWithPTY(t, bin, w.empty, testEnv(w.home), script, "create", "feature-nav", "--branch", "feature/nav")
	if res.code != 0 {
		t.Fatalf("expected success, got %d\nstdout: %s\nstderr: %s", res.code, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, "as default scope (scope.root): ON") {
		t.Fatalf("expected save-scope toggle on the review step, got: %s", res.stdout)
	}

	base := worktreeBase(t, w.home)
	for _, p := range []string{
		filepath.Join(base, "root", "root-app"),
		filepath.Join(base, "packages", "core"),
	} {
		if !exists(p) {
			t.Fatalf("expected worktree %s to exist", p)
		}
	}
	if got := configScopeRoot(t, bin, w.home); !sameDir(t, got, w.ws) {
		t.Fatalf("scope.root = %q, want %q", got, w.ws)
	}
}

func TestNavigatorCreateWithSaveToggleOffDoesNotPersistScope(t *testing.T) {
	bin := buildCLI(t)
	w := newNavigatorWorkspace(t)

	script := append(navigateToWorkspace(),
		"(scope.root): ON::s",
		"(scope.root): OFF::"+keyEnter,
		"Update local branches from origin::n\n",
	)
	res := runCLIWithPTY(t, bin, w.empty, testEnv(w.home), script, "create", "feature-nav", "--branch", "feature/nav")
	if res.code != 0 {
		t.Fatalf("expected success, got %d\nstdout: %s\nstderr: %s", res.code, res.stdout, res.stderr)
	}
	if !exists(filepath.Join(w.home, "Documents", "worktrees", "feature-nav", "root", "root-app")) {
		t.Fatalf("expected root worktree to exist")
	}
	if got := configScopeRoot(t, bin, w.home); got != "" {
		t.Fatalf("scope.root must stay unset when the toggle is off, got %q", got)
	}
}

func TestNavigatorFilterNarrowsRootAndKeepsPackageSelection(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	ws := filepath.Join(t.TempDir(), "ws")
	for name, pkg := range map[string]string{"alpha-app": "alpha_app", "beta-app": "beta_app", "core": "core"} {
		initRepoWithPackageName(t, filepath.Join(ws, name), pkg)
		runGit(t, filepath.Join(ws, name), "branch", "develop")
	}

	script := []string{
		"Step 1 - Workspace name::" + keyEnter,
		"Step 2 - Choose root repository::beta",
		"Filter: beta::" + keyEnter,                        // root = beta-app
		"Step 3 - Select packages::cor",                    // alpha-app stays preselected
		"Filter: cor::" + " ",                              // toggle core on while filtered
		"\u25c9 core::" + keyEnter,                         // core selected, continue
		"Workspace branch::" + keyEnter,                    // branch
		"Root base branch::" + keyEnter,                    // base
		"Generate .code-workspace file::" + keyEnter,       // workspace
		"Base branch for package 'alpha-app'::" + keyEnter, // alpha-app base
		"Base branch for package 'core'::" + keyEnter,      // core base
		"Step 5 - Final review::" + keyEnter,               // apply
		"Update local branches from origin::n\n",
	}

	res := runCLIWithPTY(t, bin, ws, testEnv(home), script, "create", "feature-filter", "--branch", "feature/filter")
	if res.code != 0 {
		t.Fatalf("expected success, got %d\nstdout: %s\nstderr: %s", res.code, res.stdout, res.stderr)
	}
	if strings.Contains(res.stdout, "as default scope") {
		t.Fatalf("save-scope toggle must not appear when the navigator was not used: %s", res.stdout)
	}
	base := worktreeBase(t, home)
	if !exists(filepath.Join(base, "root", "beta-app")) {
		t.Fatalf("expected filtered root beta-app to be the root worktree")
	}
	for _, name := range []string{"alpha-app", "core"} {
		if !exists(filepath.Join(base, "packages", name)) {
			t.Fatalf("expected package worktree %s (selection made before and during filtering)", name)
		}
	}
	if got := configScopeRoot(t, bin, home); got != "" {
		t.Fatalf("scope.root must stay unset, got %q", got)
	}
}

func TestNavigatorFolderWithoutReposShowsMessageAndStays(t *testing.T) {
	bin := buildCLI(t)
	w := newNavigatorWorkspace(t)

	res := runCLIWithPTY(t, bin, w.empty, testEnv(w.home), []string{
		"Current folder::" + keyTab,
		"No Flutter repositories found in ::" + keyEsc,
	}, "create", "feature-nav")
	if res.code != 2 {
		t.Fatalf("expected cancel exit code 2, got %d\nstdout: %s\nstderr: %s", res.code, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stderr, "No Flutter repositories found in ") {
		t.Fatalf("expected the no-repositories message naming the directory, got: %s", res.stderr)
	}
	if !strings.Contains(res.stderr, "Create cancelled before execution") {
		t.Fatalf("expected Esc with no previous scope to cancel the wizard, got: %s", res.stderr)
	}
	if exists(worktreeBase(t, w.home)) {
		t.Fatalf("cancelled create must not create worktrees")
	}
}

func TestNavigatorCanChangeScopeFromRootStep(t *testing.T) {
	bin := buildCLI(t)
	w := newNavigatorWorkspace(t)
	other := filepath.Join(w.root, "other")
	initRepoWithPackageName(t, filepath.Join(other, "solo-app"), "solo_app")

	// Start in ws (has repos), open the navigator with ctrl+o, go up, pick other/, select it.
	res := runCLIWithPTY(t, bin, w.ws, testEnv(w.home), []string{
		"Step 1 - Workspace name::" + keyEnter,
		"change folder::" + keyCtrlO,
		"Current folder::" + keyEnter, // ".." -> <root>
		"other/::" + keyUp + keyEnter, // cursor lands on ws/, other/ is above it
		"solo-app/::" + keyTab,
		"solo-app [solo_app]::" + keyEsc,
	}, "create", "feature-nav")
	if res.code != 2 {
		t.Fatalf("expected cancel exit code 2, got %d\nstdout: %s\nstderr: %s", res.code, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout+res.stderr, "solo-app [solo_app]") {
		t.Fatalf("expected the new scope's repositories on the root step")
	}
}

func TestNavigatorNonInteractiveWithoutReposFailsAsBefore(t *testing.T) {
	w := newNavigatorWorkspace(t)

	res := runE2E(t, w.empty, testEnv(w.home), "create", "feature-nav", "--root-repo", "root-app", "--yes", "--non-interactive")
	res.requireCode(t, 1)
	if !strings.Contains(res.stderr, "No Flutter repositories were discovered in execution scope.") {
		t.Fatalf("unexpected stderr: %s", res.stderr)
	}

	res = runE2E(t, w.empty, testEnv(w.home), "create", "feature-nav", "--scope", filepath.Join(w.root, "missing"), "--root-repo", "root-app", "--yes", "--non-interactive")
	res.requireCode(t, 2)
}

// worktreeBase returns the single workspace directory created under HOME. The
// terminal's background-colour reply can leak into the name field, so the
// directory name is looked up instead of assumed. It returns a path that does
// not exist when nothing was created.
func worktreeBase(t *testing.T, home string) string {
	t.Helper()
	root := filepath.Join(home, "Documents", "worktrees")
	entries, err := os.ReadDir(root)
	if err != nil {
		return filepath.Join(root, "none")
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "feature-") {
			return filepath.Join(root, e.Name())
		}
	}
	return filepath.Join(root, "none")
}
