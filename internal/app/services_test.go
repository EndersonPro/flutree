package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EndersonPro/flutree/internal/domain"
)

type fakeGit struct {
	currentRepo string
	worktrees   map[string][]domain.GitWorktreeEntry
	dirtyByPath map[string]bool
	removed     []string
}

func (f *fakeGit) EnsureRepo() (string, error) { return f.currentRepo, nil }
func (f *fakeGit) ListWorktrees(repoRoot string) ([]domain.GitWorktreeEntry, error) {
	return f.worktrees[filepath.Clean(repoRoot)], nil
}
func (f *fakeGit) CreateWorktree(string, string, string, string) error { return nil }
func (f *fakeGit) CreateWorktreeNew(string, string, string, string) error {
	return nil
}
func (f *fakeGit) CreateWorktreeExisting(string, string, string) error { return nil }
func (f *fakeGit) BranchExists(string, string) (bool, error)           { return false, nil }
func (f *fakeGit) SyncBranchWithRemote(string, string) error           { return nil }
func (f *fakeGit) SyncBaseBranch(string, string) (string, error)       { return "origin/main", nil }
func (f *fakeGit) RemoveWorktree(repoRoot, path string, force bool) error {
	f.removed = append(f.removed, repoRoot+"::"+path)
	return nil
}
func (f *fakeGit) IsDirty(path string) (bool, error) {
	if f.dirtyByPath == nil {
		return false, nil
	}
	return f.dirtyByPath[path], nil
}
func (f *fakeGit) DiscoverFlutterRepos(scope string) ([]domain.DiscoveredFlutterRepo, error) {
	return nil, nil
}

type fakeRegistry struct {
	records   []domain.RegistryRecord
	completed []string
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
	f.completed = append(f.completed, name)
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

func managedPaths(t *testing.T, name string) (container, rootPath, packagePath string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	container = filepath.Join(destinationRoot(), normalizeWorktreeName(name))
	rootPath = filepath.Join(container, "root", "root-app")
	packagePath = filepath.Join(container, "packages", "core")
	return container, rootPath, packagePath
}

func TestCompleteUsesRecordRepoRootAndKeepsBranch(t *testing.T) {
	container, rootPath, _ := managedPaths(t, "demo")
	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		t.Fatalf("failed to create root path: %v", err)
	}

	g := &fakeGit{}
	r := &fakeRegistry{records: []domain.RegistryRecord{{
		Name: "demo", Branch: "feature/demo", Path: rootPath, RepoRoot: "/tmp/repo", Status: "active",
	}}}
	p := &fakePrompt{ok: true}

	s := NewCompleteService(g, r, p)
	_, err := s.Run(domain.CompleteInput{Name: "demo", Yes: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(g.removed) != 1 || g.removed[0] != "/tmp/repo::"+rootPath {
		t.Fatalf("expected repo-root scoped removal, got %v", g.removed)
	}
	if len(r.completed) != 1 || r.completed[0] != "demo" {
		t.Fatalf("expected registry completion, got %v", r.completed)
	}
	if _, err := os.Stat(container); !os.IsNotExist(err) {
		t.Fatalf("expected container removal, stat err=%v", err)
	}
}

func TestCompleteRootAlsoCompletesAssociatedPackages(t *testing.T) {
	container, rootPath, packagePath := managedPaths(t, "demo")
	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		t.Fatalf("failed to create root path: %v", err)
	}
	if err := os.MkdirAll(packagePath, 0o755); err != nil {
		t.Fatalf("failed to create package path: %v", err)
	}

	g := &fakeGit{}
	r := &fakeRegistry{records: []domain.RegistryRecord{
		{Name: "demo", Branch: "feature/demo", Path: rootPath, RepoRoot: "/tmp/repo-root", Status: "active"},
		{Name: "demo__pkg__core", Branch: "feature/demo-core", Path: packagePath, RepoRoot: "/tmp/repo-core", Status: "active"},
		{Name: "other", Branch: "feature/other", Path: "/tmp/wt/other", RepoRoot: "/tmp/repo-other", Status: "active"},
	}}
	p := &fakePrompt{ok: true}

	s := NewCompleteService(g, r, p)
	_, err := s.Run(domain.CompleteInput{Name: "demo", Yes: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(g.removed) != 2 {
		t.Fatalf("expected root and package removals, got %v", g.removed)
	}
	if g.removed[0] != "/tmp/repo-root::"+rootPath {
		t.Fatalf("expected root removal first, got %v", g.removed)
	}
	if g.removed[1] != "/tmp/repo-core::"+packagePath {
		t.Fatalf("expected package removal second, got %v", g.removed)
	}

	if len(r.completed) != 2 {
		t.Fatalf("expected root and package completion, got %v", r.completed)
	}
	if r.completed[0] != "demo" || r.completed[1] != "demo__pkg__core" {
		t.Fatalf("unexpected completion order: %v", r.completed)
	}
	if _, err := os.Stat(container); !os.IsNotExist(err) {
		t.Fatalf("expected container removal, stat err=%v", err)
	}
}

func TestCompletePackageOnlyCompletesSelectedPackage(t *testing.T) {
	container, rootPath, packagePath := managedPaths(t, "demo")
	if err := os.MkdirAll(packagePath, 0o755); err != nil {
		t.Fatalf("failed to create package path: %v", err)
	}

	g := &fakeGit{}
	r := &fakeRegistry{records: []domain.RegistryRecord{
		{Name: "demo", Branch: "feature/demo", Path: rootPath, RepoRoot: "/tmp/repo-root", Status: "active"},
		{Name: "demo__pkg__core", Branch: "feature/demo-core", Path: packagePath, RepoRoot: "/tmp/repo-core", Status: "active"},
	}}
	p := &fakePrompt{ok: true}

	s := NewCompleteService(g, r, p)
	_, err := s.Run(domain.CompleteInput{Name: "demo__pkg__core", Yes: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(g.removed) != 1 || g.removed[0] != "/tmp/repo-core::"+packagePath {
		t.Fatalf("expected only package removal, got %v", g.removed)
	}
	if len(r.completed) != 1 || r.completed[0] != "demo__pkg__core" {
		t.Fatalf("expected only package completion, got %v", r.completed)
	}
	if _, err := os.Stat(container); err != nil {
		t.Fatalf("expected container to remain for package-only completion, err=%v", err)
	}
}

func TestCompleteMissingPathCleansStaleRegistryEntry(t *testing.T) {
	_, rootPath, _ := managedPaths(t, "stale")

	g := &fakeGit{}
	r := &fakeRegistry{records: []domain.RegistryRecord{{
		Name: "stale", Branch: "feature/stale", Path: rootPath, RepoRoot: "/tmp/repo", Status: "active",
	}}}
	p := &fakePrompt{ok: true}

	s := NewCompleteService(g, r, p)
	result, err := s.Run(domain.CompleteInput{Name: "stale", Yes: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.StaleCleaned {
		t.Fatalf("expected stale cleanup result")
	}
	if len(g.removed) != 0 {
		t.Fatalf("expected no git remove call for missing path, got %v", g.removed)
	}
	if len(r.completed) != 1 || r.completed[0] != "stale" {
		t.Fatalf("expected stale record completion, got %v", r.completed)
	}
}

func TestCompleteExistingPathDoesNotMarkStaleCleanup(t *testing.T) {
	_, rootPath, _ := managedPaths(t, "fresh")
	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		t.Fatalf("failed to create root path: %v", err)
	}

	g := &fakeGit{}
	r := &fakeRegistry{records: []domain.RegistryRecord{{
		Name: "fresh", Branch: "feature/fresh", Path: rootPath, RepoRoot: "/tmp/repo", Status: "active",
	}}}
	p := &fakePrompt{ok: true}

	s := NewCompleteService(g, r, p)
	result, err := s.Run(domain.CompleteInput{Name: "fresh", Yes: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StaleCleaned {
		t.Fatalf("did not expect stale cleanup for existing path")
	}
}

func TestCompleteRootRefusesContainerOutsideManagedScope(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	g := &fakeGit{}
	r := &fakeRegistry{records: []domain.RegistryRecord{{
		Name: "demo", Branch: "feature/demo", Path: "/tmp/external/demo/root/root-app", RepoRoot: "/tmp/repo", Status: "active",
	}}}
	p := &fakePrompt{ok: true}

	s := NewCompleteService(g, r, p)
	_, err := s.Run(domain.CompleteInput{Name: "demo", Yes: true})
	if err == nil {
		t.Fatalf("expected safety error for container outside destination root")
	}
	if !strings.Contains(err.Error(), "outside managed destination root") {
		t.Fatalf("unexpected error message: %v", err)
	}
	if len(g.removed) != 0 {
		t.Fatalf("expected no worktree removals on safety failure, got %v", g.removed)
	}
}

func TestCompleteRootRefusesUnexpectedRootPathLayout(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	g := &fakeGit{}
	r := &fakeRegistry{records: []domain.RegistryRecord{{
		Name: "demo", Branch: "feature/demo", Path: filepath.Join(destinationRoot(), "demo", "root-app"), RepoRoot: "/tmp/repo", Status: "active",
	}}}
	p := &fakePrompt{ok: true}

	s := NewCompleteService(g, r, p)
	_, err := s.Run(domain.CompleteInput{Name: "demo", Yes: true})
	if err == nil {
		t.Fatalf("expected path layout validation error")
	}
	if !strings.Contains(err.Error(), "Cannot determine worktree container") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestCompleteMarksMissingPathAsStaleCleanup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	missing := filepath.Join(destinationRoot(), "demo", "root", "root-app")
	g := &fakeGit{}
	r := &fakeRegistry{records: []domain.RegistryRecord{{
		Name: "demo", Branch: "feature/demo", Path: missing, RepoRoot: "/tmp/repo", Status: "active",
	}}}
	p := &fakePrompt{ok: true}

	s := NewCompleteService(g, r, p)
	result, err := s.Run(domain.CompleteInput{Name: "demo", Yes: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.StaleCleaned {
		t.Fatalf("expected stale cleanup marker")
	}
	if len(g.removed) != 0 {
		t.Fatalf("expected no git removal for missing path, got %v", g.removed)
	}
	if len(r.completed) != 1 || r.completed[0] != "demo" {
		t.Fatalf("expected registry completion for stale entry, got %v", r.completed)
	}
}

func TestCompleteExistingPathDoesNotSetStaleCleanup(t *testing.T) {
	_, rootPath, _ := managedPaths(t, "demo-no-stale")
	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		t.Fatalf("failed to create root path: %v", err)
	}

	g := &fakeGit{}
	r := &fakeRegistry{records: []domain.RegistryRecord{{
		Name: "demo-no-stale", Branch: "feature/demo-no-stale", Path: rootPath, RepoRoot: "/tmp/repo", Status: "active",
	}}}
	p := &fakePrompt{ok: true}

	s := NewCompleteService(g, r, p)
	result, err := s.Run(domain.CompleteInput{Name: "demo-no-stale", Yes: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StaleCleaned {
		t.Fatalf("did not expect stale cleanup marker")
	}
}

func TestListHidesPackageRowsAndAnnotatesRootCount(t *testing.T) {
	g := &fakeGit{worktrees: map[string][]domain.GitWorktreeEntry{}}
	r := &fakeRegistry{records: []domain.RegistryRecord{
		{Name: "demo", Branch: "feature/demo", Path: "/tmp/wt/demo", RepoRoot: "/tmp/repo-root", Status: "active"},
		{Name: "demo__pkg__core", Branch: "feature/demo-core", Path: "/tmp/wt/demo/packages/core", RepoRoot: "/tmp/repo-core", Status: "active"},
		{Name: "other", Branch: "feature/other", Path: "/tmp/wt/other", RepoRoot: "/tmp/repo-other", Status: "active"},
	}}

	s := NewListService(g, r)
	rows, err := s.Run(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected only root rows, got %d", len(rows))
	}

	if rows[0].Name == "demo" {
		if rows[0].PackageCount != 1 {
			t.Fatalf("expected package count for demo root, got %d", rows[0].PackageCount)
		}
		if rows[1].PackageCount != 0 {
			t.Fatalf("expected no package count for other root, got %d", rows[1].PackageCount)
		}
		return
	}

	if rows[1].Name != "demo" || rows[1].PackageCount != 1 {
		t.Fatalf("expected demo row with package count 1, got %+v", rows)
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
