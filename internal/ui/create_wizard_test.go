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

func TestCreateWizardNoPackageSkipsPackageValidation(t *testing.T) {
	repos := []domain.DiscoveredFlutterRepo{
		{Name: "root-app", PackageName: "root_app", RepoRoot: "/repos/root-app"},
		{Name: "core", PackageName: "core", RepoRoot: "/repos/core"},
	}

	m := newCreateWizardModel(CreateWizardInput{NoPackage: true}, repos)
	m.step = stepPackages

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	newModel := updated.(createWizardModel)

	if newModel.errMsg != "" {
		t.Fatalf("did not expect validation error in no-package mode: %q", newModel.errMsg)
	}
	if newModel.step != stepBranches {
		t.Fatalf("expected branches step in no-package mode, got %v", newModel.step)
	}
}

func TestCreateWizardNoPackageRootStepJumpsToBranches(t *testing.T) {
	repos := []domain.DiscoveredFlutterRepo{
		{Name: "root-app", PackageName: "root_app", RepoRoot: "/repos/root-app"},
		{Name: "core", PackageName: "core", RepoRoot: "/repos/core"},
	}

	m := newCreateWizardModel(CreateWizardInput{RootSelector: "root-app", NoPackage: true}, repos)
	m.step = stepRootRepo

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	newModel := updated.(createWizardModel)

	if newModel.step != stepBranches {
		t.Fatalf("expected to jump to branch step in no-package mode, got %v", newModel.step)
	}
	if len(newModel.selectedPackages) != 0 {
		t.Fatalf("expected no selected packages in no-package mode")
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
	if !regexp.MustCompile(`[|│]\s*Role\s*[|│]\s*Repository\s*[|│]\s*Package\s*[|│]\s*Branch\s*[|│]\s*Base Branch\s*[|│]`).MatchString(view) {
		t.Fatalf("expected review table header with role/branch/base columns, got: %q", view)
	}
	if !regexp.MustCompile(`[|│]\s*package\s*[|│]\s*core\s*[|│]\s*core\s*[|│]\s*feature/feature\s*[|│]\s*develop\s*[|│]`).MatchString(view) {
		t.Fatalf("expected package detail row in review table, got: %q", view)
	}
}

func filterTestRepos() []domain.DiscoveredFlutterRepo {
	return []domain.DiscoveredFlutterRepo{
		{Name: "alpha-app", PackageName: "alpha_app", RepoRoot: "/repos/alpha-app"},
		{Name: "beta-app", PackageName: "beta_app", RepoRoot: "/repos/beta-app"},
		{Name: "core", PackageName: "shared_kit", RepoRoot: "/repos/libs/core"},
	}
}

func typeText(m createWizardModel, text string) createWizardModel {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
	return updated.(createWizardModel)
}

func TestCreateWizardFilterMatchesNamePackageAndPathCaseInsensitively(t *testing.T) {
	for _, tc := range []struct {
		filter string
		want   string
	}{
		{"BETA", "beta-app"},
		{"shared", "core"},
		{"libs/", "core"},
	} {
		m := newCreateWizardModel(CreateWizardInput{}, filterTestRepos())
		m.step = stepRootRepo
		m = typeText(m, tc.filter)
		visible := m.rootVisible()
		if len(visible) != 1 || m.repos[visible[0]].Name != tc.want {
			t.Fatalf("filter %q: visible = %v, want only %s", tc.filter, visible, tc.want)
		}
		if m.repos[m.rootIndex].Name != tc.want {
			t.Fatalf("filter %q: highlighted root = %s, want %s", tc.filter, m.repos[m.rootIndex].Name, tc.want)
		}
	}
}

func TestCreateWizardFilterBackspaceAndEscRestoreTheList(t *testing.T) {
	m := newCreateWizardModel(CreateWizardInput{}, filterTestRepos())
	m.step = stepRootRepo
	m = typeText(m, "zz")
	if len(m.rootVisible()) != 0 {
		t.Fatalf("expected no match")
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(createWizardModel)
	if m.step != stepRootRepo || m.errMsg == "" {
		t.Fatalf("Enter with no match must stay on the root step with an error")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = updated.(createWizardModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = updated.(createWizardModel)
	if m.rootFilter != "" || len(m.rootVisible()) != 3 {
		t.Fatalf("backspace should clear the filter, got %q", m.rootFilter)
	}
	m = typeText(m, "core")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(createWizardModel)
	if m.cancelled || m.rootFilter != "" {
		t.Fatalf("Esc with a filter must clear it without cancelling")
	}
}

func TestCreateWizardPackageSelectionSurvivesFiltering(t *testing.T) {
	m := newCreateWizardModel(CreateWizardInput{RootSelector: "alpha-app", PackageSelectors: []string{"beta-app"}}, filterTestRepos())
	m.step = stepPackages

	m = typeText(m, "core")
	if got := m.pkgVisible(); len(got) != 1 {
		t.Fatalf("expected one visible package, got %v", got)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	m = updated.(createWizardModel)

	selected := m.selectedPackagesFromMap()
	if len(selected) != 2 {
		t.Fatalf("expected beta-app (hidden) and core selected, got %+v", selected)
	}
}

func TestCreateWizardStartsInNavigatorWhenNoRepos(t *testing.T) {
	m := newCreateWizardModel(CreateWizardInput{Scope: t.TempDir(), Discover: func(string) ([]domain.DiscoveredFlutterRepo, error) {
		return nil, nil
	}}, nil)
	if m.step != stepScope {
		t.Fatalf("expected the navigator step, got %v", m.step)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(createWizardModel)
	if !m.scanning || cmd == nil {
		t.Fatalf("expected an asynchronous scan to start")
	}
	updated, _ = m.Update(cmd())
	m = updated.(createWizardModel)
	if m.step != stepScope || m.nav.errMsg == "" {
		t.Fatalf("expected to stay in the navigator with a message, got step=%v err=%q", m.step, m.nav.errMsg)
	}
}

func TestCreateWizardSaveScopeOnlyOfferedWhenScopeDiffersFromPersisted(t *testing.T) {
	m := newCreateWizardModel(CreateWizardInput{Scope: "/a", PersistedScope: "/a"}, filterTestRepos())
	m.scopeChosen = true
	if m.shouldSaveScope() {
		t.Fatalf("scope equal to the persisted one must not be offered")
	}
	m.scope = "/b"
	if !m.shouldSaveScope() {
		t.Fatalf("a new scope should be saved by default")
	}
	m.step = stepReview
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = updated.(createWizardModel)
	if m.shouldSaveScope() {
		t.Fatalf("s must toggle the save-scope option off")
	}
}
