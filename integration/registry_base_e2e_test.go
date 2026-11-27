//go:build !windows

package integration_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func registryPath(home string) string {
	return filepath.Join(home, "Documents", "worktrees", ".worktrees_registry.json")
}

func readRegistryRecords(t *testing.T, home string) (version any, records []map[string]any) {
	t.Helper()
	b, err := os.ReadFile(registryPath(home))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Version any              `json:"version"`
		Records []map[string]any `json:"records"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("registry is not JSON: %v\n%s", err, b)
	}
	return doc.Version, doc.Records
}

func recordByName(t *testing.T, records []map[string]any, name string) map[string]any {
	t.Helper()
	for _, rec := range records {
		if rec["name"] == name {
			return rec
		}
	}
	t.Fatalf("no registry record named %q in %v", name, records)
	return nil
}

// stripBaseBranch rewrites the registry the way an older flutree wrote it:
// the same records without any base_branch field.
func stripBaseBranch(t *testing.T, home string) {
	t.Helper()
	version, records := readRegistryRecords(t, home)
	for _, rec := range records {
		delete(rec, "base_branch")
	}
	b, err := json.MarshalIndent(map[string]any{"version": version, "records": records}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(registryPath(home), append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestE2ERegistryBaseBranchRecordedByCreate(t *testing.T) {
	ws := createWorkspace(t, "feature-base", []string{"core-pkg"}, "")

	version, records := readRegistryRecords(t, ws.home)
	if version != float64(1) {
		t.Errorf("registry version = %v, must stay 1", version)
	}
	if got := recordByName(t, records, "feature-base")["base_branch"]; got != "main" {
		t.Errorf("root base_branch = %v, want main", got)
	}
	if got := recordByName(t, records, "feature-base__pkg__core-pkg")["base_branch"]; got != "main" {
		t.Errorf("package base_branch = %v, want main", got)
	}
}

func TestE2ERegistryBaseBranchRecordedByAddRepo(t *testing.T) {
	ws := createWorkspace(t, "feature-added-base", nil, "")
	initRepoWithPackageName(t, filepath.Join(ws.scope, "extra-pkg"), "extra")
	runE2E(t, ws.rootRepo, testEnv(ws.home), "add-repo", ws.name, "--scope", ws.scope, "--repo", "extra-pkg",
		"--package-base", "extra-pkg=main", "--non-interactive").requireCode(t, 0)

	_, records := readRegistryRecords(t, ws.home)
	if got := recordByName(t, records, "feature-added-base")["base_branch"]; got != "main" {
		t.Errorf("root base_branch after add-repo = %v, want main (untouched)", got)
	}
	if got := recordByName(t, records, "feature-added-base__pkg__extra-pkg")["base_branch"]; got != "main" {
		t.Errorf("add-repo package base_branch = %v, want main", got)
	}
}

func TestE2ERegistryWithoutBaseBranchStillWorks(t *testing.T) {
	ws := createWorkspace(t, "feature-old", []string{"core-pkg"}, "")
	stripBaseBranch(t, ws.home)
	before, err := os.ReadFile(registryPath(ws.home))
	if err != nil {
		t.Fatal(err)
	}
	tc := newFakeToolchain(t, 0)
	env := tc.env(ws.home)

	list := runE2E(t, t.TempDir(), env, "list", "--json")
	list.requireCode(t, 0)
	if !strings.Contains(list.stdout, "feature-old") {
		t.Errorf("list does not show the old-format workspace: %s", list.stdout)
	}
	runE2E(t, t.TempDir(), env, "pubget", ws.name).requireCode(t, 0)

	// Read-only commands never rewrite an old registry.
	after, err := os.ReadFile(registryPath(ws.home))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("list/pubget must not rewrite an old-format registry:\nbefore:\n%s\nafter:\n%s", before, after)
	}

	runE2E(t, t.TempDir(), env, "complete", ws.name, "--yes", "--non-interactive").requireCode(t, 0)
	if _, records := readRegistryRecords(t, ws.home); len(records) != 0 {
		t.Errorf("complete should remove the workspace records, left %v", records)
	}
}
