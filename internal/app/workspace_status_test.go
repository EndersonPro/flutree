package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/EndersonPro/flutree/internal/domain"
)

type inspectFakeGit struct {
	fakeGit
	repo domain.DiscoveredFlutterRepo
}

func (f *inspectFakeGit) InspectFlutterRepo(path string) (domain.DiscoveredFlutterRepo, bool, error) {
	return f.repo, f.repo.RepoRoot == path, nil
}
func (f *inspectFakeGit) DiscoverFlutterRepos(string) ([]domain.DiscoveredFlutterRepo, error) {
	return []domain.DiscoveredFlutterRepo{f.repo}, nil
}

type fakeSDK struct{ pins domain.SDKPins }

func (f fakeSDK) Pins(string, string) (domain.SDKPins, error) { return f.pins, nil }
func (f fakeSDK) Use(string, domain.SDKManager, string) error { return nil }

func TestInspectRepoReportsPathDependenciesAndPins(t *testing.T) {
	root := t.TempDir()
	pubspec := "name: app\ndependencies:\n  core:\n    path: ../core\n  http: ^1.0.0\n  git_dep:\n    git: https://example.com/x.git\ndev_dependencies:\n  tools:\n    path: ../tools\n"
	if err := os.WriteFile(filepath.Join(root, "pubspec.yaml"), []byte(pubspec), 0o644); err != nil {
		t.Fatal(err)
	}
	git := &inspectFakeGit{repo: domain.DiscoveredFlutterRepo{Name: "app", RepoRoot: root, PackageName: "app"}}
	svc := NewInspectService(git, fakeSDK{pins: domain.SDKPins{FVMVersion: "3.24.0"}}, &fakeRegistry{})

	got, err := svc.InspectRepo(root)
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsFlutter || got.PackageName != "app" || got.CurrentBranch != "main" || got.SDK.FVMVersion != "3.24.0" {
		t.Fatalf("inspection = %+v", got)
	}
	if len(got.PathDependencies) != 2 || got.PathDependencies["core"] != "../core" || got.PathDependencies["tools"] != "../tools" {
		t.Fatalf("path dependencies = %v (only path: specs belong here)", got.PathDependencies)
	}
}

func TestInspectRepoNotFlutterAndMissingPath(t *testing.T) {
	svc := NewInspectService(&inspectFakeGit{}, nil, &fakeRegistry{})
	got, err := svc.InspectRepo(t.TempDir())
	if err != nil || got.IsFlutter || got.Reason == "" {
		t.Fatalf("plain folder = %+v, %v", got, err)
	}
	if _, err := svc.InspectRepo(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("a missing path must be an error")
	}
	if _, err := svc.InspectRepo(" "); err == nil {
		t.Fatal("an empty path must be an error")
	}
}

func TestWorkspaceStatusReportsDirtyMissingAndOverrides(t *testing.T) {
	container := t.TempDir()
	rootPath := filepath.Join(container, "root", "app")
	pkgPath := filepath.Join(container, "packages", "core")
	gonePath := filepath.Join(container, "packages", "gone")
	for _, p := range []string{rootPath, pkgPath} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(rootPath, overrideFileName), []byte("dependency_overrides:\n  core:\n    path: ../../packages/core\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := &fakeRegistry{records: []domain.RegistryRecord{
		{Name: "demo", Branch: "feature/demo", Path: rootPath, RepoRoot: "/src/app", Status: "active"},
		{Name: "demo__pkg__gone", Branch: "feature/demo", Path: gonePath, RepoRoot: "/src/gone", Status: "active"},
		{Name: "demo__pkg__core", Branch: "feature/demo", Path: pkgPath, RepoRoot: "/src/core", Status: "active"},
		{Name: "other", Path: "/x", Status: "active"},
	}}
	git := &inspectFakeGit{fakeGit: fakeGit{dirtyByPath: map[string]bool{pkgPath: true}}}
	svc := NewInspectService(git, nil, reg)

	got, err := svc.WorkspaceStatus("demo")
	if err != nil {
		t.Fatal(err)
	}
	if got.Root.Dirty || !got.Root.Exists || got.Root.Role != "root" || got.ContainerPath != container {
		t.Fatalf("root = %+v container=%s", got.Root, got.ContainerPath)
	}
	if len(got.Packages) != 2 || got.Packages[0].Name != "core" || !got.Packages[0].Dirty || got.Packages[1].Name != "gone" || got.Packages[1].Exists || got.Packages[1].Dirty {
		t.Fatalf("packages = %+v", got.Packages)
	}
	if !got.Overrides.Exists || got.Overrides.Entries["core"] == nil {
		t.Fatalf("overrides = %+v", got.Overrides)
	}
	if _, err := svc.WorkspaceStatus("missing"); err == nil {
		t.Fatal("unknown workspace must be an error")
	}
}

func TestDiscoverReposIncludesCurrentBranch(t *testing.T) {
	git := &inspectFakeGit{repo: domain.DiscoveredFlutterRepo{Name: "app", RepoRoot: "/r/app", PackageName: "app"}}
	got, err := NewInspectService(git, nil, &fakeRegistry{}).DiscoverRepos("/r")
	if err != nil || len(got) != 1 || got[0].CurrentBranch != "main" || got[0].RepoRoot != "/r/app" {
		t.Fatalf("repos = %+v, %v", got, err)
	}
	if _, err := NewInspectService(git, nil, &fakeRegistry{}).DiscoverRepos(""); err == nil {
		t.Fatal("empty scope must be an error")
	}
}

func TestWorkspaceStatusReportsAheadBehindAgainstStoredBase(t *testing.T) {
	container := t.TempDir()
	rootPath := filepath.Join(container, "root", "app")
	pkgPath := filepath.Join(container, "packages", "core")
	oldPath := filepath.Join(container, "packages", "old")
	for _, p := range []string{rootPath, pkgPath, oldPath} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	reg := &fakeRegistry{records: []domain.RegistryRecord{
		{Name: "ws", Branch: "feature/ws", Path: rootPath, RepoRoot: "/src/app", Status: "active", BaseBranch: "main"},
		{Name: "ws__pkg__core", Branch: "feature/ws", Path: pkgPath, RepoRoot: "/src/core", Status: "active", BaseBranch: "gone"},
		{Name: "ws__pkg__old", Branch: "feature/ws", Path: oldPath, RepoRoot: "/src/old", Status: "active"},
	}}
	git := &fakeGit{aheadBehind: map[string][2]int{rootPath: {2, 1}}}

	got, err := NewInspectService(git, nil, reg).WorkspaceStatus("ws")
	if err != nil {
		t.Fatal(err)
	}
	if got.Root.BaseBranch != "main" || got.Root.Ahead == nil || *got.Root.Ahead != 2 || got.Root.Behind == nil || *got.Root.Behind != 1 || got.Root.AheadBehindNote != "" {
		t.Fatalf("root = %+v, want base main ahead 2 behind 1", got.Root)
	}
	byName := map[string]domain.WorktreeStatus{}
	for _, pkg := range got.Packages {
		byName[pkg.Name] = pkg
	}
	if pkg := byName["core"]; pkg.Ahead != nil || pkg.Behind != nil || pkg.AheadBehindNote == "" || pkg.BaseBranch != "gone" {
		t.Errorf("an unresolvable base must omit counts and carry a note: %+v", pkg)
	}
	if pkg := byName["old"]; pkg.Ahead != nil || pkg.Behind != nil || pkg.AheadBehindNote == "" || pkg.BaseBranch != "" {
		t.Errorf("a record without base must omit counts and carry a note: %+v", pkg)
	}
}
