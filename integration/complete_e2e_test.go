//go:build !windows

package integration_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitStatusPorcelain(t *testing.T, dir string) string {
	t.Helper()
	return strings.TrimSpace(runGit(t, dir, "status", "--porcelain"))
}

func isGitIgnored(t *testing.T, dir, file string) bool {
	t.Helper()
	cmd := exec.Command("git", "check-ignore", "-q", file)
	cmd.Dir = dir
	return cmd.Run() == nil
}

func TestE2ECreateKeepsTrackedGitignoreAndIgnoresOverrides(t *testing.T) {
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	rootRepo := filepath.Join(scope, "root-app")
	initRepoWithPackageName(t, rootRepo, "root_app")
	writeFile(t, filepath.Join(rootRepo, ".gitignore"), "build/\n")
	runGit(t, rootRepo, "add", ".gitignore")
	runGit(t, rootRepo, "commit", "-m", "gitignore")
	runGit(t, rootRepo, "push", "origin", "main")
	initRepoWithPackageName(t, filepath.Join(scope, "core-pkg"), "core")

	runE2E(t, rootRepo, testEnv(home), "create", "feature-gi", "--scope", scope, "--root-repo", "root-app",
		"--package", "core-pkg", "--package-base", "core-pkg=main", "--yes", "--non-interactive").requireCode(t, 0)

	rootWT := filepath.Join(home, "Documents", "worktrees", "feature-gi", "root", "root-app")
	if !exists(filepath.Join(rootWT, "pubspec_overrides.yaml")) {
		t.Fatalf("expected pubspec_overrides.yaml in %s", rootWT)
	}
	if status := gitStatusPorcelain(t, rootWT); status != "" {
		t.Fatalf("root worktree must be clean after create, got:\n%s", status)
	}
	b, err := os.ReadFile(filepath.Join(rootWT, ".gitignore"))
	if err != nil || string(b) != "build/\n" {
		t.Fatalf("tracked .gitignore must stay untouched, got %q (%v)", b, err)
	}
	if !isGitIgnored(t, rootWT, "pubspec_overrides.yaml") {
		t.Fatalf("pubspec_overrides.yaml must still be ignored by git")
	}
}

func TestE2ECompleteFreshWorkspaceWithPackagesWithoutForce(t *testing.T) {
	ws := createWorkspace(t, "feature-fresh", []string{"core-pkg"}, "")
	if status := gitStatusPorcelain(t, ws.rootWT); status != "" {
		t.Fatalf("root worktree must be clean after create, got:\n%s", status)
	}

	runE2E(t, t.TempDir(), testEnv(ws.home), "complete", ws.name, "--yes", "--non-interactive").requireCode(t, 0)
	if exists(ws.rootWT) || exists(ws.pkgWTs["core-pkg"]) {
		t.Fatalf("expected root and package worktrees to be removed")
	}
}

func TestE2ECompleteAfterAddRepoWithoutForce(t *testing.T) {
	ws := createWorkspace(t, "feature-added", nil, "")
	initRepoWithPackageName(t, filepath.Join(ws.scope, "core-pkg"), "core")
	runE2E(t, ws.rootRepo, testEnv(ws.home), "add-repo", ws.name, "--scope", ws.scope, "--repo", "core-pkg",
		"--package-base", "core-pkg=main", "--non-interactive").requireCode(t, 0)
	if status := gitStatusPorcelain(t, ws.rootWT); status != "" {
		t.Fatalf("root worktree must be clean after add-repo, got:\n%s", status)
	}

	runE2E(t, t.TempDir(), testEnv(ws.home), "complete", ws.name, "--yes", "--non-interactive").requireCode(t, 0)
	if exists(ws.rootWT) {
		t.Fatalf("expected root worktree to be removed")
	}
}
