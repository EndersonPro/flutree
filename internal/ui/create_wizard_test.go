package ui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/EndersonPro/flutree/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func cleanANSI(value string) string {
	return ansiRegex.ReplaceAllString(value, "")
}

func TestCreateWizardPrefillsRootAndPackages(t *testing.T) {
	repos := []domain.DiscoveredFlutterRepo{
		{Name: "root-app", PackageName: "root_app", RepoRoot: "/repos/root-app"},
		{Name: "core", PackageName: "core", RepoRoot: "/repos/core"},
		{Name: "design", PackageName: "design", RepoRoot: "/repos/design"},
	}

	m := newCreateWizardModel(CreateWizardInput{
		Name:             "feature-login",
		Branch:           "feature/login",
		BaseBranch:       "main",
		RootSelector:     "root-app",
		PackageSelectors: []string{"core"},
	}, repos)

	if m.repos[m.rootIndex].Name != "root-app" {
		t.Fatalf("unexpected root index: %+v", m.repos[m.rootIndex])
	}
	if len(m.packageCandidates) != 2 {
		t.Fatalf("unexpected package candidates length: %d", len(m.packageCandidates))
	}
	if !m.pkgSelected[0] {
		t.Fatalf("expected preselected package")
	}
}

func TestCreateWizardRequiresAtLeastOnePackage(t *testing.T) {
	repos := []domain.DiscoveredFlutterRepo{
		{Name: "root-app", PackageName: "root_app", RepoRoot: "/repos/root-app"},
		{Name: "core", PackageName: "core", RepoRoot: "/repos/core"},
	}

	m := newCreateWizardModel(CreateWizardInput{}, repos)
	m.step = stepPackages
	m.pkgSelected = map[int]bool{}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	newModel := updated.(createWizardModel)

	if newModel.errMsg == "" {
		t.Fatalf("expected validation error for empty package selection")
	}
}

func TestCreateWizardRootEnterMovesToPackageStepWhenCandidatesExist(t *testing.T) {
	repos := []domain.DiscoveredFlutterRepo{
		{Name: "root-app", PackageName: "root_app", RepoRoot: "/repos/root-app"},
		{Name: "core", PackageName: "core", RepoRoot: "/repos/core"},
	}

	m := newCreateWizardModel(CreateWizardInput{RootSelector: "root-app"}, repos)
	m.step = stepRootRepo

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	newModel := updated.(createWizardModel)

	if newModel.step != stepPackages {
		t.Fatalf("expected to enter packages step, got %v", newModel.step)
	}
	if len(newModel.packageCandidates) == 0 {
		t.Fatalf("expected package candidates to be available")
	}
}

func TestCreateWizardRootEnterShowsPackageStepEvenWithoutCandidates(t *testing.T) {
	repos := []domain.DiscoveredFlutterRepo{
		{Name: "root-app", PackageName: "root_app", RepoRoot: "/repos/root-app"},
	}

	m := newCreateWizardModel(CreateWizardInput{RootSelector: "root-app"}, repos)
	m.step = stepRootRepo

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	newModel := updated.(createWizardModel)

	if newModel.step != stepPackages {
		t.Fatalf("expected to stay in package step even without candidates, got %v", newModel.step)
	}
	if len(newModel.packageCandidates) != 0 {
		t.Fatalf("expected zero package candidates")
	}
}

func TestCreateWizardReviewSelectsDryRun(t *testing.T) {
	repos := []domain.DiscoveredFlutterRepo{
		{Name: "root-app", PackageName: "root_app", RepoRoot: "/repos/root-app"},
	}

	m := newCreateWizardModel(CreateWizardInput{}, repos)
	m.step = stepReview
	m.finalChoice = 1

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	newModel := updated.(createWizardModel)

	if newModel.finalChoice != 0 {
		t.Fatalf("expected dry-run choice, got %d", newModel.finalChoice)
	}
}

func TestCreateWizardWorkspaceStepUsesEnglishCopy(t *testing.T) {
	repos := []domain.DiscoveredFlutterRepo{
		{Name: "root-app", PackageName: "root_app", RepoRoot: "/repos/root-app"},
	}

	m := newCreateWizardModel(CreateWizardInput{}, repos)
	view := cleanANSI(m.View())

	if !strings.Contains(view, "Interactive wizard") {
		t.Fatalf("expected english wizard subtitle in view, got: %q", view)
	}
	if !strings.Contains(view, "Step 1 - Workspace name") {
		t.Fatalf("expected english step title in view, got: %q", view)
	}
	if !strings.Contains(view, "Enter to continue - Ctrl+C to cancel") {
		t.Fatalf("expected english navigation hint in view, got: %q", view)
	}
}

func TestCreateWizardReviewViewUsesEnglishChoices(t *testing.T) {
	repos := []domain.DiscoveredFlutterRepo{
		{Name: "root-app", PackageName: "root_app", RepoRoot: "/repos/root-app"},
		{Name: "core", PackageName: "core", RepoRoot: "/repos/core"},
	}

	m := newCreateWizardModel(CreateWizardInput{}, repos)
	m.step = stepReview
	m.selectedPackages = []domain.DiscoveredFlutterRepo{{Name: "core", PackageName: "core", RepoRoot: "/repos/core"}}
	m.pkgBaseBranch["core"] = "develop"
	view := cleanANSI(m.View())

	if !strings.Contains(view, "Step 5 - Final review") {
		t.Fatalf("expected english review title in view, got: %q", view)
	}
	if !strings.Contains(view, "Apply changes") {
		t.Fatalf("expected english apply action in review view, got: %q", view)
	}
	if !regexp.MustCompile(`\|\s*Role\s*\|\s*Repository\s*\|\s*Package\s*\|\s*Branch\s*\|\s*Base Branch\s*\|`).MatchString(view) {
		t.Fatalf("expected review table header with role/branch/base columns, got: %q", view)
	}
	if !regexp.MustCompile(`\|\s*package\s*\|\s*core\s*\|\s*core\s*\|\s*feature/feature\s*\|\s*develop\s*\|`).MatchString(view) {
		t.Fatalf("expected package detail row in review table, got: %q", view)
	}
}
