package app

import (
	"os"
	"path/filepath"
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
	if !strings.Contains(plan.OverrideContent, "dependency_overrides:") || !strings.Contains(plan.OverrideContent, "core:") {
		t.Fatalf("unexpected override content: %s", plan.OverrideContent)
	}
}
