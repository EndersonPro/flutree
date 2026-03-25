package app

import (
	"path/filepath"
	"testing"

	"github.com/EndersonPro/flutree/internal/domain"
)

type fakeGit struct {
	currentRepo string
	worktrees   map[string][]domain.GitWorktreeEntry
	dirty       bool
	removed     string
}

func (f *fakeGit) EnsureRepo() (string, error) { return f.currentRepo, nil }
func (f *fakeGit) ListWorktrees(repoRoot string) ([]domain.GitWorktreeEntry, error) {
	return f.worktrees[filepath.Clean(repoRoot)], nil
}
func (f *fakeGit) CreateWorktree(string, string, string, string) error { return nil }
func (f *fakeGit) RemoveWorktree(repoRoot, path string, force bool) error {
	f.removed = repoRoot + "::" + path
	return nil
}
func (f *fakeGit) IsDirty(path string) (bool, error) { return f.dirty, nil }
func (f *fakeGit) DiscoverFlutterRepos(scope string) ([]domain.DiscoveredFlutterRepo, error) {
	return nil, nil
}

type fakeRegistry struct {
	records  []domain.RegistryRecord
	complete string
}

func (f *fakeRegistry) ListRecords() ([]domain.RegistryRecord, error) { return f.records, nil }
func (f *fakeRegistry) Get(name string) (domain.RegistryRecord, error) {
	for _, rec := range f.records {
		if rec.Name == name {
			return rec, nil
		}
	}
	return domain.RegistryRecord{}, domain.NewError(domain.CategoryPrecondition, 3, "not found", "", nil)
}
func (f *fakeRegistry) Upsert(record domain.RegistryRecord) error {
	f.records = append(f.records, record)
	return nil
}
func (f *fakeRegistry) Remove(name string) (domain.RegistryRecord, error) {
	return domain.RegistryRecord{}, nil
}
func (f *fakeRegistry) MarkCompleted(name string) (domain.RegistryRecord, error) {
	f.complete = name
	return domain.RegistryRecord{}, nil
}

type fakePrompt struct{ ok bool }

func (f *fakePrompt) Confirm(message string, nonInteractive, assumeYes bool) (bool, error) {
	return f.ok, nil
}
func (f *fakePrompt) ConfirmWithToken(message, token string, nonInteractive, assumeYes bool) (bool, error) {
	return f.ok, nil
}
func (f *fakePrompt) SelectOne(message string, choices []string, nonInteractive bool) (string, error) {
	return "", nil
}
func (f *fakePrompt) SelectPackages(message string, choices []string, nonInteractive bool) ([]string, error) {
	return nil, nil
}
func (f *fakePrompt) AskText(message, defaultValue string, nonInteractive bool) (string, error) {
	return defaultValue, nil
}

func TestCompleteUsesRecordRepoRootAndKeepsBranch(t *testing.T) {
	g := &fakeGit{}
	r := &fakeRegistry{records: []domain.RegistryRecord{{
		Name: "demo", Branch: "feature/demo", Path: "/tmp/wt/demo", RepoRoot: "/tmp/repo", Status: "active",
	}}}
	p := &fakePrompt{ok: true}

	s := NewCompleteService(g, r, p)
	_, err := s.Run(domain.CompleteInput{Name: "demo", Yes: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.removed != "/tmp/repo::/tmp/wt/demo" {
		t.Fatalf("expected repo-root scoped removal, got %s", g.removed)
	}
	if r.complete != "demo" {
		t.Fatalf("expected registry completion")
	}
}

func TestListOutsideRepoFallsBackToGlobalRegistry(t *testing.T) {
	g := &fakeGit{
		worktrees: map[string][]domain.GitWorktreeEntry{
			"/tmp/repo-a": {{Path: "/tmp/wt/a", Branch: "feature/a"}},
			"/tmp/repo-b": {{Path: "/tmp/wt/b", Branch: "feature/b"}},
		},
	}
	r := &fakeRegistry{records: []domain.RegistryRecord{
		{Name: "a", Branch: "feature/a", Path: "/tmp/wt/a", RepoRoot: "/tmp/repo-a", Status: "active"},
		{Name: "b", Branch: "feature/b", Path: "/tmp/wt/b", RepoRoot: "/tmp/repo-b", Status: "active"},
	}}

	s := NewListService(g, r)
	rows, err := s.Run(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}
