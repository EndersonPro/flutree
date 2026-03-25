package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/EndersonPro/flutree/internal/domain"
)

func TestRepositoryUpsertGetRemoveRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	repo := &Repository{Path: path}

	record := domain.RegistryRecord{
		Name:     "demo",
		Branch:   "feature/demo",
		Path:     "/tmp/worktrees/demo",
		RepoRoot: "/tmp/repo",
		Status:   "active",
	}
	if err := repo.Upsert(record); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	got, err := repo.Get("demo")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got.Name != "demo" || got.Branch != "feature/demo" {
		t.Fatalf("unexpected record: %+v", got)
	}

	removed, err := repo.Remove("demo")
	if err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if removed.Name != "demo" {
		t.Fatalf("unexpected removed record: %+v", removed)
	}
}

func TestRepositoryRejectsDuplicateNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	payload := domain.RegistryDocument{
		Version: 1,
		Records: []domain.RegistryRecord{
			{Name: "demo", Branch: "a", Path: "/a", RepoRoot: "/repo", Status: "active"},
			{Name: "demo", Branch: "b", Path: "/b", RepoRoot: "/repo", Status: "active"},
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	repo := &Repository{Path: path}
	_, err = repo.ListRecords()
	if err == nil {
		t.Fatalf("expected integrity error for duplicate names")
	}
}
