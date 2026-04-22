package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/EndersonPro/flutree/internal/domain"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type AddRepoWizardInput struct {
	WorkspaceName              string
	RootBranch                 string
	InitialSelectors           []string
	InitialPackageBranchSource map[string]string
	InitialPackageBase         map[string]string
	InitialSyncPolicy          domain.AddRepoSyncPolicy
}

type AddRepoWizardResult struct {
	Cancelled           bool
	Apply               bool
	RepoSelectors       []string
	PackageBranchSource map[string]string
	PackageBaseBranch   map[string]string
	SyncPolicy          domain.AddRepoSyncPolicy
}

type addRepoWizardStep int

const (
	addRepoWizardStepSelectRepos addRepoWizardStep = iota
	addRepoWizardStepReview
	addRepoWizardStepRepoOptions
)

const (
	addRepoWizardFinalCancel = iota
	addRepoWizardFinalApply
)

type addRepoOption struct {
	SourceBranch string
	BaseBranch   string
}

type addRepoWizardModel struct {
	step addRepoWizardStep

	workspaceName string
	rootBranch    string
	repos         []domain.DiscoveredFlutterRepo

	cursor   int
	selected map[int]bool

	selectedRepos []domain.DiscoveredFlutterRepo
	repoOptions   map[string]addRepoOption
	repoIndex     int
	optionField   int

	finalChoice int
	syncPolicy  domain.AddRepoSyncPolicy

	input  textinput.Model
	errMsg string

	done      bool
	cancelled bool
}

func RunAddRepoWizard(input AddRepoWizardInput, repos []domain.DiscoveredFlutterRepo) (AddRepoWizardResult, error) {
	if len(repos) == 0 {
		return AddRepoWizardResult{}, domain.NewError(domain.CategoryPrecondition, 3, "No additional repositories available to attach.", "All discoverable repositories are already attached.", nil)
	}

	model := newAddRepoWizardModel(input, repos)
	resultModel, err := tea.NewProgram(model).Run()
	if err != nil {
		return AddRepoWizardResult{}, domain.NewError(domain.CategoryUnexpected, 1, "Interactive add-repo flow failed.", "Retry the command or switch to --non-interactive mode.", err)
	}

	finalModel, ok := resultModel.(addRepoWizardModel)
	if !ok {
		return AddRepoWizardResult{}, domain.NewError(domain.CategoryUnexpected, 1, "Invalid interactive add-repo state.", "Retry the command.", nil)
	}

	return finalModel.result(), nil
}

func newAddRepoWizardModel(input AddRepoWizardInput, repos []domain.DiscoveredFlutterRepo) addRepoWizardModel {
	orderedRepos := append([]domain.DiscoveredFlutterRepo(nil), repos...)
	sort.Slice(orderedRepos, func(i, j int) bool {
		if orderedRepos[i].Name != orderedRepos[j].Name {
			return orderedRepos[i].Name < orderedRepos[j].Name
		}
		return orderedRepos[i].RepoRoot < orderedRepos[j].RepoRoot
	})

	selected := map[int]bool{}
	for i, repo := range orderedRepos {
		for _, selector := range input.InitialSelectors {
			if matchesSelector(repo, selector) {
				selected[i] = true
				break
			}
		}
	}
	if len(selected) == 0 && len(orderedRepos) > 0 {
		selected[0] = true
	}

	typed := textinput.New()
	typed.Focus()

	syncPolicy := input.InitialSyncPolicy
	if syncPolicy == "" {
		syncPolicy = domain.AddRepoSyncAuto
	}

	m := addRepoWizardModel{
		step:          addRepoWizardStepSelectRepos,
		workspaceName: strings.TrimSpace(input.WorkspaceName),
		rootBranch:    normalizeWizardBranch(input.RootBranch, "main"),
		repos:         orderedRepos,
		selected:      selected,
		repoOptions:   map[string]addRepoOption{},
		finalChoice:   addRepoWizardFinalApply,
		syncPolicy:    syncPolicy,
		input:         typed,
	}

	m.selectedRepos = m.selectedReposFromMap()
	m.seedRepoOptions(input.InitialPackageBranchSource, input.InitialPackageBase)
	return m
}

func (m addRepoWizardModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m addRepoWizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.cancelled = true
			m.done = true
			return m, tea.Quit
		}

		switch m.step {
		case addRepoWizardStepSelectRepos:
			return m.updateSelectRepos(msg)
		case addRepoWizardStepRepoOptions:
			return m.updateRepoOptions(msg)
		case addRepoWizardStepReview:
			return m.updateReview(msg)
		}
	}

	if m.step == addRepoWizardStepRepoOptions {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m addRepoWizardModel) View() string {
	var b strings.Builder
	b.WriteString(wizardTitleStyle.Render("flutree add-repo"))
	b.WriteString("\n")
	b.WriteString(wizardSubtitleStyle.Render("Interactive repository attachment wizard"))
	b.WriteString(m.progressLabel())
	b.WriteString("\n\n")

	if m.errMsg != "" {
		b.WriteString(wizardErrorStyle.Render("Error: " + m.errMsg))
		b.WriteString("\n\n")
	}

	switch m.step {
	case addRepoWizardStepSelectRepos:
		b.WriteString(m.selectReposView())
	case addRepoWizardStepRepoOptions:
		b.WriteString(m.repoOptionsView())
	case addRepoWizardStepReview:
		b.WriteString(m.reviewView())
	}

	return b.String()
}

func (m addRepoWizardModel) updateSelectRepos(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		m.errMsg = ""
	case "down", "j":
		if m.cursor < len(m.repos)-1 {
			m.cursor++
		}
		m.errMsg = ""
	case " ":
		m.selected[m.cursor] = !m.selected[m.cursor]
		m.errMsg = ""
	case "enter":
		selected := m.selectedReposFromMap()
		if len(selected) == 0 {
			m.errMsg = "Select at least one repository before continuing."
			return m, nil
		}
		m.selectedRepos = selected
		m.ensureSelectedRepoOptions()
		m.step = addRepoWizardStepReview
		m.errMsg = ""
		return m, nil
	case "esc":
		m.cancelled = true
		m.done = true
		return m, tea.Quit
	}

	return m, nil
}

func (m addRepoWizardModel) updateRepoOptions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.cancelled = true
		m.done = true
		return m, tea.Quit
	case tea.KeyEnter:
		value := strings.TrimSpace(m.input.Value())
		if value == "" {
			if m.optionField == 0 {
				m.errMsg = "Source branch cannot be empty."
			} else {
				m.errMsg = "Base branch cannot be empty."
			}
			return m, nil
		}

		repo := m.selectedRepos[m.repoIndex]
		option := m.repoOptions[repo.RepoRoot]
		if m.optionField == 0 {
			option.SourceBranch = value
			m.optionField = 1
		} else {
			option.BaseBranch = value
			m.repoOptions[repo.RepoRoot] = option
			m.repoIndex++
			m.optionField = 0
			if m.repoIndex >= len(m.selectedRepos) {
				m.done = true
				m.errMsg = ""
				return m, tea.Quit
			}
		}
		m.repoOptions[repo.RepoRoot] = option
		m.errMsg = ""
		m.prepareRepoOptionInput()
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m addRepoWizardModel) updateReview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "left", "h":
		m.syncPolicy = cycleSyncPolicy(m.syncPolicy, -1)
	case "right", "l":
		m.syncPolicy = cycleSyncPolicy(m.syncPolicy, 1)
	case "up", "k":
		m.finalChoice = addRepoWizardFinalCancel
	case "down", "j":
		m.finalChoice = addRepoWizardFinalApply
	case "s":
		m.syncPolicy = cycleSyncPolicy(m.syncPolicy, 1)
	case "enter":
		if m.finalChoice == addRepoWizardFinalCancel {
			m.cancelled = true
			m.done = true
			return m, tea.Quit
		}
		m.repoIndex = 0
		m.optionField = 0
		m.ensureSelectedRepoOptions()
		m.prepareRepoOptionInput()
		m.step = addRepoWizardStepRepoOptions
		m.errMsg = ""
		return m, nil
	case "esc":
		m.cancelled = true
		m.done = true
		return m, tea.Quit
	}

	return m, nil
}

func (m addRepoWizardModel) progressLabel() string {
	labels := []string{"1.Select repos", "2.Review", "3.Branches"}
	for i := range labels {
		if i == int(m.step) {
			labels[i] = wizardProgressActiveStyle.Render(labels[i])
			continue
		}
		labels[i] = wizardProgressIdleStyle.Render(labels[i])
	}
	return "\n" + strings.Join(labels, "  ")
}

func (m addRepoWizardModel) selectReposView() string {
	var b strings.Builder
	b.WriteString(wizardSectionStyle.Render("Step 1 - Select repositories"))
	b.WriteString("\n")
	for i, repo := range m.repos {
		cursor := " "
		if i == m.cursor {
			cursor = ">"
		}
		marker := "[ ]"
		if m.selected[i] {
			marker = "[x]"
		}
		b.WriteString(fmt.Sprintf("%s %s %s [%s] (%s)\n", cursor, marker, repo.Name, repo.PackageName, repo.RepoRoot))
	}
	b.WriteString("\n")
	b.WriteString(wizardHintStyle.Render("Arrow keys or j/k to move • Space to toggle • Enter to continue • Esc to cancel"))
	return b.String()
}

func (m addRepoWizardModel) repoOptionsView() string {
	if len(m.selectedRepos) == 0 || m.repoIndex >= len(m.selectedRepos) {
		return wizardSectionStyle.Render("Step 3 - Configure branches") + "\n" + wizardHintStyle.Render("Finalizing selection...")
	}

	repo := m.selectedRepos[m.repoIndex]
	fieldLabel := "Source branch"
	if m.optionField == 1 {
		fieldLabel = "Base branch"
	}

	var b strings.Builder
	b.WriteString(wizardSectionStyle.Render("Step 3 - Configure branches"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("Repository %d/%d: %s\n", m.repoIndex+1, len(m.selectedRepos), repo.Name))
	b.WriteString(fmt.Sprintf("%s for %s\n", fieldLabel, repo.Name))
	b.WriteString(m.input.View())
	b.WriteString("\n\n")
	b.WriteString(wizardHintStyle.Render("Enter to continue • Esc to cancel"))
	return b.String()
}

func (m addRepoWizardModel) reviewView() string {
	choiceCancel := "(*) Cancel"
	choiceApply := "( ) Continue"
	if m.finalChoice == addRepoWizardFinalApply {
		choiceCancel = "( ) Cancel"
		choiceApply = "(*) Continue"
	}

	var b strings.Builder
	b.WriteString(wizardSectionStyle.Render("Step 2 - Review and confirm"))
	b.WriteString("\n")
	b.WriteString(renderTable(
		[]string{"Setting", "Value"},
		[][]string{
			{"Workspace", m.workspaceName},
			{"Root branch", m.rootBranch},
			{"Sync policy", string(m.syncPolicy)},
		},
	))
	b.WriteString("\n")
	b.WriteString(renderTable([]string{"Repository", "Package"}, m.reviewRows()))
	b.WriteString("\n")
	b.WriteString(choiceCancel)
	b.WriteString("\n")
	b.WriteString(choiceApply)
	b.WriteString("\n\n")
	b.WriteString(wizardHintStyle.Render("Up/Down to choose Cancel or Continue • Left/Right (or s) to change sync policy • Enter to proceed"))
	return b.String()
}

func (m *addRepoWizardModel) prepareRepoOptionInput() {
	if !m.input.Focused() {
		m.input.Focus()
	}

	repo := m.selectedRepos[m.repoIndex]
	current := m.repoOptions[repo.RepoRoot]
	if m.optionField == 0 {
		m.input.Prompt = "Source branch: "
		m.input.SetValue(normalizeWizardBranch(current.SourceBranch, m.rootBranch))
	} else {
		m.input.Prompt = "Base branch: "
		m.input.SetValue(normalizeWizardBranch(current.BaseBranch, "main"))
	}
	m.input.CursorEnd()
}

func (m *addRepoWizardModel) seedRepoOptions(initialSource, initialBase map[string]string) {
	if initialSource == nil {
		initialSource = map[string]string{}
	}
	if initialBase == nil {
		initialBase = map[string]string{}
	}

	for _, repo := range m.selectedRepos {
		source := strings.TrimSpace(initialSource[repo.RepoRoot])
		if source == "" {
			source = strings.TrimSpace(initialSource[repo.Name])
		}
		base := strings.TrimSpace(initialBase[repo.RepoRoot])
		if base == "" {
			base = strings.TrimSpace(initialBase[repo.Name])
		}
		m.repoOptions[repo.RepoRoot] = addRepoOption{
			SourceBranch: normalizeWizardBranch(source, m.rootBranch),
			BaseBranch:   normalizeWizardBranch(base, "main"),
		}
	}
}

func (m *addRepoWizardModel) ensureSelectedRepoOptions() {
	options := map[string]addRepoOption{}
	for _, repo := range m.selectedRepos {
		current := m.repoOptions[repo.RepoRoot]
		options[repo.RepoRoot] = addRepoOption{
			SourceBranch: normalizeWizardBranch(current.SourceBranch, m.rootBranch),
			BaseBranch:   normalizeWizardBranch(current.BaseBranch, "main"),
		}
	}
	m.repoOptions = options
}

func (m addRepoWizardModel) selectedReposFromMap() []domain.DiscoveredFlutterRepo {
	selected := make([]domain.DiscoveredFlutterRepo, 0, len(m.selected))
	seenRoots := map[string]struct{}{}
	for i, repo := range m.repos {
		if !m.selected[i] {
			continue
		}
		key := domain.NormalizePath(repo.RepoRoot)
		if _, exists := seenRoots[key]; exists {
			continue
		}
		seenRoots[key] = struct{}{}
		selected = append(selected, repo)
	}
	sort.Slice(selected, func(i, j int) bool {
		if selected[i].Name != selected[j].Name {
			return selected[i].Name < selected[j].Name
		}
		return selected[i].RepoRoot < selected[j].RepoRoot
	})
	return selected
}

func (m addRepoWizardModel) reviewRows() [][]string {
	rows := make([][]string, 0, len(m.selectedRepos))
	for _, repo := range m.selectedRepos {
		rows = append(rows, []string{
			repo.Name,
			repo.PackageName,
		})
	}
	return rows
}

func (m addRepoWizardModel) result() AddRepoWizardResult {
	if m.cancelled {
		return AddRepoWizardResult{Cancelled: true}
	}

	repos := m.selectedRepos
	if len(repos) == 0 {
		repos = m.selectedReposFromMap()
	}

	selectors := make([]string, 0, len(repos))
	sourceMap := map[string]string{}
	baseMap := map[string]string{}
	for _, repo := range repos {
		selectors = append(selectors, repo.RepoRoot)
		option := m.repoOptions[repo.RepoRoot]
		sourceMap[repo.RepoRoot] = normalizeWizardBranch(option.SourceBranch, m.rootBranch)
		baseMap[repo.RepoRoot] = normalizeWizardBranch(option.BaseBranch, "main")
	}

	return AddRepoWizardResult{
		Cancelled:           false,
		Apply:               m.done && !m.cancelled,
		RepoSelectors:       dedupWizardSelectors(selectors),
		PackageBranchSource: sourceMap,
		PackageBaseBranch:   baseMap,
		SyncPolicy:          m.syncPolicy,
	}
}

func dedupWizardSelectors(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		normalized := domain.NormalizePath(trimmed)
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func cycleSyncPolicy(current domain.AddRepoSyncPolicy, direction int) domain.AddRepoSyncPolicy {
	values := []domain.AddRepoSyncPolicy{domain.AddRepoSyncAuto, domain.AddRepoSyncAlways, domain.AddRepoSyncNever}
	index := 0
	for i, value := range values {
		if value == current {
			index = i
			break
		}
	}

	index += direction
	if index < 0 {
		index = len(values) - 1
	}
	if index >= len(values) {
		index = 0
	}
	return values[index]
}

func normalizeWizardBranch(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(fallback)
}
