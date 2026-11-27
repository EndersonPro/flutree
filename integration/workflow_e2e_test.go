//go:build !windows

package integration_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

type listedRecord struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	Status       string `json:"status"`
	PackageCount int    `json:"package_count"`
}

func listManaged(t *testing.T, home string) []listedRecord {
	t.Helper()
	res := runE2E(t, t.TempDir(), testEnv(home), "list", "--global", "--json")
	res.requireCode(t, 0)
	var rows []listedRecord
	if err := json.Unmarshal([]byte(res.stdout), &rows); err != nil {
		t.Fatalf("list --json is not JSON: %v\n%s", err, res.stdout)
	}
	return rows
}

func TestE2EWorkspaceLifecycleCreateListAddRepoComplete(t *testing.T) {
	ws := createWorkspace(t, "feature-life", nil, "")
	extra := filepath.Join(ws.scope, "extra-pkg")
	initRepoWithPackageName(t, extra, "extra")

	rows := listManaged(t, ws.home)
	if len(rows) != 1 || rows[0].Name != ws.name || rows[0].Path != ws.rootWT {
		t.Fatalf("after create, list = %+v", rows)
	}

	runE2E(t, ws.rootRepo, testEnv(ws.home),
		"add-repo", ws.name, "--scope", ws.scope, "--repo", "extra-pkg",
		"--package-base", "extra-pkg=main", "--non-interactive").requireCode(t, 0)
	pkgWT := filepath.Join(ws.home, "Documents", "worktrees", ws.name, "packages", "extra-pkg")
	if !exists(filepath.Join(pkgWT, "pubspec.yaml")) {
		t.Fatalf("expected attached package worktree at %s", pkgWT)
	}
	// list groups package worktrees under their workspace row.
	if rows := listManaged(t, ws.home); len(rows) != 1 || rows[0].PackageCount != 1 || rows[0].Status != "active" {
		t.Fatalf("after add-repo, list = %+v", rows)
	}

	runE2E(t, t.TempDir(), testEnv(ws.home), "complete", ws.name, "--yes", "--non-interactive").requireCode(t, 0)
	if exists(ws.rootWT) {
		t.Fatalf("expected root worktree %s to be removed", ws.rootWT)
	}
	for _, row := range listManaged(t, ws.home) {
		if row.Name == ws.name {
			t.Fatalf("completed workspace still listed: %+v", row)
		}
	}
}

func TestE2ECompleteUnknownWorktreeFails(t *testing.T) {
	res := runE2E(t, t.TempDir(), testEnv(t.TempDir()), "complete", "ghost", "--yes", "--non-interactive")
	if res.code == 0 {
		t.Fatalf("expected failure for unknown worktree, stdout: %s", res.stdout)
	}
	if !strings.Contains(res.stderr, "ghost") {
		t.Fatalf("expected error to name the worktree, got: %s", res.stderr)
	}
}

func TestE2ECompleteNonInteractiveWithoutYesKeepsWorktree(t *testing.T) {
	ws := createWorkspace(t, "feature-keep", nil, "")

	res := runE2E(t, t.TempDir(), testEnv(ws.home), "complete", ws.name, "--non-interactive")
	if res.code == 0 {
		t.Fatalf("expected non-interactive complete without --yes to fail, stdout: %s", res.stdout)
	}
	if !exists(ws.rootWT) {
		t.Fatalf("worktree must be kept when confirmation is missing")
	}
}

func TestE2ECompleteDirtyWorktreeNeedsForce(t *testing.T) {
	ws := createWorkspace(t, "feature-dirty", nil, "")
	writeFile(t, filepath.Join(ws.rootWT, "README.md"), "local edit\n")

	dirty := runE2E(t, t.TempDir(), testEnv(ws.home), "complete", ws.name, "--yes", "--non-interactive")
	if dirty.code == 0 {
		t.Fatalf("expected dirty worktree completion to fail without --force")
	}
	if !exists(ws.rootWT) {
		t.Fatalf("dirty worktree must be kept without --force")
	}

	runE2E(t, t.TempDir(), testEnv(ws.home), "complete", ws.name, "--yes", "--non-interactive", "--force").requireCode(t, 0)
	if exists(ws.rootWT) {
		t.Fatalf("expected --force to remove the dirty worktree")
	}
}

// Every command that supports --json must write exactly one JSON document to
// stdout so scripts can pipe it.
func TestE2EJSONOutputIsTheOnlyStdoutContent(t *testing.T) {
	ws := createWorkspace(t, "feature-json", nil, "")
	tc := newFakeToolchain(t, 0)
	env := tc.env(ws.home)
	scopeDir := t.TempDir()
	other := filepath.Join(ws.scope, "other-app")
	initRepoWithPackageName(t, other, "other_app")
	extra := filepath.Join(ws.scope, "extra-pkg")
	initRepoWithPackageName(t, extra, "extra")

	cases := []struct {
		name string
		cwd  string
		args []string
	}{
		{"version", ws.home, []string{"version", "--json"}},
		{"list", ws.home, []string{"list", "--global", "--json"}},
		{"config set", ws.home, []string{"config", "set", "scope.root", scopeDir, "--json"}},
		{"config get", ws.home, []string{"config", "get", "scope.root", "--json"}},
		{"pubget", ws.home, []string{"pubget", ws.name, "--json"}},
		{"clean", ws.rootWT, []string{"clean", "--json"}},
		{"create", other, []string{"create", "feature-json-2", "--scope", ws.scope, "--root-repo", "other-app", "--no-package", "--yes", "--non-interactive", "--json"}},
		{"add-repo", ws.rootRepo, []string{"add-repo", ws.name, "--scope", ws.scope, "--repo", "extra-pkg", "--package-base", "extra-pkg=main", "--non-interactive", "--json"}},
		{"complete", ws.home, []string{"complete", ws.name, "--yes", "--non-interactive", "--force", "--json"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := runE2E(t, tc.cwd, env, tc.args...)
			res.requireCode(t, 0)
			var doc any
			if err := json.Unmarshal([]byte(res.stdout), &doc); err != nil {
				t.Fatalf("stdout is not a single JSON document: %v\nstdout:\n%s", err, res.stdout)
			}
		})
	}
}
