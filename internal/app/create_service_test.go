package app

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/EndersonPro/flutree/internal/domain"
)

type fakeCreateGit struct {
	repos    []domain.DiscoveredFlutterRepo
	created  []string
	removed  []string
	dirty    bool
	worktree map[string][]domain.GitWorktreeEntry
}

func (f *fakeCreateGit) EnsureRepo() (string, error) { return "", nil }
func (f *fakeCreateGit) ListWorktrees(repoRoot string) ([]domain.GitWorktreeEntry, error) {
	return f.worktree[repoRoot], nil
}
func (f *fakeCreateGit) CreateWorktree(repoRoot, path, branch, baseBranch string) error {
	_ = os.MkdirAll(path, 0o755)
	f.created = append(f.created, strings.Join([]string{repoRoot, path, branch, baseBranch}, "::"))
	return nil
}
func (f *fakeCreateGit) RemoveWorktree(repoRoot, path string, force bool) error {
	f.removed = append(f.removed, repoRoot+"::"+path)
	return nil
}
func (f *fakeCreateGit) IsDirty(path string) (bool, error) { return f.dirty, nil }
func (f *fakeCreateGit) DiscoverFlutterRepos(scope string) ([]domain.DiscoveredFlutterRepo, error) {
	return f.repos, nil
}

type fakeCreatePrompt struct{}

func (f *fakeCreatePrompt) Confirm(message string, nonInteractive, assumeYes bool) (bool, error) {
	return true, nil
}
func (f *fakeCreatePrompt) ConfirmWithToken(message, token string, nonInteractive, assumeYes bool) (bool, error) {
	return true, nil
}
func (f *fakeCreatePrompt) SelectOne(message string, choices []string, nonInteractive bool) (string, error) {
	return choices[0], nil
}
func (f *fakeCreatePrompt) SelectPackages(message string, choices []string, nonInteractive bool) ([]string, error) {
	return choices, nil
}
func (f *fakeCreatePrompt) AskText(message, defaultValue string, nonInteractive bool) (string, error) {
	return defaultValue, nil
}

func TestBuildDryPlanBuildsRootAndPackagePlans(t *testing.T) {
	root := t.TempDir()
	repoRoot := filepath.Join(root, "root-app")
	repoPkg := filepath.Join(root, "core-pkg")
	g := &fakeCreateGit{
		repos: []domain.DiscoveredFlutterRepo{
			{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"},
			{Name: "core-pkg", RepoRoot: repoPkg, PackageName: "core"},
		},
	}
	svc := NewCreateService(g, &fakeRegistry{}, &fakeCreatePrompt{})
	name := "workspace-demo-" + strings.ReplaceAll(filepath.Base(root), "_", "-")
	_ = os.RemoveAll(filepath.Join(destinationRoot(), normalizeWorktreeName(name)))

	plan, err := svc.BuildDryPlan(domain.CreateInput{
		Name:              name,
		Branch:            "feature/workspace-demo",
		BaseBranch:        "main",
		ExecutionScope:    root,
		RootSelector:      "root-app",
		PackageSelectors:  []string{"core-pkg"},
		PackageBaseBranch: map[string]string{"core-pkg": "develop"},
		GenerateWorkspace: true,
		NonInteractive:    true,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if plan.Root.Repo.Name != "root-app" || len(plan.Packages) != 1 {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	wantPkgBranch := "feature/" + normalizeWorktreeName(name) + "-core-pkg"
	if plan.Packages[0].Branch != wantPkgBranch {
		t.Fatalf("unexpected package branch: %s", plan.Packages[0].Branch)
	}
	if !strings.Contains(plan.OverrideContent, "dependency_overrides:") || !strings.Contains(plan.OverrideContent, "core:") {
		t.Fatalf("unexpected override content: %s", plan.OverrideContent)
	}
}

func TestBuildDryPlanGeneratesPerPackageBranchFromWorkspaceName(t *testing.T) {
	root := t.TempDir()
	repoRoot := filepath.Join(root, "root-app")
	repoPkg := filepath.Join(root, "core-pkg")
	g := &fakeCreateGit{
		repos: []domain.DiscoveredFlutterRepo{
			{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"},
			{Name: "core-pkg", RepoRoot: repoPkg, PackageName: "core"},
		},
	}
	svc := NewCreateService(g, &fakeRegistry{}, &fakeCreatePrompt{})

	plan, err := svc.BuildDryPlan(domain.CreateInput{
		Name:              "Feature Login",
		Branch:            "feature/feature-login",
		BaseBranch:        "main",
		ExecutionScope:    root,
		RootSelector:      "root-app",
		PackageSelectors:  []string{"core-pkg"},
		PackageBaseBranch: map[string]string{"core-pkg": "develop"},
		GenerateWorkspace: false,
		NonInteractive:    true,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(plan.Packages) != 1 {
		t.Fatalf("unexpected package count: %d", len(plan.Packages))
	}
	wantPkgBranch := "feature/feature-login-core-pkg"
	if plan.Packages[0].Branch != wantPkgBranch {
		t.Fatalf("package branch mismatch. got=%s want=%s", plan.Packages[0].Branch, wantPkgBranch)
	}
	if plan.Packages[0].Branch == plan.Root.Branch {
		t.Fatalf("package branch must not reuse root branch. root=%s package=%s", plan.Root.Branch, plan.Packages[0].Branch)
	}
}

func TestBuildDryPlanKeepsSameSelectionsForInteractiveAndNonInteractiveInputs(t *testing.T) {
	root := t.TempDir()
	repoRoot := filepath.Join(root, "root-app")
	repoPkgA := filepath.Join(root, "core-pkg")
	repoPkgB := filepath.Join(root, "design-pkg")
	g := &fakeCreateGit{
		repos: []domain.DiscoveredFlutterRepo{
			{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"},
			{Name: "core-pkg", RepoRoot: repoPkgA, PackageName: "core"},
			{Name: "design-pkg", RepoRoot: repoPkgB, PackageName: "design"},
		},
	}
	svc := NewCreateService(g, &fakeRegistry{}, &fakeCreatePrompt{})

	baseInput := domain.CreateInput{
		Name:              "Feature Login",
		Branch:            "feature/feature-login",
		BaseBranch:        "main",
		ExecutionScope:    root,
		RootSelector:      "root-app",
		PackageSelectors:  []string{"core-pkg", "design-pkg"},
		PackageBaseBranch: map[string]string{"core-pkg": "develop", "design-pkg": "release"},
		GenerateWorkspace: true,
	}

	interactivePlan, err := svc.BuildDryPlan(withNonInteractive(baseInput, false))
	if err != nil {
		t.Fatalf("interactive-like input should build plan: %v", err)
	}
	nonInteractivePlan, err := svc.BuildDryPlan(withNonInteractive(baseInput, true))
	if err != nil {
		t.Fatalf("non-interactive input should build same plan: %v", err)
	}

	if interactivePlan.Root.Repo.Name != nonInteractivePlan.Root.Repo.Name {
		t.Fatalf("root selection mismatch. interactive=%s nonInteractive=%s", interactivePlan.Root.Repo.Name, nonInteractivePlan.Root.Repo.Name)
	}
	if interactivePlan.Root.Branch != nonInteractivePlan.Root.Branch {
		t.Fatalf("root branch mismatch. interactive=%s nonInteractive=%s", interactivePlan.Root.Branch, nonInteractivePlan.Root.Branch)
	}
	if !reflect.DeepEqual(planPackages(interactivePlan.Packages), planPackages(nonInteractivePlan.Packages)) {
		t.Fatalf("package selection mismatch. interactive=%v nonInteractive=%v", planPackages(interactivePlan.Packages), planPackages(nonInteractivePlan.Packages))
	}
	if interactivePlan.WorkspacePath != nonInteractivePlan.WorkspacePath {
		t.Fatalf("workspace path mismatch. interactive=%s nonInteractive=%s", interactivePlan.WorkspacePath, nonInteractivePlan.WorkspacePath)
	}
}

func withNonInteractive(input domain.CreateInput, nonInteractive bool) domain.CreateInput {
	input.NonInteractive = nonInteractive
	return input
}

func planPackages(packages []domain.PlannedWorktree) []string {
	out := make([]string, 0, len(packages))
	for _, pkg := range packages {
		out = append(out, strings.Join([]string{pkg.Repo.Name, pkg.Branch, pkg.BaseBranch}, "::"))
	}
	return out
}

func TestBuildWorkspaceFoldersKeepsRootFirstAndPackagesSorted(t *testing.T) {
	container := filepath.Join(string(filepath.Separator), "tmp", "worktrees", "feature-login")
	root := domain.PlannedWorktree{Path: filepath.Join(container, "root", "root-app")}
	packages := []domain.PlannedWorktree{
		{Path: filepath.Join(container, "packages", "zeta")},
		{Path: filepath.Join(container, "packages", "alpha")},
	}

	folders := buildWorkspaceFolders(root, packages, container)
	want := []string{"root/root-app", "packages/alpha", "packages/zeta"}

	if !reflect.DeepEqual(folders, want) {
		t.Fatalf("unexpected folders. got=%v want=%v", folders, want)
	}
}

func TestBuildWorkspaceFoldersFallsBackToAbsoluteRootPath(t *testing.T) {
	container := filepath.Join(string(filepath.Separator), "tmp", "worktrees", "feature-login")
	root := domain.PlannedWorktree{Path: filepath.Join(string(filepath.Separator), "another", "volume", "root", "root-app")}

	folders := buildWorkspaceFolders(root, nil, container)
	want := []string{filepath.ToSlash(root.Path)}

	if !reflect.DeepEqual(folders, want) {
		t.Fatalf("unexpected folders. got=%v want=%v", folders, want)
	}
}

func TestApplyAddsOverrideEntryToGitignoreWhenMissing(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoRoot := filepath.Join(root, "root-app")
	g := &fakeCreateGit{
		repos: []domain.DiscoveredFlutterRepo{{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"}},
	}
	svc := NewCreateService(g, &fakeRegistry{}, &fakeCreatePrompt{})

	name := "gitignore-missing-" + strings.ReplaceAll(filepath.Base(root), "_", "-")
	_ = os.RemoveAll(filepath.Join(destinationRoot(), normalizeWorktreeName(name)))

	plan, err := svc.BuildDryPlan(domain.CreateInput{
		Name:           name,
		Branch:         "feature/gitignore-missing",
		BaseBranch:     "main",
		ExecutionScope: root,
		RootSelector:   "root-app",
		NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	if err := os.MkdirAll(plan.Root.Path, 0o755); err != nil {
		t.Fatalf("failed to create root path: %v", err)
	}
	if err := os.WriteFile(filepath.Join(plan.Root.Path, ".gitignore"), []byte(".dart_tool/"), 0o644); err != nil {
		t.Fatalf("failed to write .gitignore fixture: %v", err)
	}

	if _, err := svc.Apply(plan); err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(plan.Root.Path, ".gitignore"))
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}
	got := string(b)
	want := ".dart_tool/\npubspec_overrides.yaml\n"
	if got != want {
		t.Fatalf("unexpected .gitignore content. got=%q want=%q", got, want)
	}
}

func TestApplyDoesNotDuplicateOverrideEntryInGitignore(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoRoot := filepath.Join(root, "root-app")
	g := &fakeCreateGit{
		repos: []domain.DiscoveredFlutterRepo{{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"}},
	}
	svc := NewCreateService(g, &fakeRegistry{}, &fakeCreatePrompt{})

	name := "gitignore-existing-" + strings.ReplaceAll(filepath.Base(root), "_", "-")
	_ = os.RemoveAll(filepath.Join(destinationRoot(), normalizeWorktreeName(name)))

	plan, err := svc.BuildDryPlan(domain.CreateInput{
		Name:           name,
		Branch:         "feature/gitignore-existing",
		BaseBranch:     "main",
		ExecutionScope: root,
		RootSelector:   "root-app",
		NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	if err := os.MkdirAll(plan.Root.Path, 0o755); err != nil {
		t.Fatalf("failed to create root path: %v", err)
	}
	initial := ".dart_tool/\npubspec_overrides.yaml\n"
	if err := os.WriteFile(filepath.Join(plan.Root.Path, ".gitignore"), []byte(initial), 0o644); err != nil {
		t.Fatalf("failed to write .gitignore fixture: %v", err)
	}

	if _, err := svc.Apply(plan); err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(plan.Root.Path, ".gitignore"))
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}
	got := string(b)
	if strings.Count(got, "pubspec_overrides.yaml") != 1 {
		t.Fatalf("expected single override entry, got=%q", got)
	}
	if got != initial {
		t.Fatalf("expected .gitignore to remain unchanged. got=%q want=%q", got, initial)
	}
}
