package ui

import (
	"fmt"
	"sort"
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
	PackageSelectors  []string
	PackageBaseBranch map[string]string
}

type CreateWizardResult struct {
	Name              string
	Branch            string
	BaseBranch        string
	GenerateWorkspace bool
	RootSelector      string
	PackageSelectors  []string
	PackageBaseBranch map[string]string
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

	rootIndex int

	packageCandidates []domain.DiscoveredFlutterRepo
	pkgCursor         int
	pkgSelected       map[int]bool

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
	if len(repos) == 0 {
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
		PackageSelectors:  selectors,
		PackageBaseBranch: copyStringMap(finalModel.pkgBaseBranch),
		Apply:             finalModel.finalChoice == 1,
	}, nil
}

func newCreateWizardModel(input CreateWizardInput, repos []domain.DiscoveredFlutterRepo) createWizardModel {
	orderedRepos := append([]domain.DiscoveredFlutterRepo(nil), repos...)
	sort.Slice(orderedRepos, func(i, j int) bool {
		if orderedRepos[i].Name != orderedRepos[j].Name {
			return orderedRepos[i].Name < orderedRepos[j].Name
		}
		return orderedRepos[i].RepoRoot < orderedRepos[j].RepoRoot
	})

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
		rootIndex:         rootIndex,
		pkgSelected:       map[int]bool{},
		pkgBaseBranch:     copyStringMap(input.PackageBaseBranch),
		input:             ti,
		finalChoice:       1,
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
		for i, repo := range m.repos {
			cursor := " "
			if i == m.rootIndex {
				cursor = ">"
			}
			b.WriteString(fmt.Sprintf("%s %s [%s] (%s)\n", cursor, repo.Name, repo.PackageName, repo.RepoRoot))
		}
		b.WriteString("\n")
		b.WriteString(wizardHintStyle.Render("Arrow keys or j/k to move - Enter to confirm"))
	case stepPackages:
		b.WriteString(wizardSectionStyle.Render("Step 3 - Select packages"))
		b.WriteString("\n")
		if len(m.packageCandidates) == 0 {
			b.WriteString("No package candidates found for this root.\n")
			b.WriteString(wizardHintStyle.Render("Enter to continue"))
			break
		}
		for i, pkg := range m.packageCandidates {
			cursor := " "
			if i == m.pkgCursor {
				cursor = ">"
			}
			marker := "[ ]"
			if m.pkgSelected[i] {
				marker = "[x]"
			}
			b.WriteString(fmt.Sprintf("%s %s %s [%s]\n", cursor, marker, pkg.Name, pkg.PackageName))
		}
		b.WriteString("\n")
		b.WriteString(wizardHintStyle.Render("Space to toggle - Enter to continue"))
	case stepBranches:
		b.WriteString(m.branchesView())
	case stepReview:
		b.WriteString(m.reviewView())
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
	switch msg.String() {
	case "up", "k":
		if m.rootIndex > 0 {
			m.rootIndex--
		}
		m.errMsg = ""
	case "down", "j":
		if m.rootIndex < len(m.repos)-1 {
			m.rootIndex++
		}
		m.errMsg = ""
	case "enter":
		selected := m.selectedPackageSelectors()
		m.refreshPackageCandidates(selected)
		m.step = stepPackages
		return m, nil
	case "esc":
		m.cancelled = true
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

func (m createWizardModel) updatePackages(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

	switch msg.String() {
	case "up", "k":
		if m.pkgCursor > 0 {
			m.pkgCursor--
		}
		m.errMsg = ""
	case "down", "j":
		if m.pkgCursor < len(m.packageCandidates)-1 {
			m.pkgCursor++
		}
		m.errMsg = ""
	case " ":
		m.pkgSelected[m.pkgCursor] = !m.pkgSelected[m.pkgCursor]
		m.errMsg = ""
	case "enter":
		selected := m.selectedPackagesFromMap()
		if len(selected) == 0 {
			m.errMsg = "Select at least one package."
			return m, nil
		}
		m.errMsg = ""
		m.selectedPackages = selected
		m.step = stepBranches
		m.prepareBranchFieldInput()
	case "esc":
		m.cancelled = true
		m.done = true
		return m, tea.Quit
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
	labels := []string{"1.Name", "2.Root", "3.Packages", "4.Branches", "5.Review"}
	for i := range labels {
		if i == int(m.step) {
			labels[i] = wizardProgressActiveStyle.Render(labels[i])
			continue
		}
		labels[i] = wizardProgressIdleStyle.Render(labels[i])
	}
	return "\n" + strings.Join(labels, "  ")
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
	b.WriteString(renderTable(
		[]string{"Setting", "Value"},
		[][]string{
			{"Workspace", m.name},
			{"Root", m.repos[m.rootIndex].Name},
			{"Workspace file", fmt.Sprintf("%t", m.generateWorkspace)},
		},
	))
	b.WriteString("\n")

	b.WriteString(renderTable(
		[]string{"Role", "Repository", "Package", "Branch", "Base Branch"},
		m.reviewRows(),
	))
	b.WriteString("\n")
	b.WriteString(choiceDryRun)
	b.WriteString("\n")
	b.WriteString(choiceApply)
	b.WriteString("\n\n")
	b.WriteString(wizardHintStyle.Render("Arrow keys to choose - Enter to finish"))
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
