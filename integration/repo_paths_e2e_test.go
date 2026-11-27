//go:build !windows

package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func hasBranch(t *testing.T, repo, branch string) bool {
	t.Helper()
	return strings.TrimSpace(runGit(t, repo, "branch", "--list", branch)) != ""
}

func TestRepoPathCreateAcceptsReposOutsideScope(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	base := t.TempDir()
	scope := filepath.Join(base, "scope")
	initRepoWithPackageName(t, filepath.Join(scope, "decoy"), "decoy")
	rootRepo := filepath.Join(base, "a", "root-app")
	pkgRepo := filepath.Join(base, "b", "libs", "pkg-x")
	initRepoWithPackageName(t, rootRepo, "root_app")
	initRepoWithPackageName(t, pkgRepo, "pkg_x")

	res := runCLI(t, bin, base, testEnv(home), "", "create", "feature-paths", "--scope", scope,
		"--root-repo", rootRepo, "--package", pkgRepo, "--package-base", "pkg-x=main",
		"--non-interactive", "--yes")
	if res.code != 0 {
		t.Fatalf("expected success, got %d\nstdout: %s\nstderr: %s", res.code, res.stdout, res.stderr)
	}
	container := filepath.Join(home, "Documents", "worktrees", "feature-paths")
	for _, p := range []string{filepath.Join(container, "root", "root-app"), filepath.Join(container, "packages", "pkg-x")} {
		if !exists(p) {
			t.Fatalf("expected worktree %s", p)
		}
	}
	overrides, err := os.ReadFile(filepath.Join(container, "root", "root-app", "pubspec_overrides.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(overrides), "pkg_x") || !strings.Contains(string(overrides), "packages/pkg-x") {
		t.Fatalf("overrides must point pkg_x at its worktree:\n%s", overrides)
	}
}

func TestRepoPathCreateWorksWhenScopeHasNoRepos(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	base := t.TempDir()
	scope := filepath.Join(base, "empty-scope")
	if err := os.MkdirAll(scope, 0o755); err != nil {
		t.Fatal(err)
	}
	rootRepo := filepath.Join(base, "somewhere", "root-app")
	initRepoWithPackageName(t, rootRepo, "root_app")

	// A relative path is resolved from the current directory.
	res := runCLI(t, bin, filepath.Join(base, "somewhere"), testEnv(home), "", "create", "feature-rel", "--scope", scope,
		"--root-repo", "./root-app", "--no-package", "--non-interactive", "--yes")
	if res.code != 0 {
		t.Fatalf("expected success, got %d\nstdout: %s\nstderr: %s", res.code, res.stdout, res.stderr)
	}
	if !exists(filepath.Join(home, "Documents", "worktrees", "feature-rel", "root", "root-app")) {
		t.Fatalf("expected the root worktree")
	}
}

func TestRepoPathAddRepoAcceptsRepoOutsideScope(t *testing.T) {
	ws := createWorkspace(t, "feature-addpath", nil, "")
	outside := filepath.Join(t.TempDir(), "elsewhere", "pkg-z")
	initRepoWithPackageName(t, outside, "pkg_z")

	res := runE2E(t, ws.rootRepo, testEnv(ws.home), "add-repo", ws.name, "--scope", ws.scope,
		"--repo", outside, "--package-base", "pkg-z=main", "--non-interactive")
	res.requireCode(t, 0)
	if !exists(filepath.Join(ws.home, "Documents", "worktrees", ws.name, "packages", "pkg-z")) {
		t.Fatalf("expected the attached package worktree")
	}
}

func TestRepoPathMissingPackageBaseFailsBeforeAnySideEffect(t *testing.T) {
	home := t.TempDir()
	scope := t.TempDir()
	rootRepo := filepath.Join(scope, "root-app")
	pkgRepo := filepath.Join(scope, "pkg-x")
	initRepoWithPackageName(t, rootRepo, "root_app")
	initRepoWithPackageName(t, pkgRepo, "pkg_x")

	res := runE2E(t, scope, testEnv(home), "create", "feature-nobase", "--scope", scope,
		"--root-repo", "root-app", "--package", "pkg-x", "--non-interactive", "--yes")
	res.requireCode(t, 2)
	if !strings.Contains(res.stderr, "Base branch 'develop' does not exist in pkg-x.") {
		t.Fatalf("unexpected stderr: %s", res.stderr)
	}
	if !strings.Contains(res.stderr, "Pass --package-base pkg-x=<branch>.") {
		t.Fatalf("expected the hint, got: %s", res.stderr)
	}
	for _, repo := range []string{rootRepo, pkgRepo} {
		if hasBranch(t, repo, "feature/feature-nobase") {
			t.Fatalf("no branch must be created in %s", repo)
		}
	}
	if exists(filepath.Join(home, "Documents", "worktrees", "feature-nobase")) {
		t.Fatalf("no container may be created")
	}
}

func TestRepoPathFailedCreateDeletesOnlyBranchesItCreated(t *testing.T) {
	home := t.TempDir()
	scope := t.TempDir()
	rootRepo := filepath.Join(scope, "root-app")
	pkgA := filepath.Join(scope, "pkg-a")
	pkgB := filepath.Join(scope, "pkg-b")
	initRepoWithPackageName(t, rootRepo, "root_app")
	initRepoWithPackageName(t, pkgA, "pkg_a")
	initRepoWithPackageName(t, pkgB, "pkg_b")
	// pkg-a already has the branch: it is reused and must survive the failure.
	runGit(t, pkgA, "branch", "feature/keep")
	// pkg-b has the branch checked out, so adding a worktree for it fails.
	runGit(t, pkgB, "checkout", "-b", "feature/keep")

	res := runE2E(t, scope, testEnv(home), "create", "feature-fail", "--scope", scope,
		"--branch", "feature/keep", "--root-repo", "root-app", "--package", "pkg-a", "--package", "pkg-b",
		"--package-base", "pkg-a=main", "--package-base", "pkg-b=main",
		"--reuse-existing-branch", "--non-interactive", "--yes")
	if res.code == 0 {
		t.Fatalf("expected the create to fail\nstdout: %s", res.stdout)
	}
	if hasBranch(t, rootRepo, "feature/keep") {
		t.Fatalf("the branch created in this run must be deleted from root-app")
	}
	for _, repo := range []string{pkgA, pkgB} {
		if !hasBranch(t, repo, "feature/keep") {
			t.Fatalf("the pre-existing branch must survive in %s", repo)
		}
	}
	if exists(filepath.Join(home, "Documents", "worktrees", "feature-fail")) {
		t.Fatalf("the container must be rolled back")
	}
}
