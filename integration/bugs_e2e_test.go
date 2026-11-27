//go:build !windows

package integration_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// B6: completing a workspace whose package worktree was deleted by hand must
// prune git's metadata so the same path can be created again.
func TestBugCompletePrunesManuallyDeletedWorktree(t *testing.T) {
	ws := createWorkspace(t, "feat-w", []string{"pkg-a", "pkg-b"}, "")
	pkgB := ws.pkgWTs["pkg-b"]
	if err := os.RemoveAll(pkgB); err != nil {
		t.Fatal(err)
	}

	runE2E(t, ws.rootRepo, testEnv(ws.home), "complete", "feat-w", "--yes", "--non-interactive").requireCode(t, 0)

	list := runGit(t, filepath.Join(ws.scope, "pkg-b"), "worktree", "list", "--porcelain")
	if strings.Contains(list, "prunable") || strings.Contains(list, "feat-w") {
		t.Fatalf("source repo still lists the removed worktree:\n%s", list)
	}
	container := filepath.Join(ws.home, "Documents", "worktrees", "feat-w")
	if exists(container) {
		t.Fatalf("container %s was left behind", container)
	}

	runE2E(t, ws.rootRepo, testEnv(ws.home),
		"create", "feat-w", "--scope", ws.scope, "--root-repo", "root-app", "--yes", "--non-interactive",
		"--package", "pkg-a", "--package-base", "pkg-a=main",
		"--package", "pkg-b", "--package-base", "pkg-b=main",
		"--reuse-existing-branch",
	).requireCode(t, 0)
}

// B7: a failed create removes the container it made, and a retry works.
func TestBugFailedCreateRemovesItsContainer(t *testing.T) {
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	repo := filepath.Join(scope, "root-app")
	initRepoWithPubspec(t, repo, "root_app", "")
	runGit(t, repo, "branch", "feature/feat-x")
	env := testEnv(home)
	args := []string{"create", "feat-x", "--scope", scope, "--root-repo", "root-app", "--no-package", "--yes", "--non-interactive"}

	runE2E(t, repo, env, args...).requireCode(t, 2)
	container := filepath.Join(home, "Documents", "worktrees", "feat-x")
	if exists(container) {
		t.Fatalf("failed create left container %s behind", container)
	}

	runE2E(t, repo, env, append(args, "--reuse-existing-branch")...).requireCode(t, 0)
}

// B7: a container that existed before the run is never removed.
func TestBugFailedCreateKeepsPreExistingContainer(t *testing.T) {
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	repo := filepath.Join(scope, "root-app")
	initRepoWithPubspec(t, repo, "root_app", "")
	container := filepath.Join(home, "Documents", "worktrees", "feat-y")
	keep := filepath.Join(container, "keep.txt")
	writeFile(t, keep, "mine")

	res := runE2E(t, repo, testEnv(home), "create", "feat-y", "--scope", scope, "--root-repo", "root-app", "--no-package", "--yes", "--non-interactive")
	if res.code == 0 {
		t.Fatalf("create over an existing container must fail\n%s", res.stdout)
	}
	if !exists(keep) {
		t.Fatalf("pre-existing container content was removed")
	}
}

// B8: the --json error code equals the process exit code.
func TestBugJSONErrorCodeMatchesExitCode(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"precondition", []string{"pubget", "ghost", "--json"}, 1},
		{"input", []string{"pubget", " ", "--json"}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := runE2E(t, cwd, testEnv(home), tc.args...)
			res.requireCode(t, tc.want)
			var env struct {
				Error struct {
					Code int `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(res.stderr), &env); err != nil {
				t.Fatalf("stderr is not a JSON envelope: %v\n%s", err, res.stderr)
			}
			if env.Error.Code != tc.want {
				t.Fatalf("json code = %d, want %d\n%s", env.Error.Code, tc.want, res.stderr)
			}
		})
	}
}

// B9: a pubget failure shows the tool output.
func TestBugPubgetFailureShowsToolOutput(t *testing.T) {
	ws := createWorkspace(t, "feat-p", []string{"pkg-a"}, "")
	tc := newFakeToolchain(t, 1)
	writeFile(t, filepath.Join(ws.pkgWTs["pkg-a"], "pubspec.yaml"), "name: pkg_a\n")
	writeFile(t, filepath.Join(ws.rootWT, "pubspec.yaml"), "name: root_app\n")

	res := runE2E(t, ws.rootRepo, tc.env(ws.home), "pubget", "feat-p")
	res.requireCode(t, 1)
	if !strings.Contains(res.stderr, "dart failed") {
		t.Fatalf("stderr hides the tool output:\n%s", res.stderr)
	}
}
