package ui

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/EndersonPro/flutree/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

func TestAddRepoWizardBlocksContinueWhenSelectionEmpty(t *testing.T) {
	repos := []domain.DiscoveredFlutterRepo{
		{Name: "core-pkg", PackageName: "core", RepoRoot: "/repos/core"},
		{Name: "design-pkg", PackageName: "design", RepoRoot: "/repos/design"},
	}

	m := newAddRepoWizardModel(AddRepoWizardInput{RootBranch: "feature/login"}, repos)
	m.selected = map[int]bool{}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next := updated.(addRepoWizardModel)

	if !strings.Contains(next.errMsg, "Select at least one repository") {
		t.Fatalf("expected empty-selection validation, got %q", next.errMsg)
	}
	if next.step != addRepoWizardStepSelectRepos {
		t.Fatalf("expected to remain on selection step, got %v", next.step)
	}
}

func TestAddRepoWizardSupportsNavigationAndToggle(t *testing.T) {
	repos := []domain.DiscoveredFlutterRepo{
		{Name: "core-pkg", PackageName: "core", RepoRoot: "/repos/core"},
		{Name: "design-pkg", PackageName: "design", RepoRoot: "/repos/design"},
	}

	m := newAddRepoWizardModel(AddRepoWizardInput{RootBranch: "feature/login"}, repos)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	next := updated.(addRepoWizardModel)
	if next.cursor != 1 {
		t.Fatalf("expected cursor to move down, got %d", next.cursor)
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	next = updated.(addRepoWizardModel)
	if !next.selected[1] {
		t.Fatalf("expected current repo to be toggled")
	}
}

func TestAddRepoWizardReviewApplyProducesRepoRootKeyedResult(t *testing.T) {
	repos := []domain.DiscoveredFlutterRepo{
		{Name: "design-pkg", PackageName: "design", RepoRoot: "/repos/design"},
		{Name: "core-pkg", PackageName: "core", RepoRoot: "/repos/core"},
	}

	m := newAddRepoWizardModel(AddRepoWizardInput{RootBranch: "feature/login"}, repos)
	m.selected = map[int]bool{0: true, 1: true}
	m.selectedRepos = m.selectedReposFromMap()
	m.repoOptions = map[string]addRepoOption{
		"/repos/design": {SourceBranch: "feature/design", BaseBranch: "main"},
		"/repos/core":   {SourceBranch: "feature/core", BaseBranch: "release/1.0"},
	}
	m.step = addRepoWizardStepReview
	m.finalChoice = addRepoWizardFinalApply

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next := updated.(addRepoWizardModel)
	if next.step != addRepoWizardStepRepoOptions {
		t.Fatalf("expected review confirm to continue into branch/base prompts, got step=%v", next.step)
	}
	if next.done {
		t.Fatalf("expected wizard to continue after review confirmation")
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next = updated.(addRepoWizardModel)
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next = updated.(addRepoWizardModel)
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next = updated.(addRepoWizardModel)
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next = updated.(addRepoWizardModel)

	if !next.done {
		t.Fatalf("expected wizard to finish after branch/base prompts")
	}

	result := next.result()
	if !result.Apply || result.Cancelled {
		t.Fatalf("expected apply=true and cancelled=false, got %+v", result)
	}

	if len(result.RepoSelectors) != 2 {
		t.Fatalf("expected 2 selected repos, got %v", result.RepoSelectors)
	}

	if result.RepoSelectors[0] != "/repos/core" || result.RepoSelectors[1] != "/repos/design" {
		t.Fatalf("expected deterministic sorted selectors by repo name, got %v", result.RepoSelectors)
	}

	if result.PackageBranchSource["/repos/core"] != "feature/core" {
		t.Fatalf("expected repo-root keyed source map, got %v", result.PackageBranchSource)
	}
	if result.PackageBaseBranch["/repos/design"] != "main" {
		t.Fatalf("expected repo-root keyed base map, got %v", result.PackageBaseBranch)
	}
}

func TestAddRepoWizardCancelOnEscHasNoApply(t *testing.T) {
	repos := []domain.DiscoveredFlutterRepo{{Name: "core-pkg", PackageName: "core", RepoRoot: "/repos/core"}}
	m := newAddRepoWizardModel(AddRepoWizardInput{RootBranch: "feature/login"}, repos)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next := updated.(addRepoWizardModel)
	result := next.result()

	if !result.Cancelled {
		t.Fatalf("expected cancelled result on esc")
	}
	if result.Apply {
		t.Fatalf("expected apply=false when cancelled")
	}
}

func TestAddRepoWizardViewShowsReviewTableAndEnglishCopy(t *testing.T) {
	repos := []domain.DiscoveredFlutterRepo{{Name: "core-pkg", PackageName: "core", RepoRoot: filepath.Clean("/repos/core")}}
	m := newAddRepoWizardModel(AddRepoWizardInput{RootBranch: "feature/login"}, repos)
	m.step = addRepoWizardStepReview
	m.selected = map[int]bool{0: true}
	m.selectedRepos = m.selectedReposFromMap()

	view := cleanANSI(m.View())
	if !strings.Contains(view, "Step 2 - Review and confirm") {
		t.Fatalf("expected review title in english, got %q", view)
	}
	if !regexp.MustCompile(`\|\s*Repository\s*\|\s*Package\s*\|`).MatchString(view) {
		t.Fatalf("expected review repository table headers, got %q", view)
	}
	if !strings.Contains(view, "Continue") {
		t.Fatalf("expected continue option in review view, got %q", view)
	}
}
