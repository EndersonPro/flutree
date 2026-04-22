package app

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/EndersonPro/flutree/internal/domain"
)

type AddRepoService struct {
	git      GitPort
	registry RegistryPort
	prompt   PromptPort
}

type AddRepoSelectionContextInput struct {
	WorkspaceName  string
	ExecutionScope string
}

type AddRepoSelectionContext struct {
	WorkspaceName string
	RootBranch    string
	Candidates    []domain.DiscoveredFlutterRepo
}

type addRepoRuntimeContext struct {
	rootRecord       domain.RegistryRecord
	containerPath    string
	existingPackages []domain.RegistryRecord
	candidates       []domain.DiscoveredFlutterRepo
}

func NewAddRepoService(git GitPort, registry RegistryPort, prompt PromptPort) *AddRepoService {
	return &AddRepoService{git: git, registry: registry, prompt: prompt}
}

func (s *AddRepoService) BuildSelectionContext(input AddRepoSelectionContextInput) (AddRepoSelectionContext, error) {
	ctx, err := s.buildRuntimeContext(input)
	if err != nil {
		return AddRepoSelectionContext{}, err
	}

	return AddRepoSelectionContext{
		WorkspaceName: ctx.rootRecord.Name,
		RootBranch:    ctx.rootRecord.Branch,
		Candidates:    append([]domain.DiscoveredFlutterRepo(nil), ctx.candidates...),
	}, nil
}

func (s *AddRepoService) Run(input domain.AddRepoInput) (domain.AddRepoResult, error) {
	workspaceName := strings.TrimSpace(input.WorkspaceName)
	if workspaceName == "" {
		return domain.AddRepoResult{}, domain.NewError(domain.CategoryInput, 2, "Missing workspace name.", "Usage: flutree add-repo <workspace> --repo <selector>", nil)
	}
	if _, isPackage := splitPackageRecordName(workspaceName); isPackage {
		return domain.AddRepoResult{}, domain.NewError(domain.CategoryInput, 2, "Add-repo requires root workspace name.", "Use root workspace name shown by `flutree list`.", nil)
	}

	ctx, err := s.buildRuntimeContext(AddRepoSelectionContextInput{WorkspaceName: workspaceName, ExecutionScope: input.ExecutionScope})
	if err != nil {
		return domain.AddRepoResult{}, err
	}

	rootRecord := ctx.rootRecord
	containerPath := ctx.containerPath
	existingPackages := ctx.existingPackages
	candidates := ctx.candidates

	selectors := dedupStringsPreservingOrder(input.RepoSelectors)
	if len(selectors) == 0 {
		if input.NonInteractive {
			return domain.AddRepoResult{}, domain.NewError(domain.CategoryInput, 2, "Repository selection is required in non-interactive mode.", "Pass one or more --repo selectors.", nil)
		}
		choices := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			choices = append(choices, repoLabel(candidate))
		}
		selectedChoices, err := s.prompt.SelectPackages("Select repositories to attach", choices, false)
		if err != nil {
			return domain.AddRepoResult{}, err
		}
		for _, selected := range selectedChoices {
			for _, candidate := range candidates {
				if repoLabel(candidate) == selected {
					selectors = append(selectors, candidate.RepoRoot)
				}
			}
		}
		selectors = dedupStringsPreservingOrder(selectors)
	}

	selectedRepos := make([]domain.DiscoveredFlutterRepo, 0, len(selectors))
	for _, selector := range selectors {
		repo, found := findRepoBySelector(candidates, selector)
		if !found {
			return domain.AddRepoResult{}, domain.NewError(domain.CategoryInput, 2, "Unknown --repo selector: "+selector+".", "Use discoverable repository name/package/path.", nil)
		}
		selectedRepos = append(selectedRepos, repo)
	}
	selectedRepos = dedupRepos(selectedRepos)
	sort.Slice(selectedRepos, func(i, j int) bool {
		if selectedRepos[i].Name != selectedRepos[j].Name {
			return selectedRepos[i].Name < selectedRepos[j].Name
		}
		return selectedRepos[i].RepoRoot < selectedRepos[j].RepoRoot
	})

	if input.NonInteractive && strings.TrimSpace(rootRecord.Branch) == "" {
		return domain.AddRepoResult{}, domain.NewError(
			domain.CategoryInput,
			2,
			"Cannot resolve target branch for attached repositories in non-interactive mode.",
			"Root workspace branch is empty in registry. Provide --package-branch-source for each --repo or fix registry branch metadata.",
			nil,
		)
	}
	rootBranch := normalizeBranchName(rootRecord.Branch)

	syncWithRemote, err := s.resolveSyncPolicy(input)
	if err != nil {
		return domain.AddRepoResult{}, err
	}

	createSvc := NewCreateService(s.git, s.registry, s.prompt)
	newPlans := []domain.PlannedWorktree{}
	hasExplicitPackageBase := len(input.PackageBaseBranch) > 0
	for _, repo := range selectedRepos {
		branch, base, resolveErr := s.resolvePackageBranching(input, repo, rootBranch, hasExplicitPackageBase)
		if resolveErr != nil {
			return domain.AddRepoResult{}, resolveErr
		}
		if input.NonInteractive && (strings.TrimSpace(branch) == "" || strings.TrimSpace(base) == "") {
			return domain.AddRepoResult{}, domain.NewError(
				domain.CategoryInput,
				2,
				"Non-interactive add-repo requires deterministic branch source and base values.",
				"Provide --package-branch-source and --package-base explicitly for every selected repository.",
				nil,
			)
		}
		newPlans = append(newPlans, domain.PlannedWorktree{
			Repo:       repo,
			Role:       "package",
			Path:       filepath.Join(containerPath, "packages", repo.Name),
			Branch:     normalizeBranchName(branch),
			BaseBranch: normalizeBranchName(base),
		})
	}

	created := []domain.PlannedWorktree{}
	persistedNames := []string{}
	rollback := func() {
		for i := len(persistedNames) - 1; i >= 0; i-- {
			_, _ = s.registry.Remove(persistedNames[i])
		}
		for i := len(created) - 1; i >= 0; i-- {
			_ = s.git.RemoveWorktree(created[i].Repo.RepoRoot, created[i].Path, true)
		}
	}

	rootFilePatterns := mergeRootFilePatterns(input.RootFiles)
	for _, plan := range newPlans {
		if err := os.MkdirAll(filepath.Dir(plan.Path), 0o755); err != nil {
			rollback()
			return domain.AddRepoResult{}, domain.NewError(domain.CategoryPersistence, 5, "Failed to create package worktree directory.", plan.Path, err)
		}
		if err := createSvc.createPlannedWorktree(plan, domain.CreateApplyOptions{
			NonInteractive:      input.NonInteractive,
			ReuseExistingBranch: input.ReuseExistingBranch,
			SyncWithRemote:      syncWithRemote,
		}); err != nil {
			rollback()
			message := "Failed to create attached worktree for repository '" + plan.Repo.Name + "'."
			if strings.Contains(strings.ToLower(err.Error()), "sync") {
				message = "Failed to sync repository '" + plan.Repo.Name + "' before attached worktree creation."
			}
			return domain.AddRepoResult{}, domain.NewError(
				domain.CategoryGit,
				1,
				message,
				"Branch: "+plan.Branch+" | Base: "+plan.BaseBranch,
				err,
			)
		}
		if err := copyRootFiles(plan.Repo.RepoRoot, plan.Path, rootFilePatterns); err != nil {
			rollback()
			return domain.AddRepoResult{}, domain.NewError(domain.CategoryPersistence, 5, "Failed to copy root files into attached worktree.", plan.Path, err)
		}
		created = append(created, plan)

		record := domain.RegistryRecord{
			Name:     rootRecord.Name + "__pkg__" + plan.Repo.Name,
			Branch:   plan.Branch,
			Path:     plan.Path,
			RepoRoot: plan.Repo.RepoRoot,
			Status:   "active",
		}
		if err := s.registry.Upsert(record); err != nil {
			rollback()
			return domain.AddRepoResult{}, err
		}
		persistedNames = append(persistedNames, record.Name)
	}

	allPackages := append([]domain.RegistryRecord{}, existingPackages...)
	for _, plan := range newPlans {
		allPackages = append(allPackages, domain.RegistryRecord{
			Name:     rootRecord.Name + "__pkg__" + plan.Repo.Name,
			Branch:   plan.Branch,
			Path:     plan.Path,
			RepoRoot: plan.Repo.RepoRoot,
			Status:   "active",
		})
	}

	overridePackages := []domain.PlannedWorktree{}
	for _, pkg := range allPackages {
		repoName := filepath.Base(pkg.Path)
		packageName := readPackageNameFromWorktree(pkg.Path)
		overridePackages = append(overridePackages, domain.PlannedWorktree{
			Repo: domain.DiscoveredFlutterRepo{
				Name:        repoName,
				RepoRoot:    pkg.RepoRoot,
				PackageName: packageName,
			},
			Role:   "package",
			Path:   pkg.Path,
			Branch: pkg.Branch,
		})
	}

	overridePath := filepath.Join(rootRecord.Path, overrideFileName)
	if err := writeOverrideFile(overridePath, rootRecord.Path, overridePackages); err != nil {
		rollback()
		return domain.AddRepoResult{}, domain.NewError(domain.CategoryPersistence, 5, "Failed to update pubspec_overrides.yaml.", overridePath, err)
	}
	if err := ensureGitignoreContains(rootRecord.Path, filepath.Base(overridePath)); err != nil {
		rollback()
		return domain.AddRepoResult{}, domain.NewError(domain.CategoryPersistence, 5, "Failed to update .gitignore for pubspec_overrides.yaml.", filepath.Join(rootRecord.Path, ".gitignore"), err)
	}

	workspacePackages := make([]domain.PlannedWorktree, 0, len(allPackages))
	for _, pkg := range allPackages {
		workspacePackages = append(workspacePackages, domain.PlannedWorktree{Path: pkg.Path})
	}
	workspacePath := filepath.Join(containerPath, rootRecord.Name+".code-workspace")
	if err := updateWorkspaceIfExists(workspacePath, rootRecord.Path, workspacePackages, containerPath); err != nil {
		rollback()
		return domain.AddRepoResult{}, domain.NewError(domain.CategoryPersistence, 5, "Failed to update VSCode workspace file.", workspacePath, err)
	}

	added := make([]string, 0, len(newPlans))
	for _, plan := range newPlans {
		added = append(added, plan.Repo.Name)
	}
	return domain.AddRepoResult{
		WorkspaceName:  rootRecord.Name,
		AddedRepos:     added,
		OverridePath:   overridePath,
		SelectedBranch: rootRecord.Branch,
	}, nil
}

func (s *AddRepoService) buildRuntimeContext(input AddRepoSelectionContextInput) (addRepoRuntimeContext, error) {
	workspaceName := strings.TrimSpace(input.WorkspaceName)
	if workspaceName == "" {
		return addRepoRuntimeContext{}, domain.NewError(domain.CategoryInput, 2, "Missing workspace name.", "Usage: flutree add-repo <workspace> --repo <selector>", nil)
	}

	records, err := s.registry.ListRecords()
	if err != nil {
		return addRepoRuntimeContext{}, err
	}

	rootRecord, ok := findRecordByName(records, workspaceName)
	if !ok {
		return addRepoRuntimeContext{}, domain.NewError(domain.CategoryPrecondition, 3, "Managed workspace '"+workspaceName+"' was not found in registry.", "Run `flutree list` to inspect managed entries.", nil)
	}
	if _, isPackage := splitPackageRecordName(rootRecord.Name); isPackage {
		return addRepoRuntimeContext{}, domain.NewError(domain.CategoryInput, 2, "Add-repo requires root workspace name.", "Use root workspace name shown by `flutree list`.", nil)
	}

	containerPath, removeContainer, err := completionContainerPath(rootRecord)
	if err != nil {
		return addRepoRuntimeContext{}, err
	}
	if !removeContainer {
		return addRepoRuntimeContext{}, domain.NewError(domain.CategoryPrecondition, 3, "Unable to determine workspace container path.", "Expected root worktree path in '<container>/root/<repository>'.", nil)
	}

	discovered, err := s.git.DiscoverFlutterRepos(input.ExecutionScope)
	if err != nil {
		return addRepoRuntimeContext{}, err
	}

	rootRepo, ok := findRepoBySelector(discovered, rootRecord.RepoRoot)
	if !ok {
		return addRepoRuntimeContext{}, domain.NewError(domain.CategoryPrecondition, 3, "Root repository is not discoverable in provided scope.", "Scope: "+input.ExecutionScope, nil)
	}

	existingPackages := workspacePackageRecords(rootRecord.Name, records)
	existingRepoRoots := map[string]struct{}{filepath.Clean(rootRecord.RepoRoot): {}}
	for _, rec := range existingPackages {
		existingRepoRoots[filepath.Clean(rec.RepoRoot)] = struct{}{}
	}

	candidates := []domain.DiscoveredFlutterRepo{}
	for _, repo := range discovered {
		if filepath.Clean(repo.RepoRoot) == filepath.Clean(rootRepo.RepoRoot) {
			continue
		}
		if _, exists := existingRepoRoots[filepath.Clean(repo.RepoRoot)]; exists {
			continue
		}
		candidates = append(candidates, repo)
	}
	if len(candidates) == 0 {
		return addRepoRuntimeContext{}, domain.NewError(domain.CategoryPrecondition, 3, "No additional repositories available to attach.", "All discoverable repositories are already attached.", nil)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Name != candidates[j].Name {
			return candidates[i].Name < candidates[j].Name
		}
		return candidates[i].RepoRoot < candidates[j].RepoRoot
	})

	return addRepoRuntimeContext{
		rootRecord:       rootRecord,
		containerPath:    containerPath,
		existingPackages: existingPackages,
		candidates:       candidates,
	}, nil
}

func (s *AddRepoService) resolveSyncPolicy(input domain.AddRepoInput) (bool, error) {
	policy := input.SyncPolicy
	if policy == "" {
		policy = domain.AddRepoSyncAuto
	}
	switch policy {
	case domain.AddRepoSyncAlways:
		return true, nil
	case domain.AddRepoSyncNever:
		return false, nil
	case domain.AddRepoSyncAuto:
		if input.NonInteractive {
			return false, nil
		}
		confirmSync, err := s.prompt.Confirm(
			"Update local branches from origin before creating attached worktrees?",
			false,
			false,
		)
		if err != nil {
			return false, err
		}
		return confirmSync, nil
	default:
		return false, domain.NewError(domain.CategoryInput, 2, "Unknown add-repo sync policy.", "Use auto|always|never.", nil)
	}
}

func (s *AddRepoService) resolvePackageBranching(input domain.AddRepoInput, repo domain.DiscoveredFlutterRepo, rootBranch string, hasExplicitPackageBase bool) (string, string, error) {
	branch := strings.TrimSpace(input.PackageBranchSource[repo.Name])
	if branch == "" {
		branch = strings.TrimSpace(input.PackageBranchSource[repo.RepoRoot])
	}
	if branch == "" {
		if !input.NonInteractive && !hasExplicitPackageBase {
			resolved, err := s.prompt.AskText("Target branch for package '"+repo.Name+"'", rootBranch, false)
			if err != nil {
				return "", "", err
			}
			branch = resolved
		} else {
			branch = rootBranch
		}
	}

	base := strings.TrimSpace(input.PackageBaseBranch[repo.Name])
	if base == "" {
		base = strings.TrimSpace(input.PackageBaseBranch[repo.RepoRoot])
	}
	if base == "" {
		switch {
		case hasExplicitPackageBase:
			base = "main"
		case input.NonInteractive:
			base = "main"
		default:
			resolved, err := s.prompt.AskText("Base branch for package '"+repo.Name+"'", "main", false)
			if err != nil {
				return "", "", err
			}
			base = resolved
		}
	}

	return branch, base, nil
}

func workspacePackageRecords(rootName string, records []domain.RegistryRecord) []domain.RegistryRecord {
	prefix := rootName + "__pkg__"
	out := []domain.RegistryRecord{}
	for _, record := range records {
		if strings.HasPrefix(record.Name, prefix) {
			out = append(out, record)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func readPackageNameFromWorktree(repoPath string) string {
	pubspecPath := filepath.Join(repoPath, "pubspec.yaml")
	content, err := os.ReadFile(pubspecPath)
	if err != nil {
		return filepath.Base(repoPath)
	}
	for _, rawLine := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || !strings.HasPrefix(line, "name:") {
			continue
		}
		token := strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		token = strings.Trim(token, "\"'")
		if token != "" {
			return token
		}
	}
	return filepath.Base(repoPath)
}

func updateWorkspaceIfExists(workspacePath, rootPath string, packages []domain.PlannedWorktree, containerPath string) error {
	if _, err := os.Stat(workspacePath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	rootPlan := domain.PlannedWorktree{Path: rootPath}
	folders := buildWorkspaceFolders(rootPlan, packages, containerPath)
	return writeWorkspace(workspacePath, folders)
}
