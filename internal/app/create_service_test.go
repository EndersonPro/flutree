package app

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/EndersonPro/flutree/internal/domain"
)

type fakeCreateGit struct {
	repos              []domain.DiscoveredFlutterRepo
	created            []string
	createdExisting    []string
	removed            []string
	dirty              bool
	worktree           map[string][]domain.GitWorktreeEntry
	branchExistsByID   map[string]bool
	remoteBranchExists map[string]bool
	syncBranchCalls    []string
	syncBranchErr      error
	syncCalls          []string
	syncErr            error
	remoteBranchErr    error
	// missingBases lists "<repoRoot>::<branch>" base branches that do not exist.
	missingBases  map[string]bool
	deleted       []string
	inspectable   map[string]domain.DiscoveredFlutterRepo
	failCreateAt  string
	discoverCalls int
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
func (f *fakeCreateGit) CreateWorktreeNew(repoRoot, path, branch, startPoint string) error {
	if f.failCreateAt != "" && strings.HasSuffix(path, f.failCreateAt) {
		return errors.New("boom")
	}
	_ = os.MkdirAll(path, 0o755)
	f.created = append(f.created, strings.Join([]string{"new", repoRoot, path, branch, startPoint}, "::"))
	return nil
}
func (f *fakeCreateGit) CreateWorktreeExisting(repoRoot, path, branch string) error {
	_ = os.MkdirAll(path, 0o755)
	f.createdExisting = append(f.createdExisting, strings.Join([]string{repoRoot, path, branch}, "::"))
	return nil
}
func (f *fakeCreateGit) BranchExists(repoRoot, branch string) (bool, error) {
	if f.branchExistsByID == nil {
		return false, nil
	}
	return f.branchExistsByID[repoRoot+"::"+branch], nil
}
func (f *fakeCreateGit) RemoteBranchExists(repoRoot, branch string) (bool, error) {
	if f.remoteBranchErr != nil {
		return false, f.remoteBranchErr
	}
	if f.remoteBranchExists == nil {
		return false, nil
	}
	return f.remoteBranchExists[repoRoot+"::"+branch], nil
}
func (f *fakeCreateGit) SyncBranchWithRemote(repoRoot, branch string) error {
	f.syncBranchCalls = append(f.syncBranchCalls, repoRoot+"::"+branch)
	if f.syncBranchErr != nil {
		return f.syncBranchErr
	}
	return nil
}
func (f *fakeCreateGit) SyncBaseBranch(repoRoot, baseBranch string) (string, error) {
	f.syncCalls = append(f.syncCalls, repoRoot+"::"+baseBranch)
	if f.syncErr != nil {
		return "", f.syncErr
	}
	return "origin/" + baseBranch, nil
}
func (f *fakeCreateGit) PruneWorktrees(string) error { return nil }
func (f *fakeCreateGit) RemoveWorktree(repoRoot, path string, force bool) error {
	f.removed = append(f.removed, repoRoot+"::"+path)
	return nil
}
func (f *fakeCreateGit) IsDirty(path string) (bool, error)       { return f.dirty, nil }
func (f *fakeCreateGit) EnsureLocalExclude(string, string) error { return nil }
func (f *fakeCreateGit) DiscoverFlutterRepos(scope string) ([]domain.DiscoveredFlutterRepo, error) {
	f.discoverCalls++
	return f.repos, nil
}
func (f *fakeCreateGit) InspectFlutterRepo(path string) (domain.DiscoveredFlutterRepo, bool, error) {
	repo, ok := f.inspectable[path]
	return repo, ok, nil
}
func (f *fakeCreateGit) BaseBranchExists(repoRoot, branch string) (bool, error) {
	return !f.missingBases[repoRoot+"::"+branch], nil
}
func (f *fakeCreateGit) DeleteBranch(repoRoot, branch string) error {
	f.deleted = append(f.deleted, repoRoot+"::"+branch)
	return nil
}

type fakeCreatePrompt struct {
	forceDecline bool
	askTextCalls int
}

func (f *fakeCreatePrompt) Confirm(message string, nonInteractive, assumeYes bool) (bool, error) {
	if f.forceDecline {
		return false, nil
	}
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
	f.askTextCalls++
	return defaultValue, nil
}

func TestBuildDryPlanNoPackageCreatesRootOnlyPlan(t *testing.T) {
	root := t.TempDir()
	repoRoot := filepath.Join(root, "root-app")
	repoPkg := filepath.Join(root, "core-pkg")
	g := &fakeCreateGit{
		repos: []domain.DiscoveredFlutterRepo{
			{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"},
			{Name: "core-pkg", RepoRoot: repoPkg, PackageName: "core"},
		},
	}
	p := &fakeCreatePrompt{}
	svc := NewCreateService(g, &fakeRegistry{}, p)

	plan, err := svc.BuildDryPlan(domain.CreateInput{
		Name:           "root-only",
		Branch:         "feature/root-only",
		BaseBranch:     "main",
		ExecutionScope: root,
		RootSelector:   "root-app",
		NoPackage:      true,
		NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(plan.Packages) != 0 {
		t.Fatalf("expected no package plans in no-package mode, got %d", len(plan.Packages))
	}
	if plan.OverridePath != "" || plan.OverrideContent != "" {
		t.Fatalf("expected no override file in no-package mode, got path=%q content=%q", plan.OverridePath, plan.OverrideContent)
	}
	if p.askTextCalls != 0 {
		t.Fatalf("expected no package base prompts in no-package mode, got %d", p.askTextCalls)
	}
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
	wantPkgBranch := "feature/workspace-demo"
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
	wantPkgBranch := "feature/feature-login"
	if plan.Packages[0].Branch != wantPkgBranch {
		t.Fatalf("package branch mismatch. got=%s want=%s", plan.Packages[0].Branch, wantPkgBranch)
	}
	if plan.Packages[0].Branch != plan.Root.Branch {
		t.Fatalf("package branch must match root branch. root=%s package=%s", plan.Root.Branch, plan.Packages[0].Branch)
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

func TestBuildDryPlanInteractiveWithoutPackageSelectorsUsesPromptSelection(t *testing.T) {
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
	plan, err := svc.BuildDryPlan(domain.CreateInput{
		Name:           "Feature Login",
		Branch:         "feature/feature-login",
		BaseBranch:     "main",
		ExecutionScope: root,
		RootSelector:   "root-app",
		NonInteractive: false,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if len(plan.Packages) != 2 {
		t.Fatalf("expected prompt-selected packages, got %d", len(plan.Packages))
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

func TestApplyFailsInNonInteractiveModeWhenExistingBranchReuseNotExplicit(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoRoot := filepath.Join(root, "root-app")
	g := &fakeCreateGit{
		repos: []domain.DiscoveredFlutterRepo{{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"}},
		branchExistsByID: map[string]bool{
			repoRoot + "::feature/reuse-me": true,
		},
	}
	svc := NewCreateService(g, &fakeRegistry{}, &fakeCreatePrompt{})

	plan, err := svc.BuildDryPlan(domain.CreateInput{
		Name:           "reuse-me",
		Branch:         "feature/reuse-me",
		BaseBranch:     "main",
		ExecutionScope: root,
		RootSelector:   "root-app",
		NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	_, err = svc.Apply(plan, domain.CreateApplyOptions{NonInteractive: true, ReuseExistingBranch: false})
	if err == nil {
		t.Fatalf("expected branch reuse confirmation error")
	}
	if !strings.Contains(err.Error(), "Existing branch reuse requires explicit --reuse-existing-branch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyReusesExistingBranchWhenUserConfirms(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoRoot := filepath.Join(root, "root-app")
	g := &fakeCreateGit{
		repos: []domain.DiscoveredFlutterRepo{{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"}},
		branchExistsByID: map[string]bool{
			repoRoot + "::feature/reuse-me": true,
		},
	}
	p := &fakeCreatePrompt{}
	svc := NewCreateService(g, &fakeRegistry{}, p)

	plan, err := svc.BuildDryPlan(domain.CreateInput{
		Name:           "reuse-me",
		Branch:         "feature/reuse-me",
		BaseBranch:     "main",
		ExecutionScope: root,
		RootSelector:   "root-app",
	})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	if _, err := svc.Apply(plan, domain.CreateApplyOptions{}); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if len(g.createdExisting) != 1 {
		t.Fatalf("expected existing-branch worktree path, got %+v", g.createdExisting)
	}
	if len(g.syncCalls) != 0 {
		t.Fatalf("expected no base sync for existing branch reuse, got %+v", g.syncCalls)
	}
}

func TestApplyCancelsWhenUserDeclinesExistingBranchReuse(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoRoot := filepath.Join(root, "root-app")
	g := &fakeCreateGit{
		repos: []domain.DiscoveredFlutterRepo{{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"}},
		branchExistsByID: map[string]bool{
			repoRoot + "::feature/reuse-no": true,
		},
	}
	p := &fakeCreatePrompt{forceDecline: true}
	svc := NewCreateService(g, &fakeRegistry{}, p)

	plan, err := svc.BuildDryPlan(domain.CreateInput{
		Name:           "reuse-no",
		Branch:         "feature/reuse-no",
		BaseBranch:     "main",
		ExecutionScope: root,
		RootSelector:   "root-app",
	})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	_, err = svc.Apply(plan, domain.CreateApplyOptions{})
	if err == nil {
		t.Fatalf("expected cancellation error")
	}
	if !strings.Contains(err.Error(), "Create cancelled by user while confirming branch reuse") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplySyncsBaseBranchBeforeNewBranchCreation(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoRoot := filepath.Join(root, "root-app")
	g := &fakeCreateGit{
		repos: []domain.DiscoveredFlutterRepo{{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"}},
	}
	svc := NewCreateService(g, &fakeRegistry{}, &fakeCreatePrompt{})

	plan, err := svc.BuildDryPlan(domain.CreateInput{
		Name:           "sync-order",
		Branch:         "feature/sync-order",
		BaseBranch:     "main",
		ExecutionScope: root,
		RootSelector:   "root-app",
		NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	if _, err := svc.Apply(plan, domain.CreateApplyOptions{NonInteractive: true, SyncWithRemote: true}); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if len(g.syncCalls) == 0 {
		t.Fatalf("expected base sync call before worktree create")
	}
	if len(g.created) == 0 {
		t.Fatalf("expected new worktree creation call")
	}
}

func TestBuildDryPlanUsesDefaultBranchWhenBranchOmitted(t *testing.T) {
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
	if plan.Root.Branch != "feature/feature-login" {
		t.Fatalf("unexpected default root branch: %s", plan.Root.Branch)
	}
	if plan.Packages[0].Branch != "feature/feature-login" {
		t.Fatalf("unexpected default package branch: %s", plan.Packages[0].Branch)
	}
}

func TestApplyNonInteractiveRequiresExplicitReuseForExistingBranch(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoRoot := filepath.Join(root, "root-app")
	g := &fakeCreateGit{
		repos: []domain.DiscoveredFlutterRepo{{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"}},
		branchExistsByID: map[string]bool{
			repoRoot + "::feature/existing": true,
		},
	}
	svc := NewCreateService(g, &fakeRegistry{}, &fakeCreatePrompt{})

	name := "existing-branch-" + strings.ReplaceAll(filepath.Base(root), "_", "-")
	_ = os.RemoveAll(filepath.Join(destinationRoot(), normalizeWorktreeName(name)))

	plan, err := svc.BuildDryPlan(domain.CreateInput{
		Name:           name,
		Branch:         "feature/existing",
		BaseBranch:     "main",
		ExecutionScope: root,
		RootSelector:   "root-app",
		NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	_, err = svc.Apply(plan, domain.CreateApplyOptions{NonInteractive: true})
	if err == nil {
		t.Fatalf("expected non-interactive reuse error")
	}
	if !strings.Contains(err.Error(), "Existing branch reuse requires explicit --reuse-existing-branch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyReusesExistingBranchWhenAllowed(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoRoot := filepath.Join(root, "root-app")
	g := &fakeCreateGit{
		repos: []domain.DiscoveredFlutterRepo{{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"}},
		branchExistsByID: map[string]bool{
			repoRoot + "::feature/existing": true,
		},
	}
	svc := NewCreateService(g, &fakeRegistry{}, &fakeCreatePrompt{})

	name := "existing-branch-ok-" + strings.ReplaceAll(filepath.Base(root), "_", "-")
	_ = os.RemoveAll(filepath.Join(destinationRoot(), normalizeWorktreeName(name)))

	plan, err := svc.BuildDryPlan(domain.CreateInput{
		Name:           name,
		Branch:         "feature/existing",
		BaseBranch:     "main",
		ExecutionScope: root,
		RootSelector:   "root-app",
		NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	if _, err := svc.Apply(plan, domain.CreateApplyOptions{NonInteractive: true, ReuseExistingBranch: true}); err != nil {
		t.Fatalf("expected reuse to pass, got: %v", err)
	}
	if len(g.createdExisting) != 1 {
		t.Fatalf("expected existing branch create path, got=%v", g.createdExisting)
	}
	if len(g.syncCalls) != 0 {
		t.Fatalf("sync should not run when branch already exists, got=%v", g.syncCalls)
	}
}

func TestApplyOptionSyncWithRemoteSyncsExistingBranchBeforeReuse(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoRoot := filepath.Join(root, "root-app")
	g := &fakeCreateGit{
		repos: []domain.DiscoveredFlutterRepo{{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"}},
		branchExistsByID: map[string]bool{
			repoRoot + "::feature/existing": true,
		},
	}
	svc := NewCreateService(g, &fakeRegistry{}, &fakeCreatePrompt{})

	name := "existing-branch-sync-" + strings.ReplaceAll(filepath.Base(root), "_", "-")
	_ = os.RemoveAll(filepath.Join(destinationRoot(), normalizeWorktreeName(name)))

	plan, err := svc.BuildDryPlan(domain.CreateInput{
		Name:           name,
		Branch:         "feature/existing",
		BaseBranch:     "main",
		ExecutionScope: root,
		RootSelector:   "root-app",
		NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	if _, err := svc.Apply(plan, domain.CreateApplyOptions{
		NonInteractive:      true,
		ReuseExistingBranch: true,
		SyncWithRemote:      true,
	}); err != nil {
		t.Fatalf("expected reuse with sync to pass, got: %v", err)
	}

	if len(g.syncBranchCalls) != 1 || g.syncBranchCalls[0] != repoRoot+"::feature/existing" {
		t.Fatalf("expected branch sync call, got=%v", g.syncBranchCalls)
	}
	if len(g.createdExisting) != 1 {
		t.Fatalf("expected existing branch create path, got=%v", g.createdExisting)
	}
}

func TestApplyCreatesNewBranchFromLocalBaseWhenSyncDisabled(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoRoot := filepath.Join(root, "root-app")
	g := &fakeCreateGit{
		repos: []domain.DiscoveredFlutterRepo{{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"}},
	}
	svc := NewCreateService(g, &fakeRegistry{}, &fakeCreatePrompt{})

	name := "local-base-create-" + strings.ReplaceAll(filepath.Base(root), "_", "-")
	_ = os.RemoveAll(filepath.Join(destinationRoot(), normalizeWorktreeName(name)))

	plan, err := svc.BuildDryPlan(domain.CreateInput{
		Name:           name,
		Branch:         "feature/new-branch",
		BaseBranch:     "main",
		ExecutionScope: root,
		RootSelector:   "root-app",
		NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	if _, err := svc.Apply(plan, domain.CreateApplyOptions{NonInteractive: true}); err != nil {
		t.Fatalf("expected apply success: %v", err)
	}
	if len(g.syncCalls) != 0 {
		t.Fatalf("expected no base sync when sync is disabled, got=%v", g.syncCalls)
	}
	if len(g.created) != 1 || !strings.HasPrefix(g.created[0], "new::") {
		t.Fatalf("expected new branch create path, got=%v", g.created)
	}
	if !strings.HasSuffix(g.created[0], "::main") {
		t.Fatalf("expected local base branch start point, got=%v", g.created)
	}
}

func TestApplySyncsBaseBeforeCreatingNewBranchWhenRequested(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoRoot := filepath.Join(root, "root-app")
	g := &fakeCreateGit{
		repos: []domain.DiscoveredFlutterRepo{{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"}},
	}
	svc := NewCreateService(g, &fakeRegistry{}, &fakeCreatePrompt{})

	name := "sync-before-create-" + strings.ReplaceAll(filepath.Base(root), "_", "-")
	_ = os.RemoveAll(filepath.Join(destinationRoot(), normalizeWorktreeName(name)))

	plan, err := svc.BuildDryPlan(domain.CreateInput{
		Name:           name,
		Branch:         "feature/new-branch",
		BaseBranch:     "main",
		ExecutionScope: root,
		RootSelector:   "root-app",
		NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	if _, err := svc.Apply(plan, domain.CreateApplyOptions{NonInteractive: true, SyncWithRemote: true}); err != nil {
		t.Fatalf("expected apply success: %v", err)
	}
	if len(g.syncCalls) != 1 || g.syncCalls[0] != repoRoot+"::main" {
		t.Fatalf("expected one sync before create, got=%v", g.syncCalls)
	}
	if len(g.created) != 1 || !strings.HasPrefix(g.created[0], "new::") {
		t.Fatalf("expected new branch create path, got=%v", g.created)
	}
}

func TestApplyReturnsErrorAndSkipsWorktreeCreationWhenSyncFails(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoRoot := filepath.Join(root, "root-app")
	g := &fakeCreateGit{
		repos:   []domain.DiscoveredFlutterRepo{{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"}},
		syncErr: errors.New("sync failed"),
	}
	svc := NewCreateService(g, &fakeRegistry{}, &fakeCreatePrompt{})

	name := "sync-fail-" + strings.ReplaceAll(filepath.Base(root), "_", "-")
	_ = os.RemoveAll(filepath.Join(destinationRoot(), normalizeWorktreeName(name)))

	plan, err := svc.BuildDryPlan(domain.CreateInput{
		Name:           name,
		Branch:         "feature/sync-fail",
		BaseBranch:     "main",
		ExecutionScope: root,
		RootSelector:   "root-app",
		NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	_, err = svc.Apply(plan, domain.CreateApplyOptions{NonInteractive: true, SyncWithRemote: true})
	if err == nil {
		t.Fatalf("expected apply to fail when base branch sync fails")
	}
	if len(g.created) != 0 {
		t.Fatalf("expected no new worktree creation when sync fails, got=%v", g.created)
	}
	if len(g.createdExisting) != 0 {
		t.Fatalf("expected no existing-branch worktree creation when sync fails, got=%v", g.createdExisting)
	}
}

func TestApplyFetchesRemoteBranchWhenExistsOnOriginButNotLocally(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoRoot := filepath.Join(root, "root-app")
	g := &fakeCreateGit{
		repos: []domain.DiscoveredFlutterRepo{{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"}},
		remoteBranchExists: map[string]bool{
			repoRoot + "::feature/remote-branch": true,
		},
	}
	svc := NewCreateService(g, &fakeRegistry{}, &fakeCreatePrompt{})

	name := "remote-branch-fetch-" + strings.ReplaceAll(filepath.Base(root), "_", "-")
	_ = os.RemoveAll(filepath.Join(destinationRoot(), normalizeWorktreeName(name)))

	plan, err := svc.BuildDryPlan(domain.CreateInput{
		Name:           name,
		Branch:         "feature/remote-branch",
		BaseBranch:     "main",
		ExecutionScope: root,
		RootSelector:   "root-app",
		NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	if _, err := svc.Apply(plan, domain.CreateApplyOptions{NonInteractive: true, SyncWithRemote: true}); err != nil {
		t.Fatalf("expected apply success: %v", err)
	}
	// Should have called SyncBranchWithRemote for the target branch (not just base sync)
	if len(g.syncBranchCalls) != 1 || g.syncBranchCalls[0] != repoRoot+"::feature/remote-branch" {
		t.Fatalf("expected branch sync call for remote branch, got=%v", g.syncBranchCalls)
	}
	// Should use CreateWorktreeExisting (branch now exists locally after sync)
	if len(g.createdExisting) != 1 {
		t.Fatalf("expected existing branch create path, got=%v", g.createdExisting)
	}
	// Should NOT have called base branch sync
	if len(g.syncCalls) != 0 {
		t.Fatalf("expected no base sync when remote branch exists, got=%v", g.syncCalls)
	}
	// Should NOT have called CreateWorktreeNew
	if len(g.created) != 0 {
		t.Fatalf("expected no new branch creation when remote branch exists, got=%v", g.created)
	}
}

func TestApplyFallsBackToNewBranchWhenRemoteBranchDoesNotExist(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoRoot := filepath.Join(root, "root-app")
	g := &fakeCreateGit{
		repos: []domain.DiscoveredFlutterRepo{{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"}},
		// remoteBranchExists is nil — means no remote branches exist
	}
	svc := NewCreateService(g, &fakeRegistry{}, &fakeCreatePrompt{})

	name := "no-remote-fallback-" + strings.ReplaceAll(filepath.Base(root), "_", "-")
	_ = os.RemoveAll(filepath.Join(destinationRoot(), normalizeWorktreeName(name)))

	plan, err := svc.BuildDryPlan(domain.CreateInput{
		Name:           name,
		Branch:         "feature/new-from-base",
		BaseBranch:     "main",
		ExecutionScope: root,
		RootSelector:   "root-app",
		NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	if _, err := svc.Apply(plan, domain.CreateApplyOptions{NonInteractive: true, SyncWithRemote: true}); err != nil {
		t.Fatalf("expected apply success: %v", err)
	}
	// Should have synced base branch
	if len(g.syncCalls) != 1 || g.syncCalls[0] != repoRoot+"::main" {
		t.Fatalf("expected base sync call, got=%v", g.syncCalls)
	}
	// Should have created new branch from base
	if len(g.created) != 1 || !strings.HasPrefix(g.created[0], "new::") {
		t.Fatalf("expected new branch create path, got=%v", g.created)
	}
	// Should NOT have called SyncBranchWithRemote for the target branch
	if len(g.syncBranchCalls) != 0 {
		t.Fatalf("expected no target branch sync when remote does not exist, got=%v", g.syncBranchCalls)
	}
}

func TestApplySkipsRemoteCheckWhenSyncWithRemoteIsDisabled(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoRoot := filepath.Join(root, "root-app")
	g := &fakeCreateGit{
		repos: []domain.DiscoveredFlutterRepo{{Name: "root-app", RepoRoot: repoRoot, PackageName: "root_app"}},
		remoteBranchExists: map[string]bool{
			repoRoot + "::feature/some-branch": true,
		},
	}
	svc := NewCreateService(g, &fakeRegistry{}, &fakeCreatePrompt{})

	name := "no-sync-skip-remote-" + strings.ReplaceAll(filepath.Base(root), "_", "-")
	_ = os.RemoveAll(filepath.Join(destinationRoot(), normalizeWorktreeName(name)))

	plan, err := svc.BuildDryPlan(domain.CreateInput{
		Name:           name,
		Branch:         "feature/some-branch",
		BaseBranch:     "main",
		ExecutionScope: root,
		RootSelector:   "root-app",
		NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}

	// SyncWithRemote: false — should NOT check remote at all
	if _, err := svc.Apply(plan, domain.CreateApplyOptions{NonInteractive: true}); err != nil {
		t.Fatalf("expected apply success: %v", err)
	}
	// Should NOT have checked remote branch existence
	// Should have created new branch from local base
	if len(g.created) != 1 || !strings.HasPrefix(g.created[0], "new::") {
		t.Fatalf("expected new branch create path, got=%v", g.created)
	}
	if !strings.HasSuffix(g.created[0], "::main") {
		t.Fatalf("expected local base branch start point, got=%v", g.created)
	}
	if len(g.syncBranchCalls) != 0 {
		t.Fatalf("expected no target branch sync when sync disabled, got=%v", g.syncBranchCalls)
	}
	if len(g.syncCalls) != 0 {
		t.Fatalf("expected no base sync when sync disabled, got=%v", g.syncCalls)
	}
}

func TestBuildDryPlanPackageBranchUsesExactExplicitOrDefaultRootBranch(t *testing.T) {
	tests := []struct {
		name       string
		branch     string
		wantBranch string
	}{
		{name: "explicit branch", branch: "release/2.3.0", wantBranch: "release/2.3.0"},
		{name: "default branch", branch: "", wantBranch: "feature/feature-login"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
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
				Branch:            tc.branch,
				BaseBranch:        "main",
				ExecutionScope:    root,
				RootSelector:      "root-app",
				PackageSelectors:  []string{"core-pkg"},
				PackageBaseBranch: map[string]string{"core-pkg": "develop"},
				NonInteractive:    true,
			})
			if err != nil {
				t.Fatalf("build plan failed: %v", err)
			}
			if plan.Root.Branch != tc.wantBranch {
				t.Fatalf("root branch mismatch. got=%s want=%s", plan.Root.Branch, tc.wantBranch)
			}
			if len(plan.Packages) != 1 || plan.Packages[0].Branch != tc.wantBranch {
				t.Fatalf("package branch mismatch. got=%+v want=%s", plan.Packages, tc.wantBranch)
			}
		})
	}
}

func TestBuildDryPlanUsesExplicitReposInsteadOfDiscovery(t *testing.T) {
	root := t.TempDir()
	repos := []domain.DiscoveredFlutterRepo{
		{Name: "root-app", RepoRoot: filepath.Join(root, "a", "root-app"), PackageName: "root_app"},
		{Name: "pkg-x", RepoRoot: filepath.Join(root, "b", "pkg-x"), PackageName: "pkg_x"},
	}
	g := &fakeCreateGit{}
	svc := NewCreateService(g, &fakeRegistry{}, &fakeCreatePrompt{})

	plan, err := svc.BuildDryPlan(domain.CreateInput{
		Name: "explicit", BaseBranch: "main", ExecutionScope: root, Repos: repos,
		RootSelector: "root-app", PackageSelectors: []string{"pkg-x"},
		PackageBaseBranch: map[string]string{"pkg-x": "main"}, NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if g.discoverCalls != 0 {
		t.Fatalf("explicit repos must replace discovery, got %d discover calls", g.discoverCalls)
	}
	if len(plan.Packages) != 1 || plan.Packages[0].Repo.Name != "pkg-x" {
		t.Fatalf("unexpected packages: %+v", plan.Packages)
	}
}

func TestBuildDryPlanAddsPathSelectorOutsideScope(t *testing.T) {
	scopeRepo := domain.DiscoveredFlutterRepo{Name: "root-app", RepoRoot: "/scope/root-app", PackageName: "root_app"}
	outside := domain.DiscoveredFlutterRepo{Name: "pkg-x", RepoRoot: "/elsewhere/pkg-x", PackageName: "pkg_x"}
	g := &fakeCreateGit{repos: []domain.DiscoveredFlutterRepo{scopeRepo}, inspectable: map[string]domain.DiscoveredFlutterRepo{"/elsewhere/pkg-x": outside}}
	svc := NewCreateService(g, &fakeRegistry{}, &fakeCreatePrompt{})

	plan, err := svc.BuildDryPlan(domain.CreateInput{
		Name: "outside", BaseBranch: "main", ExecutionScope: "/scope", RootSelector: "root-app",
		PackageSelectors: []string{"/elsewhere/pkg-x"}, PackageBaseBranch: map[string]string{"pkg-x": "main"}, NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(plan.Packages) != 1 || plan.Packages[0].Repo.RepoRoot != "/elsewhere/pkg-x" {
		t.Fatalf("expected the outside repo as package, got %+v", plan.Packages)
	}
}

func TestBuildDryPlanFailsWhenPackageBaseBranchIsMissing(t *testing.T) {
	root := t.TempDir()
	repos := []domain.DiscoveredFlutterRepo{
		{Name: "root-app", RepoRoot: filepath.Join(root, "root-app"), PackageName: "root_app"},
		{Name: "core", RepoRoot: filepath.Join(root, "core"), PackageName: "core"},
	}
	g := &fakeCreateGit{repos: repos, missingBases: map[string]bool{filepath.Join(root, "core") + "::develop": true}}
	svc := NewCreateService(g, &fakeRegistry{}, &fakeCreatePrompt{})

	_, err := svc.BuildDryPlan(domain.CreateInput{
		Name: "nobase", BaseBranch: "main", ExecutionScope: root, RootSelector: "root-app",
		PackageSelectors: []string{"core"}, NonInteractive: true,
	})
	var flutreeErr *domain.AppError
	if !errors.As(err, &flutreeErr) || flutreeErr.Code != 2 {
		t.Fatalf("expected an input error with code 2, got %v", err)
	}
	if flutreeErr.Message != "Base branch 'develop' does not exist in core." || flutreeErr.Hint != "Pass --package-base core=<branch>." {
		t.Fatalf("unexpected message/hint: %q / %q", flutreeErr.Message, flutreeErr.Hint)
	}
}

func TestApplyFailureDeletesOnlyBranchesCreatedInThisRun(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	rootRepo := filepath.Join(root, "root-app")
	pkgA := filepath.Join(root, "pkg-a")
	pkgB := filepath.Join(root, "pkg-b")
	g := &fakeCreateGit{
		repos: []domain.DiscoveredFlutterRepo{
			{Name: "root-app", RepoRoot: rootRepo, PackageName: "root_app"},
			{Name: "pkg-a", RepoRoot: pkgA, PackageName: "pkg_a"},
			{Name: "pkg-b", RepoRoot: pkgB, PackageName: "pkg_b"},
		},
		branchExistsByID: map[string]bool{pkgA + "::feature/x": true}, // pre-existing: reused
		failCreateAt:     filepath.Join("packages", "pkg-b"),
	}
	svc := NewCreateService(g, &fakeRegistry{}, &fakeCreatePrompt{})
	plan, err := svc.BuildDryPlan(domain.CreateInput{
		Name: "rollback-" + strings.ReplaceAll(filepath.Base(root), "_", "-"), Branch: "feature/x", BaseBranch: "main", ExecutionScope: root,
		RootSelector: "root-app", PackageSelectors: []string{"pkg-a", "pkg-b"},
		PackageBaseBranch: map[string]string{"pkg-a": "main", "pkg-b": "main"}, NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("build plan failed: %v", err)
	}
	if _, err := svc.Apply(plan, domain.CreateApplyOptions{NonInteractive: true, ReuseExistingBranch: true}); err == nil {
		t.Fatalf("expected the apply to fail")
	}
	if len(g.deleted) != 1 || g.deleted[0] != rootRepo+"::feature/x" {
		t.Fatalf("only the branch created in this run may be deleted, got %v", g.deleted)
	}
}

func (f *fakeCreateGit) CurrentBranch(string) (string, error) { return "main", nil }
func (f *fakeCreateGit) ListBranches(string) ([]string, []string, error) {
	return []string{"main"}, []string{"origin/main"}, nil
}
