package ui

import (
	"fmt"
	"os"
	"path/filepath"
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

	// Inspect reports whether a single folder is a Flutter repository (it marks
	// folders in the navigator and lets Tab pick a repo folder directly).
	Inspect func(path string) (domain.DiscoveredFlutterRepo, bool)
	// BaseBranchExists checks a package base branch exists in a repository.
	BaseBranchExists func(repoRoot, branch string) bool
	// ScanInitial makes the wizard discover Scope itself (showing "Scanning")
	// when it is started without repositories.
	ScanInitial bool
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
	// Repos is the full candidate set the wizard worked with (it may span
	// unrelated folders); the plan must use it instead of rediscovering a scope.
	Repos     []domain.DiscoveredFlutterRepo
	Apply     bool
	Cancelled bool
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
	// stepInitialScan waits for the first discovery of the starting scope.
	stepInitialScan
)

// navMode tells what Tab does in the folder navigator.
type navMode int

const (
	navScope       navMode = iota // pick the scope folder or a root repo folder
	navAddPackages                // add package repos from anywhere
)

type scanKind int

const (
	scanInitial scanKind = iota
	scanScope
	scanRootParent
	scanAddFolder
)

// scanResultMsg carries the outcome of an asynchronous discovery.
type scanResultMsg struct {
	seq   int
	kind  scanKind
	dir   string
	repos []domain.DiscoveredFlutterRepo
	err   error
	root  domain.DiscoveredFlutterRepo
}

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
	navMode        navMode
	rootChosen     bool // the root was picked directly in the navigator
	inspect        func(path string) (domain.DiscoveredFlutterRepo, bool)
	baseExists     func(repoRoot, branch string) bool
	width          int

	scanning    bool
	scanDir     string
	scanSeq     int
	scanKind    scanKind
	pendingRoot domain.DiscoveredFlutterRepo
	initCmd     tea.Cmd
	initRoot    string
	initPkgs    []string

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
		selectors = append(selectors, selectorFor(pkg, finalModel.repos))
	}

	return CreateWizardResult{
		Name:              strings.TrimSpace(finalModel.name),
		Branch:            strings.TrimSpace(finalModel.branch),
		BaseBranch:        strings.TrimSpace(finalModel.baseBranch),
		GenerateWorkspace: finalModel.generateWorkspace,
		RootSelector:      selectorFor(finalModel.repos[finalModel.rootIndex], finalModel.repos),
		NoPackage:         finalModel.noPackage,
		PackageSelectors:  selectors,
		PackageBaseBranch: copyStringMap(finalModel.pkgBaseBranch),
		Scope:             finalModel.scope,
		SaveScope:         finalModel.shouldSaveScope(),
		Repos:             append([]domain.DiscoveredFlutterRepo(nil), finalModel.repos...),
		Apply:             finalModel.finalChoice == 1,
	}, nil
}

// selectorFor names a repo for the plan: its name, or its path when another
// candidate shares the name.
func selectorFor(repo domain.DiscoveredFlutterRepo, all []domain.DiscoveredFlutterRepo) string {
	for _, other := range all {
		if other.Name == repo.Name && domain.NormalizePath(other.RepoRoot) != domain.NormalizePath(repo.RepoRoot) {
			return repo.RepoRoot
		}
	}
	return repo.Name
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

// withSelectorRepos adds to repos every path selector (--root-repo, --package)
// that names a Flutter repo not already known, such as one outside the scope,
// using the same inspection the folder navigator uses.
func withSelectorRepos(inspect func(string) (domain.DiscoveredFlutterRepo, bool), repos []domain.DiscoveredFlutterRepo, selectors []string) []domain.DiscoveredFlutterRepo {
	if inspect == nil {
		return repos
	}
	out := append([]domain.DiscoveredFlutterRepo(nil), repos...)
	for _, selector := range selectors {
		if !selectorIsPath(selector) {
			continue
		}
		known := false
		for _, repo := range out {
			if matchesSelector(repo, selector) {
				known = true
				break
			}
		}
		if known {
			continue
		}
		if repo, ok := inspect(expandSelectorPath(selector)); ok {
			out = append(out, repo)
		}
	}
	return out
}

func selectorIsPath(selector string) bool {
	sel := strings.TrimSpace(selector)
	return strings.HasPrefix(sel, "~") || strings.HasPrefix(sel, ".") || strings.ContainsRune(sel, filepath.Separator)
}

// expandSelectorPath expands a leading "~" and makes the path absolute.
func expandSelectorPath(selector string) string {
	sel := strings.TrimSpace(selector)
	if sel == "~" || strings.HasPrefix(sel, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			sel = filepath.Join(home, strings.TrimPrefix(sel, "~"))
		}
	}
	return domain.NormalizePath(sel)
}

func newCreateWizardModel(input CreateWizardInput, repos []domain.DiscoveredFlutterRepo) createWizardModel {
	orderedRepos := sortedRepos(withSelectorRepos(input.Inspect, repos, append([]string{input.RootSelector}, input.PackageSelectors...)))

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
		inspect:           input.Inspect,
		baseExists:        input.BaseBranchExists,
		initRoot:          input.RootSelector,
		initPkgs:          input.PackageSelectors,
	}
	if len(orderedRepos) == 0 {
		if input.ScanInitial && input.Discover != nil {
			m.step = stepInitialScan
			m.initCmd = m.startScan(scanInitial, input.Scope)
			return m
		}
		m.step = stepScope
		m.nav = m.newNavigator(input.Scope)
		return m
	}
	m.refreshPackageCandidates(input.PackageSelectors)
	return m
}

func (m createWizardModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.initCmd)
}

func (m createWizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.nav.width = msg.Width
		return m, nil
	case scanResultMsg:
		return m.handleScan(msg)
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.cancelled = true
			m.done = true
			return m, tea.Quit
		}
		if m.scanning {
			// Discovery is running: only Esc (stop waiting) does anything.
			if msg.Type == tea.KeyEsc {
				m.cancelScan()
			}
			return m, nil
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
		b.WriteString(m.hint("Enter to continue - Ctrl+C to cancel"))
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
		b.WriteString(m.hint("Scope: " + shortenLeft(m.scope, m.width-len("Scope: "))))
		b.WriteString("\n")
		b.WriteString(m.hint("Type to filter - Arrow keys to move - Enter to confirm - ctrl+o change folder - Esc clears filter/cancels"))
	case stepPackages:
		b.WriteString(wizardSectionStyle.Render("Step 3 - Select packages"))
		b.WriteString("\n")
		if m.noPackage {
			b.WriteString("Package selection skipped (--no-package).\n")
			b.WriteString(m.hint("Enter to continue"))
			break
		}
		if len(m.packageCandidates) == 0 {
			b.WriteString("No package candidates found for this root.\n")
			b.WriteString(m.hint(m.packagesHint(false)))
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
		b.WriteString(m.hint(m.packagesHint(true)))
	case stepBranches:
		b.WriteString(m.branchesView())
	case stepReview:
		b.WriteString(m.reviewView())
	case stepScope:
		b.WriteString(m.scopeView())
	case stepInitialScan:
		b.WriteString(m.scanLine())
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
		m.input.Blur()
		if m.rootChosen {
			m.advanceFromRoot()
			return m, nil
		}
		m.step = stepRootRepo
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
		m.advanceFromRoot()
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

	if msg.Type == tea.KeyCtrlO {
		m.openAddPackages()
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
			if m.baseExists != nil && !m.baseExists(pkg.RepoRoot, v) {
				m.errMsg = fmt.Sprintf("Base branch '%s' does not exist in %s.", v, pkg.Name)
				return m, nil
			}
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
			m.hint("Space or arrows to toggle - Enter to continue")
	}

	if m.branchFieldIndex == 3 {
		pkg := m.selectedPackages[m.pkgBaseIndex]
		return wizardSectionStyle.Render("Step 4 - Branch settings") + "\n" +
			fmt.Sprintf("Base branch for package '%s':\n%s\n\n%s", pkg.Name, m.input.View(), m.hint("Enter to continue"))
	}

	title := "Workspace branch"
	if m.branchFieldIndex == 1 {
		title = "Root base branch"
	}
	return wizardSectionStyle.Render("Step 4 - Branch settings") + "\n" + title + "\n" + m.input.View() + "\n\n" + m.hint("Enter to continue")
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
		b.WriteString(fmt.Sprintf("Save %s as default scope (scope.root): %s\n\n", shortenLeft(m.scope, m.width-len("Save  as default scope (scope.root): OFF")), state))
		hint = "Arrow keys to choose - s to toggle save scope - Enter to finish"
	}
	b.WriteString(m.hint(hint))
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
	if repo.Name == sel || repo.PackageName == sel {
		return true
	}
	if selectorIsPath(sel) {
		sel = expandSelectorPath(sel)
	}
	return domain.NormalizePath(repo.RepoRoot) == domain.NormalizePath(sel)
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

// hint renders a key hint, wrapped to the terminal width.
func (m createWizardModel) hint(text string) string {
	return wizardHintStyle.Render(wrapText(text, m.width))
}

func (m createWizardModel) packagesHint(withList bool) string {
	if !withList {
		return "Enter to continue - ctrl+o add packages from another folder"
	}
	return "Type to filter - Arrow keys to move - Space to toggle - Enter to continue - ctrl+o add packages from another folder - Esc clears filter/cancels"
}

// newNavigator opens the folder navigator, marking Flutter repo folders.
func (m createWizardModel) newNavigator(dir string) dirNavigator {
	var isRepo func(string) bool
	if m.inspect != nil {
		isRepo = func(path string) bool {
			_, ok := m.inspect(path)
			return ok
		}
	}
	nav := newDirNavigatorWith(dir, isRepo)
	nav.width = m.width
	return nav
}

// currentRepo reports the Flutter repo the navigator is standing in, if any.
func (m createWizardModel) currentRepo() (domain.DiscoveredFlutterRepo, bool) {
	if m.inspect == nil {
		return domain.DiscoveredFlutterRepo{}, false
	}
	return m.inspect(m.nav.path)
}

func (m *createWizardModel) openNavigator() {
	if m.discover == nil {
		return
	}
	m.nav = m.newNavigator(m.scope)
	m.navMode = navScope
	m.step = stepScope
}

func (m *createWizardModel) openAddPackages() {
	if m.discover == nil {
		return
	}
	m.nav = m.newNavigator(m.scope)
	m.navMode = navAddPackages
	m.step = stepScope
}

// advanceFromRoot leaves the root choice for the packages step (or the
// branches step when packages are skipped).
func (m *createWizardModel) advanceFromRoot() {
	m.errMsg = ""
	if m.noPackage {
		m.selectedPackages = nil
		m.pkgBaseBranch = map[string]string{}
		m.step = stepBranches
		m.prepareBranchFieldInput()
		return
	}
	m.refreshPackageCandidates(m.selectedPackageSelectors())
	m.step = stepPackages
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
		return m.selectFolder()
	case tea.KeyEsc:
		if m.nav.up() {
			return m, nil
		}
		if m.navMode == navAddPackages {
			m.step = stepPackages
			return m, nil
		}
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

// selectFolder is Tab in the navigator.
func (m createWizardModel) selectFolder() (tea.Model, tea.Cmd) {
	dir := m.nav.path
	if m.discover == nil {
		return m, nil
	}
	repo, isRepo := m.currentRepo()

	if m.navMode == navAddPackages {
		if isRepo {
			if domain.NormalizePath(repo.RepoRoot) == domain.NormalizePath(m.repos[m.rootIndex].RepoRoot) {
				m.nav.errMsg = "That folder is the root repository."
				return m, nil
			}
			m.addPackageRepos([]domain.DiscoveredFlutterRepo{repo}, true)
			m.step = stepPackages
			return m, nil
		}
		cmd := m.startScan(scanAddFolder, dir)
		return m, cmd
	}

	if isRepo {
		// The folder itself is the root; its siblings become package candidates.
		m.pendingRoot = repo
		cmd := m.startScan(scanRootParent, filepath.Dir(dir))
		return m, cmd
	}
	cmd := m.startScan(scanScope, dir)
	return m, cmd
}

// startScan marks a discovery as running and returns the command that runs it.
func (m *createWizardModel) startScan(kind scanKind, dir string) tea.Cmd {
	m.scanSeq++
	m.scanning = true
	m.scanDir = dir
	m.scanKind = kind
	seq, discover, root := m.scanSeq, m.discover, m.pendingRoot
	return func() tea.Msg {
		repos, err := discover(dir)
		return scanResultMsg{seq: seq, kind: kind, dir: dir, repos: repos, err: err, root: root}
	}
}

// cancelScan stops waiting for the running discovery; its result is dropped.
func (m *createWizardModel) cancelScan() {
	m.scanSeq++
	m.scanning = false
	if m.step == stepInitialScan {
		m.step = stepScope
		m.navMode = navScope
		m.nav = m.newNavigator(m.scanDir)
	}
}

func (m createWizardModel) handleScan(msg scanResultMsg) (tea.Model, tea.Cmd) {
	if !m.scanning || msg.seq != m.scanSeq {
		return m, nil
	}
	m.scanning = false
	found := msg.err == nil && len(msg.repos) > 0
	if msg.kind == scanInitial {
		// Explicit path selectors outside the scope join the candidates, and
		// alone are enough to skip the scope navigator.
		withPaths := withSelectorRepos(m.inspect, msg.repos, append([]string{m.initRoot}, m.initPkgs...))
		if len(withPaths) > len(msg.repos) {
			msg.repos, found = withPaths, true
		}
	}

	switch msg.kind {
	case scanInitial:
		if !found {
			m.step = stepScope
			m.navMode = navScope
			m.nav = m.newNavigator(msg.dir)
			return m, nil
		}
		m.repos = sortedRepos(msg.repos)
		m.rootIndex = 0
		for i, repo := range m.repos {
			if matchesSelector(repo, m.initRoot) {
				m.rootIndex = i
				break
			}
		}
		m.refreshPackageCandidates(m.initPkgs)
		m.step = stepWorkspaceName
		m.input.Focus()
	case scanScope:
		if !found {
			m.nav.errMsg = fmt.Sprintf("No Flutter repositories found in %s.", msg.dir)
			return m, nil
		}
		m.adoptScope(msg.dir, msg.repos)
	case scanRootParent:
		repos := []domain.DiscoveredFlutterRepo{msg.root}
		if msg.err == nil {
			repos = append(repos, msg.repos...)
		}
		m.adoptDirectRoot(msg.root, msg.dir, repos)
	case scanAddFolder:
		if !found {
			m.nav.errMsg = fmt.Sprintf("No Flutter repositories found in %s.", msg.dir)
			return m, nil
		}
		m.addPackageRepos(msg.repos, false)
		m.step = stepPackages
	}
	return m, nil
}

// resetChoices clears what depended on the previous candidate set and, when
// the workspace name is still unasked, moves to that step.
func (m *createWizardModel) resetChoices() {
	m.rootFilter = ""
	m.selectedPackages = nil
	m.pkgBaseBranch = map[string]string{}
	m.errMsg = ""
	if !m.nameConfirmed {
		m.step = stepWorkspaceName
		m.input.Focus()
	}
}

// adoptScope makes dir the scope with the repos found in it.
func (m *createWizardModel) adoptScope(dir string, repos []domain.DiscoveredFlutterRepo) {
	m.repos = sortedRepos(repos)
	m.scope = dir
	m.scopeChosen = true
	m.saveScope = true
	m.rootChosen = false
	m.rootIndex = 0
	m.refreshPackageCandidates(nil)
	m.resetChoices()
	if m.nameConfirmed {
		m.step = stepRootRepo
	}
}

// adoptDirectRoot makes root the root repo without the root list; repos (found
// in the root's parent folder) become the package candidates.
func (m *createWizardModel) adoptDirectRoot(root domain.DiscoveredFlutterRepo, parent string, repos []domain.DiscoveredFlutterRepo) {
	seen := map[string]bool{}
	all := make([]domain.DiscoveredFlutterRepo, 0, len(repos))
	for _, repo := range repos {
		key := domain.NormalizePath(repo.RepoRoot)
		if !seen[key] {
			seen[key] = true
			all = append(all, repo)
		}
	}
	m.repos = sortedRepos(all)
	for i, repo := range m.repos {
		if domain.NormalizePath(repo.RepoRoot) == domain.NormalizePath(root.RepoRoot) {
			m.rootIndex = i
		}
	}
	m.scope = parent
	m.scopeChosen = false // no scope was chosen: nothing to offer to save
	m.rootChosen = true
	m.refreshPackageCandidates(nil)
	m.resetChoices()
	if m.nameConfirmed {
		m.advanceFromRoot()
	}
}

// addPackageRepos merges repos into the candidate set (deduplicated, never the
// root), keeping the current selection; selectNew also selects them.
func (m *createWizardModel) addPackageRepos(repos []domain.DiscoveredFlutterRepo, selectNew bool) {
	rootKey := domain.NormalizePath(m.repos[m.rootIndex].RepoRoot)
	selected := map[string]bool{}
	for _, repo := range m.selectedPackagesFromMap() {
		selected[domain.NormalizePath(repo.RepoRoot)] = true
	}
	known := map[string]bool{}
	for _, repo := range m.repos {
		known[domain.NormalizePath(repo.RepoRoot)] = true
	}
	merged := append([]domain.DiscoveredFlutterRepo(nil), m.repos...)
	for _, repo := range repos {
		key := domain.NormalizePath(repo.RepoRoot)
		if key == rootKey {
			continue
		}
		if !known[key] {
			known[key] = true
			merged = append(merged, repo)
		}
		if selectNew {
			selected[key] = true
		}
	}
	m.repos = sortedRepos(merged)
	for i, repo := range m.repos {
		if domain.NormalizePath(repo.RepoRoot) == rootKey {
			m.rootIndex = i
		}
	}
	m.packageCandidates = make([]domain.DiscoveredFlutterRepo, 0, len(m.repos))
	m.pkgSelected = map[int]bool{}
	for _, repo := range m.repos {
		key := domain.NormalizePath(repo.RepoRoot)
		if key == rootKey {
			continue
		}
		if selected[key] {
			m.pkgSelected[len(m.packageCandidates)] = true
		}
		m.packageCandidates = append(m.packageCandidates, repo)
	}
	m.pkgCursor = 0
	m.pkgFilter = ""
	m.selectedPackages = m.selectedPackagesFromMap()
}

func (m createWizardModel) scanLine() string {
	return "Scanning " + shortenLeft(m.scanDir, m.width-len("Scanning …")) + "…\n\n" + m.hint("Esc to stop waiting - Ctrl+C to cancel")
}

func (m createWizardModel) scopeView() string {
	var b strings.Builder
	title := "Choose scope folder"
	back := "cancels"
	if len(m.repos) > 0 {
		back = "back"
	}
	hint := "Arrow keys or j/k to move - Enter to open - Tab to select this folder as scope (a Flutter repo folder becomes the root) - Esc goes up, then " + back
	if m.navMode == navAddPackages {
		title = "Add packages - choose a folder"
		hint = "Arrow keys or j/k to move - Enter to open - Tab to add this folder (a Flutter repo is added selected, other folders add the repos found in them) - Esc goes up, then back"
	}
	b.WriteString(wizardSectionStyle.Render(title))
	b.WriteString("\n")
	b.WriteString(m.nav.view())
	b.WriteString("\n")
	if m.scanning {
		b.WriteString(m.scanLine())
		return b.String()
	}
	b.WriteString(m.hint(hint))
	return b.String()
}
