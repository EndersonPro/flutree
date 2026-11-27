package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/EndersonPro/flutree/internal/domain"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type CreateWizardInput struct {
	Name              string
	Branch            string
	BaseBranch        string
	GenerateWorkspace bool
	RootSelector      string
	NoPackage         bool
	PackageSelectors  []string
	PackageBaseBranch map[string]string

	// Scope is the directory the repositories were discovered in. Discover lists
	// the Flutter repositories of a directory and powers the scope navigator.
	// PersistedScope is the current scope.root value (empty when unset).
	Scope          string
	Discover       func(scope string) ([]domain.DiscoveredFlutterRepo, error)
	PersistedScope string
}

type CreateWizardResult struct {
	Name              string
	Branch            string
	BaseBranch        string
	GenerateWorkspace bool
	RootSelector      string
	NoPackage         bool
	PackageSelectors  []string
	PackageBaseBranch map[string]string
	Scope             string
	SaveScope         bool
	Apply             bool
	Cancelled         bool
}

type createWizardStep int

const (
	stepWorkspaceName createWizardStep = iota
	stepRootRepo
	stepPackages
	stepBranches
	stepReview
	// stepScope is the directory navigator: the first step when the starting
	// scope has no repositories, otherwise opened from the root step.
	stepScope
)

var (
	wizardTitleStyle          = lipgloss.NewStyle().Bold(true).Foreground(uiAccentColor)
	wizardSubtitleStyle       = lipgloss.NewStyle().Foreground(uiMutedColor)
	wizardSectionStyle        = lipgloss.NewStyle().Bold(true).Foreground(uiAccentColor)
	wizardHintStyle           = lipgloss.NewStyle().Foreground(uiMutedColor)
	wizardErrorStyle          = lipgloss.NewStyle().Bold(true).Foreground(uiErrorColor)
	wizardProgressActiveStyle = lipgloss.NewStyle().Bold(true).Foreground(uiAccentColor).Underline(true)
	wizardProgressIdleStyle   = lipgloss.NewStyle().Foreground(uiMutedColor)
)

type createWizardModel struct {
	step createWizardStep

	repos []domain.DiscoveredFlutterRepo

	name              string
	branch            string
	baseBranch        string
	generateWorkspace bool
	noPackage         bool

	rootIndex int

	packageCandidates []domain.DiscoveredFlutterRepo
	pkgCursor         int
	pkgSelected       map[int]bool

	rootFilter string
	pkgFilter  string

	scope          string
	persistedScope string
	discover       func(scope string) ([]domain.DiscoveredFlutterRepo, error)
	scopeChosen    bool
	saveScope      bool
	nameConfirmed  bool
	nav            dirNavigator

	branchFieldIndex int
	selectedPackages []domain.DiscoveredFlutterRepo
	pkgBaseIndex     int
	pkgBaseBranch    map[string]string

	finalChoice int

	input  textinput.Model
	errMsg string

	done      bool
	cancelled bool
}

func RunCreateWizard(input CreateWizardInput, repos []domain.DiscoveredFlutterRepo) (CreateWizardResult, error) {
	if len(repos) == 0 && input.Discover == nil {
		return CreateWizardResult{}, domain.NewError(domain.CategoryInput, 2, "No Flutter repositories found in scope.", "Adjust --scope so flutree can discover a root repository.", nil)
	}

	model := newCreateWizardModel(input, repos)
	resultModel, err := tea.NewProgram(model).Run()
	if err != nil {
		return CreateWizardResult{}, domain.NewError(domain.CategoryUnexpected, 1, "Interactive create flow failed.", "Retry the command or switch to --non-interactive mode.", err)
	}

	finalModel, ok := resultModel.(createWizardModel)
	if !ok {
		return CreateWizardResult{}, domain.NewError(domain.CategoryUnexpected, 1, "Invalid interactive create state.", "Retry the command.", nil)
	}

	if finalModel.cancelled {
		return CreateWizardResult{Cancelled: true}, nil
	}

	selectors := make([]string, 0, len(finalModel.selectedPackages))
	for _, pkg := range finalModel.selectedPackages {
		selectors = append(selectors, pkg.Name)
	}

	return CreateWizardResult{
		Name:              strings.TrimSpace(finalModel.name),
		Branch:            strings.TrimSpace(finalModel.branch),
		BaseBranch:        strings.TrimSpace(finalModel.baseBranch),
		GenerateWorkspace: finalModel.generateWorkspace,
		RootSelector:      finalModel.repos[finalModel.rootIndex].Name,
		NoPackage:         finalModel.noPackage,
		PackageSelectors:  selectors,
		PackageBaseBranch: copyStringMap(finalModel.pkgBaseBranch),
		Scope:             finalModel.scope,
		SaveScope:         finalModel.shouldSaveScope(),
		Apply:             finalModel.finalChoice == 1,
	}, nil
}

func sortedRepos(repos []domain.DiscoveredFlutterRepo) []domain.DiscoveredFlutterRepo {
	ordered := append([]domain.DiscoveredFlutterRepo(nil), repos...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Name != ordered[j].Name {
			return ordered[i].Name < ordered[j].Name
		}
		return ordered[i].RepoRoot < ordered[j].RepoRoot
	})
	return ordered
}

func newCreateWizardModel(input CreateWizardInput, repos []domain.DiscoveredFlutterRepo) createWizardModel {
	orderedRepos := sortedRepos(repos)

	rootIndex := 0
	for i, repo := range orderedRepos {
		if matchesSelector(repo, input.RootSelector) {
			rootIndex = i
			break
		}
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = "feature"
	}
	branch := strings.TrimSpace(input.Branch)
	if branch == "" {
		branch = "feature/feature"
	}
	baseBranch := strings.TrimSpace(input.BaseBranch)
	if baseBranch == "" {
		baseBranch = "main"
	}

	ti := textinput.New()
	ti.Focus()
	ti.Prompt = "Workspace name: "
	ti.SetValue(name)
	ti.CursorEnd()

	m := createWizardModel{
		step:              stepWorkspaceName,
		repos:             orderedRepos,
		name:              name,
		branch:            branch,
		baseBranch:        baseBranch,
		generateWorkspace: input.GenerateWorkspace,
		noPackage:         input.NoPackage,
		rootIndex:         rootIndex,
		pkgSelected:       map[int]bool{},
		pkgBaseBranch:     copyStringMap(input.PackageBaseBranch),
		input:             ti,
		finalChoice:       1,
		scope:             input.Scope,
		persistedScope:    input.PersistedScope,
		discover:          input.Discover,
		saveScope:         true,
	}
	if len(orderedRepos) == 0 {
		m.step = stepScope
		m.nav = newDirNavigator(input.Scope)
		return m
	}
	m.refreshPackageCandidates(input.PackageSelectors)
	return m
}

func (m createWizardModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m createWizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.cancelled = true
			m.done = true
			return m, tea.Quit
		}

		switch m.step {
		case stepWorkspaceName:
			return m.updateWorkspaceName(msg)
		case stepRootRepo:
			return m.updateRootRepo(msg)
		case stepPackages:
			return m.updatePackages(msg)
		case stepBranches:
			return m.updateBranches(msg)
		case stepReview:
			return m.updateReview(msg)
		case stepScope:
			return m.updateScope(msg)
		}
	}

	if m.step == stepWorkspaceName || m.step == stepBranches {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m createWizardModel) View() string {
	var b strings.Builder
	b.WriteString(wizardTitleStyle.Render("flutree create"))
	b.WriteString("\n")
	b.WriteString(wizardSubtitleStyle.Render("Interactive wizard"))
	b.WriteString(m.progressLabel())
	b.WriteString("\n\n")

	if m.errMsg != "" {
		b.WriteString(wizardErrorStyle.Render("Error: " + m.errMsg))
		b.WriteString("\n\n")
	}

	switch m.step {
	case stepWorkspaceName:
		b.WriteString(wizardSectionStyle.Render("Step 1 - Workspace name"))
		b.WriteString("\n")
		b.WriteString(m.input.View())
		b.WriteString("\n\n")
		b.WriteString(wizardHintStyle.Render("Enter to continue - Ctrl+C to cancel"))
	case stepRootRepo:
		b.WriteString(wizardSectionStyle.Render("Step 2 - Choose root repository"))
		b.WriteString("\n")
		b.WriteString(filterLine(m.rootFilter))
		visible := m.rootVisible()
		if len(visible) == 0 {
			b.WriteString("No repositories match the filter.\n")
		}
		for _, idx := range visible {
			cursor := " "
			if idx == m.rootIndex {
				cursor = ">"
			}
			repo := m.repos[idx]
			b.WriteString(fmt.Sprintf("%s %s [%s] (%s)\n", cursor, repo.Name, repo.PackageName, repo.RepoRoot))
		}
		b.WriteString("\n")
		b.WriteString(wizardHintStyle.Render("Scope: " + m.scope))
		b.WriteString("\n")
		b.WriteString(wizardHintStyle.Render("Type to filter - Arrow keys to move - Enter to confirm - ctrl+o change folder - Esc clears filter/cancels"))
	case stepPackages:
		b.WriteString(wizardSectionStyle.Render("Step 3 - Select packages"))
		b.WriteString("\n")
		if m.noPackage {
			b.WriteString("Package selection skipped (--no-package).\n")
			b.WriteString(wizardHintStyle.Render("Enter to continue"))
			break
		}
		if len(m.packageCandidates) == 0 {
			b.WriteString("No package candidates found for this root.\n")
			b.WriteString(wizardHintStyle.Render("Enter to continue"))
			break
		}
		b.WriteString(filterLine(m.pkgFilter))
		visible := m.pkgVisible()
		if len(visible) == 0 {
			b.WriteString("No packages match the filter.\n")
		}
		for pos, idx := range visible {
			pkg := m.packageCandidates[idx]
			cursor := " "
			if pos == m.pkgCursor {
				cursor = ">"
			}
			marker := "○"
			if m.pkgSelected[idx] {
				marker = "◉"
			}
			b.WriteString(fmt.Sprintf("%s %s %s [%s]\n", cursor, marker, pkg.Name, pkg.PackageName))
		}
		b.WriteString("\n")
		b.WriteString(wizardHintStyle.Render("Type to filter - Arrow keys to move - Space to toggle - Enter to continue - Esc clears filter/cancels"))
	case stepBranches:
		b.WriteString(m.branchesView())
	case stepReview:
		b.WriteString(m.reviewView())
	case stepScope:
		b.WriteString(m.scopeView())
	}

	return b.String()
}

func (m createWizardModel) updateWorkspaceName(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		value := strings.TrimSpace(m.input.Value())
		if value == "" {
			m.errMsg = "Workspace name cannot be empty."
			return m, nil
		}
		m.errMsg = ""
		m.name = value
		m.nameConfirmed = true
		m.step = stepRootRepo
		m.input.Blur()
		return m, nil
	case tea.KeyEsc:
		m.cancelled = true
		m.done = true
		return m, tea.Quit
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m createWizardModel) updateRootRepo(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		m.moveRoot(-1)
		m.errMsg = ""
		return m, nil
	case tea.KeyDown:
		m.moveRoot(1)
		m.errMsg = ""
		return m, nil
	case tea.KeyCtrlO:
		m.openNavigator()
		return m, nil
	case tea.KeyBackspace:
		m.rootFilter = trimLastRune(m.rootFilter)
		m.errMsg = ""
		m.syncRootToFilter()
		return m, nil
	case tea.KeyEnter:
		if len(m.rootVisible()) == 0 {
			m.errMsg = "No repository matches the filter."
			return m, nil
		}
		m.errMsg = ""
		if m.noPackage {
			m.selectedPackages = nil
			m.pkgBaseBranch = map[string]string{}
			m.step = stepBranches
			m.prepareBranchFieldInput()
			return m, nil
		}
		selected := m.selectedPackageSelectors()
		m.refreshPackageCandidates(selected)
		m.step = stepPackages
		return m, nil
	case tea.KeyEsc:
		if m.rootFilter != "" {
			m.rootFilter = ""
			m.syncRootToFilter()
			return m, nil
		}
		m.cancelled = true
		m.done = true
		return m, tea.Quit
	case tea.KeyRunes:
		m.rootFilter += string(msg.Runes)
		m.errMsg = ""
		m.syncRootToFilter()
	}
	return m, nil
}

func (m createWizardModel) updatePackages(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.noPackage {
		if msg.String() == "enter" {
			m.selectedPackages = nil
			m.pkgBaseBranch = map[string]string{}
			m.step = stepBranches
			m.prepareBranchFieldInput()
		}
		if msg.String() == "esc" {
			m.cancelled = true
			m.done = true
			return m, tea.Quit
		}
		return m, nil
	}

	if len(m.packageCandidates) == 0 {
		if msg.String() == "enter" {
			m.selectedPackages = nil
			m.step = stepBranches
			m.prepareBranchFieldInput()
		}
		if msg.String() == "esc" {
			m.cancelled = true
			m.done = true
			return m, tea.Quit
		}
		return m, nil
	}

	visible := m.pkgVisible()
	switch msg.Type {
	case tea.KeyUp:
		if m.pkgCursor > 0 {
			m.pkgCursor--
		}
		m.errMsg = ""
	case tea.KeyDown:
		if m.pkgCursor < len(visible)-1 {
			m.pkgCursor++
		}
		m.errMsg = ""
	case tea.KeyBackspace:
		m.pkgFilter = trimLastRune(m.pkgFilter)
		m.pkgCursor = 0
		m.errMsg = ""
	case tea.KeySpace:
		if m.pkgCursor < len(visible) {
			idx := visible[m.pkgCursor]
			m.pkgSelected[idx] = !m.pkgSelected[idx]
		}
		m.errMsg = ""
	case tea.KeyEnter:
		selected := m.selectedPackagesFromMap()
		if len(selected) == 0 {
			m.errMsg = "Select at least one package."
			return m, nil
		}
		m.errMsg = ""
		m.selectedPackages = selected
		m.step = stepBranches
		m.prepareBranchFieldInput()
	case tea.KeyEsc:
		if m.pkgFilter != "" {
			m.pkgFilter = ""
			m.pkgCursor = 0
			return m, nil
		}
		m.cancelled = true
		m.done = true
		return m, tea.Quit
	case tea.KeyRunes:
		m.pkgFilter += string(msg.Runes)
		m.pkgCursor = 0
		m.errMsg = ""
	}

	return m, nil
}

func (m createWizardModel) updateBranches(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.cancelled = true
		m.done = true
		return m, tea.Quit
	case tea.KeyEnter:
		switch m.branchFieldIndex {
		case 0:
			v := strings.TrimSpace(m.input.Value())
			if v == "" {
				m.errMsg = "Branch cannot be empty."
				return m, nil
			}
			m.branch = v
			m.branchFieldIndex = 1
			m.errMsg = ""
			m.prepareBranchFieldInput()
			return m, nil
		case 1:
			v := strings.TrimSpace(m.input.Value())
			if v == "" {
				m.errMsg = "Base branch cannot be empty."
				return m, nil
			}
			m.baseBranch = v
			m.branchFieldIndex = 2
			m.errMsg = ""
			m.prepareBranchFieldInput()
			return m, nil
		case 2:
			m.errMsg = ""
			if len(m.selectedPackages) == 0 {
				m.step = stepReview
				return m, nil
			}
			m.branchFieldIndex = 3
			m.pkgBaseIndex = 0
			m.prepareBranchFieldInput()
			return m, nil
		case 3:
			v := strings.TrimSpace(m.input.Value())
			if v == "" {
				m.errMsg = "Package base branch cannot be empty."
				return m, nil
			}
			pkg := m.selectedPackages[m.pkgBaseIndex]
			m.pkgBaseBranch[pkg.Name] = v
			m.pkgBaseIndex++
			m.errMsg = ""
			if m.pkgBaseIndex >= len(m.selectedPackages) {
				m.step = stepReview
				return m, nil
			}
			m.prepareBranchFieldInput()
			return m, nil
		}
	}

	if m.branchFieldIndex == 2 {
		switch msg.String() {
		case "left", "right", " ", "up", "down", "j", "k":
			m.generateWorkspace = !m.generateWorkspace
			m.errMsg = ""
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m createWizardModel) updateReview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "left", "up", "k", "h":
		if m.finalChoice > 0 {
			m.finalChoice--
		}
	case "right", "down", "j", "l":
		if m.finalChoice < 1 {
			m.finalChoice++
		}
	case "s":
		if m.offersSaveScope() {
			m.saveScope = !m.saveScope
		}
	case "enter":
		m.done = true
		return m, tea.Quit
	case "esc":
		m.cancelled = true
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

func (m *createWizardModel) refreshPackageCandidates(preselected []string) {
	root := m.repos[m.rootIndex]
	m.packageCandidates = make([]domain.DiscoveredFlutterRepo, 0, len(m.repos))
	for _, repo := range m.repos {
		if domain.NormalizePath(repo.RepoRoot) == domain.NormalizePath(root.RepoRoot) {
			continue
		}
		m.packageCandidates = append(m.packageCandidates, repo)
	}
	m.pkgCursor = 0
	m.pkgFilter = ""
	m.pkgSelected = map[int]bool{}

	for i, repo := range m.packageCandidates {
		for _, selector := range preselected {
			if matchesSelector(repo, selector) {
				m.pkgSelected[i] = true
				break
			}
		}
	}

	if len(m.packageCandidates) > 0 && len(m.pkgSelected) == 0 {
		m.pkgSelected[0] = true
	}
	m.selectedPackages = m.selectedPackagesFromMap()
}

func (m createWizardModel) progressLabel() string {
	type stepInfo struct {
		num   int
		label string
	}
	steps := []stepInfo{
		{1, "Name"},
		{2, "Root"},
		{3, "Packages"},
		{4, "Branches"},
		{5, "Review"},
	}
	var out strings.Builder
	out.WriteString("\n")
	for _, s := range steps {
		prefix := "○"
		style := wizardProgressIdleStyle
		if int(m.step)+1 == s.num {
			prefix = "●"
			style = wizardProgressActiveStyle
		}
		out.WriteString(style.Render(prefix + " Step " + strconv.Itoa(s.num) + ": " + s.label))
		if s.num < len(steps) {
			out.WriteString("  ")
		}
	}
	return out.String()
}

func (m createWizardModel) branchesView() string {
	if m.branchFieldIndex == 2 {
		state := "OFF"
		if m.generateWorkspace {
			state = "ON"
		}
		return wizardSectionStyle.Render("Step 4 - Branch settings") + "\n" +
			"Generate .code-workspace file: " + state + "\n\n" +
			wizardHintStyle.Render("Space or arrows to toggle - Enter to continue")
	}

	if m.branchFieldIndex == 3 {
		pkg := m.selectedPackages[m.pkgBaseIndex]
		return wizardSectionStyle.Render("Step 4 - Branch settings") + "\n" +
			fmt.Sprintf("Base branch for package '%s':\n%s\n\n%s", pkg.Name, m.input.View(), wizardHintStyle.Render("Enter to continue"))
	}

	title := "Workspace branch"
	if m.branchFieldIndex == 1 {
		title = "Root base branch"
	}
	return wizardSectionStyle.Render("Step 4 - Branch settings") + "\n" + title + "\n" + m.input.View() + "\n\n" + wizardHintStyle.Render("Enter to continue")
}

func (m createWizardModel) reviewView() string {
	choiceDryRun := "( ) Dry-run only"
	choiceApply := "( ) Apply changes"
	if m.finalChoice == 0 {
		choiceDryRun = "(*) Dry-run only"
	} else {
		choiceApply = "(*) Apply changes"
	}

	var b strings.Builder
	b.WriteString(wizardSectionStyle.Render("Step 5 - Final review"))
	b.WriteString("\n")
	b.WriteString(renderStyledTableNoZebra(
		[]string{"Setting", "Value"},
		[][]string{
			{"Workspace", m.name},
			{"Root", m.repos[m.rootIndex].Name},
			{"Workspace file", fmt.Sprintf("%t", m.generateWorkspace)},
		},
	))
	b.WriteString("\n")

	b.WriteString(renderStyledTableNoZebra(
		[]string{"Role", "Repository", "Package", "Branch", "Base Branch"},
		m.reviewRows(),
	))
	b.WriteString("\n")
	b.WriteString(choiceDryRun)
	b.WriteString("\n")
	b.WriteString(choiceApply)
	b.WriteString("\n\n")
	hint := "Arrow keys to choose - Enter to finish"
	if m.offersSaveScope() {
		state := "OFF"
		if m.saveScope {
			state = "ON"
		}
		b.WriteString(fmt.Sprintf("Save %s as default scope (scope.root): %s\n\n", m.scope, state))
		hint = "Arrow keys to choose - s to toggle save scope - Enter to finish"
	}
	b.WriteString(wizardHintStyle.Render(hint))
	return b.String()
}

func (m createWizardModel) reviewRows() [][]string {
	rows := [][]string{{
		"root",
		m.repos[m.rootIndex].Name,
		m.repos[m.rootIndex].PackageName,
		m.branch,
		m.baseBranch,
	}}

	if len(m.selectedPackages) == 0 {
		return append(rows, []string{"package", "(none)", "(none)", m.branch, "(n/a)"})
	}

	for _, pkg := range m.selectedPackages {
		baseBranch := strings.TrimSpace(m.pkgBaseBranch[pkg.Name])
		if baseBranch == "" {
			baseBranch = "develop"
		}
		rows = append(rows, []string{"package", pkg.Name, pkg.PackageName, m.branch, baseBranch})
	}
	return rows
}

func (m *createWizardModel) prepareBranchFieldInput() {
	if m.branchFieldIndex == 2 {
		m.input.Blur()
		return
	}

	if !m.input.Focused() {
		m.input.Focus()
	}

	switch m.branchFieldIndex {
	case 0:
		m.input.Prompt = "Branch: "
		m.input.SetValue(m.branch)
		m.input.CursorEnd()
	case 1:
		m.input.Prompt = "Base branch: "
		m.input.SetValue(m.baseBranch)
		m.input.CursorEnd()
	case 3:
		pkg := m.selectedPackages[m.pkgBaseIndex]
		value := m.pkgBaseBranch[pkg.Name]
		if strings.TrimSpace(value) == "" {
			value = "develop"
		}
		m.input.Prompt = "Package base branch: "
		m.input.SetValue(value)
		m.input.CursorEnd()
	}
}

func (m createWizardModel) selectedPackagesFromMap() []domain.DiscoveredFlutterRepo {
	selected := make([]domain.DiscoveredFlutterRepo, 0, len(m.pkgSelected))
	for i, repo := range m.packageCandidates {
		if m.pkgSelected[i] {
			selected = append(selected, repo)
		}
	}
	return selected
}

func (m createWizardModel) selectedPackageSelectors() []string {
	selected := m.selectedPackagesFromMap()
	out := make([]string, 0, len(selected))
	for _, pkg := range selected {
		out = append(out, pkg.Name)
	}
	return out
}

func copyStringMap(values map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range values {
		out[k] = v
	}
	return out
}

func matchesSelector(repo domain.DiscoveredFlutterRepo, selector string) bool {
	sel := strings.TrimSpace(selector)
	if sel == "" {
		return false
	}
	return repo.Name == sel || repo.PackageName == sel || domain.NormalizePath(repo.RepoRoot) == domain.NormalizePath(sel)
}

func trimLastRune(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return ""
	}
	return string(runes[:len(runes)-1])
}

func filterLine(filter string) string {
	if filter == "" {
		return wizardHintStyle.Render("Filter: (type to filter)") + "\n"
	}
	return "Filter: " + filter + "\n"
}

// repoMatchesFilter is a case-insensitive substring match over the repository
// name, package name and path.
func repoMatchesFilter(repo domain.DiscoveredFlutterRepo, filter string) bool {
	needle := strings.ToLower(strings.TrimSpace(filter))
	if needle == "" {
		return true
	}
	for _, field := range []string{repo.Name, repo.PackageName, repo.RepoRoot} {
		if strings.Contains(strings.ToLower(field), needle) {
			return true
		}
	}
	return false
}

// rootVisible returns the indexes (into m.repos) matching the root filter.
func (m createWizardModel) rootVisible() []int {
	out := make([]int, 0, len(m.repos))
	for i, repo := range m.repos {
		if repoMatchesFilter(repo, m.rootFilter) {
			out = append(out, i)
		}
	}
	return out
}

// pkgVisible returns the indexes (into m.packageCandidates) matching the package filter.
func (m createWizardModel) pkgVisible() []int {
	out := make([]int, 0, len(m.packageCandidates))
	for i, repo := range m.packageCandidates {
		if repoMatchesFilter(repo, m.pkgFilter) {
			out = append(out, i)
		}
	}
	return out
}

func (m *createWizardModel) moveRoot(delta int) {
	visible := m.rootVisible()
	pos := -1
	for i, idx := range visible {
		if idx == m.rootIndex {
			pos = i
			break
		}
	}
	next := pos + delta
	if pos < 0 {
		next = 0
	}
	if next < 0 || next >= len(visible) {
		return
	}
	m.rootIndex = visible[next]
}

// syncRootToFilter keeps the highlighted root among the visible repositories.
func (m *createWizardModel) syncRootToFilter() {
	visible := m.rootVisible()
	if len(visible) == 0 {
		return
	}
	for _, idx := range visible {
		if idx == m.rootIndex {
			return
		}
	}
	m.rootIndex = visible[0]
}

func (m createWizardModel) offersSaveScope() bool {
	if !m.scopeChosen {
		return false
	}
	return domain.NormalizePath(m.scope) != domain.NormalizePath(m.persistedScope)
}

func (m createWizardModel) shouldSaveScope() bool {
	return m.offersSaveScope() && m.saveScope
}

func (m *createWizardModel) openNavigator() {
	if m.discover == nil {
		return
	}
	m.nav = newDirNavigator(m.scope)
	m.step = stepScope
}

func (m createWizardModel) updateScope(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		m.nav.moveUp()
	case tea.KeyDown:
		m.nav.moveDown()
	case tea.KeyEnter:
		m.nav.enter()
	case tea.KeyTab:
		return m.selectScope()
	case tea.KeyEsc:
		if len(m.repos) > 0 {
			m.step = stepRootRepo
			return m, nil
		}
		m.cancelled = true
		m.done = true
		return m, tea.Quit
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "k":
			m.nav.moveUp()
		case "j":
			m.nav.moveDown()
		}
	}
	return m, nil
}

func (m createWizardModel) selectScope() (tea.Model, tea.Cmd) {
	dir := m.nav.path
	if m.discover == nil {
		return m, nil
	}
	repos, err := m.discover(dir)
	if err != nil || len(repos) == 0 {
		m.nav.errMsg = fmt.Sprintf("No Flutter repositories found in %s.", dir)
		return m, nil
	}

	m.repos = sortedRepos(repos)
	m.scope = dir
	m.scopeChosen = true
	m.saveScope = true
	m.rootIndex = 0
	m.rootFilter = ""
	m.refreshPackageCandidates(nil)
	m.selectedPackages = nil
	m.pkgBaseBranch = map[string]string{}
	m.errMsg = ""
	if m.nameConfirmed {
		m.step = stepRootRepo
	} else {
		m.step = stepWorkspaceName
		m.input.Focus()
	}
	return m, nil
}

func (m createWizardModel) scopeView() string {
	var b strings.Builder
	b.WriteString(wizardSectionStyle.Render("Choose scope folder"))
	b.WriteString("\n")
	b.WriteString(m.nav.view())
	b.WriteString("\n")
	hint := "Arrow keys or j/k to move - Enter to open - Tab to select this folder as scope - Esc to cancel"
	if len(m.repos) > 0 {
		hint = "Arrow keys or j/k to move - Enter to open - Tab to select this folder as scope - Esc to go back"
	}
	b.WriteString(wizardHintStyle.Render(hint))
	return b.String()
}
