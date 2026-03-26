package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/EndersonPro/flutree/internal/domain"
)

type CreateService struct {
	git      GitPort
	registry RegistryPort
	prompt   PromptPort
}

const overrideFileName = "pubspec_overrides.yaml"

func NewCreateService(git GitPort, registry RegistryPort, prompt PromptPort) *CreateService {
	return &CreateService{git: git, registry: registry, prompt: prompt}
}

func (s *CreateService) BuildDryPlan(input domain.CreateInput) (domain.CreateDryPlan, error) {
	normalizedName := normalizeWorktreeName(input.Name)
	rootBranchInput := strings.TrimSpace(input.Branch)
	if rootBranchInput == "" {
		rootBranchInput = defaultBranchFor(normalizedName)
	}
	rootBranch := normalizeBranchName(rootBranchInput)
	baseBranch := normalizeBranchName(input.BaseBranch)

	repos, err := s.git.DiscoverFlutterRepos(input.ExecutionScope)
	if err != nil {
		return domain.CreateDryPlan{}, err
	}

	rootRepo, err := s.resolveRootRepo(repos, input.RootSelector, input.NonInteractive)
	if err != nil {
		return domain.CreateDryPlan{}, err
	}

	packages := []domain.DiscoveredFlutterRepo{}
	if !input.NoPackage {
		packages, err = s.resolvePackageRepos(repos, rootRepo, input.PackageSelectors, input.NonInteractive)
		if err != nil {
			return domain.CreateDryPlan{}, err
		}
	}

	container := filepath.Join(destinationRoot(), normalizedName)
	if _, err := os.Stat(container); err == nil {
		return domain.CreateDryPlan{}, domain.NewError(
			domain.CategoryPrecondition, 3,
			"Target worktree container already exists.",
			container,
			nil,
		)
	}

	rootPlan := domain.PlannedWorktree{
		Repo:       rootRepo,
		Role:       "root",
		Path:       filepath.Join(container, "root", rootRepo.Name),
		Branch:     rootBranch,
		BaseBranch: baseBranch,
	}

	packagePlans := make([]domain.PlannedWorktree, 0, len(packages))
	for _, pkg := range packages {
		branch := rootBranch
		pkgBase, ok := input.PackageBaseBranch[pkg.Name]
		if !ok {
			pkgBase, ok = input.PackageBaseBranch[pkg.RepoRoot]
		}
		if !ok {
			pkgBase, err = s.prompt.AskText(
				"Base branch for package '"+pkg.Name+"'",
				"develop",
				input.NonInteractive,
			)
			if err != nil {
				return domain.CreateDryPlan{}, err
			}
		}
		packagePlans = append(packagePlans, domain.PlannedWorktree{
			Repo:       pkg,
			Role:       "package",
			Path:       filepath.Join(container, "packages", pkg.Name),
			Branch:     branch,
			BaseBranch: normalizeBranchName(pkgBase),
		})
	}
	sort.Slice(packagePlans, func(i, j int) bool { return packagePlans[i].Repo.Name < packagePlans[j].Repo.Name })

	overridePath := filepath.Join(rootPlan.Path, overrideFileName)
	overrideContent := buildOverrideContent(rootPlan, packagePlans)

	workspacePath := ""
	workspaceFolders := []string{}
	if input.GenerateWorkspace {
		workspacePath = filepath.Join(container, normalizedName+".code-workspace")
		workspaceFolders = buildWorkspaceFolders(rootPlan, packagePlans, container)
	}

	return domain.CreateDryPlan{
		NormalizedName:   normalizedName,
		ContainerPath:    container,
		Root:             rootPlan,
		Packages:         packagePlans,
		RootFiles:        mergeRootFilePatterns(input.RootFiles),
		OverridePath:     overridePath,
		OverrideContent:  overrideContent,
		WorkspacePath:    workspacePath,
		WorkspaceFolders: workspaceFolders,
	}, nil
}

func (s *CreateService) Apply(plan domain.CreateDryPlan, options domain.CreateApplyOptions) (domain.CreateResult, error) {
	records, err := s.registry.ListRecords()
	if err != nil {
		return domain.CreateResult{}, err
	}
	for _, rec := range records {
		if rec.Name == plan.NormalizedName {
			return domain.CreateResult{}, domain.NewError(
				domain.CategoryPrecondition, 3,
				"Managed worktree name already exists in registry.",
				"Choose another name or complete/remove the existing one.",
				nil,
			)
		}
	}

	created := []domain.PlannedWorktree{}
	persisted := []string{}
	rollback := func() {
		for i := len(persisted) - 1; i >= 0; i-- {
			_, _ = s.registry.Remove(persisted[i])
		}
		for i := len(created) - 1; i >= 0; i-- {
			_ = s.git.RemoveWorktree(created[i].Repo.RepoRoot, created[i].Path, true)
		}
	}

	if err := os.MkdirAll(filepath.Dir(plan.Root.Path), 0o755); err != nil {
		return domain.CreateResult{}, domain.NewError(domain.CategoryPersistence, 5, "Failed to create root worktree directory.", plan.Root.Path, err)
	}
	if err := s.createPlannedWorktree(plan.Root, options); err != nil {
		return domain.CreateResult{}, err
	}
	if err := copyRootFiles(plan.Root.Repo.RepoRoot, plan.Root.Path, plan.RootFiles); err != nil {
		rollback()
		return domain.CreateResult{}, domain.NewError(domain.CategoryPersistence, 5, "Failed to copy root files into root worktree.", plan.Root.Path, err)
	}
	created = append(created, plan.Root)

	for _, pkg := range plan.Packages {
		if err := os.MkdirAll(filepath.Dir(pkg.Path), 0o755); err != nil {
			rollback()
			return domain.CreateResult{}, domain.NewError(domain.CategoryPersistence, 5, "Failed to create package worktree directory.", pkg.Path, err)
		}
		if err := s.createPlannedWorktree(pkg, options); err != nil {
			rollback()
			return domain.CreateResult{}, err
		}
		if err := copyRootFiles(pkg.Repo.RepoRoot, pkg.Path, plan.RootFiles); err != nil {
			rollback()
			return domain.CreateResult{}, domain.NewError(domain.CategoryPersistence, 5, "Failed to copy root files into package worktree.", pkg.Path, err)
		}
		created = append(created, pkg)
	}

	if err := os.WriteFile(plan.OverridePath, []byte(plan.OverrideContent), 0o644); err != nil {
		rollback()
		return domain.CreateResult{}, domain.NewError(domain.CategoryPersistence, 5, "Failed to write pubspec_overrides.yaml.", plan.OverridePath, err)
	}

	if err := ensureGitignoreContains(plan.Root.Path, filepath.Base(plan.OverridePath)); err != nil {
		rollback()
		return domain.CreateResult{}, domain.NewError(domain.CategoryPersistence, 5, "Failed to update .gitignore for pubspec_overrides.yaml.", filepath.Join(plan.Root.Path, ".gitignore"), err)
	}

	if plan.WorkspacePath != "" {
		if err := writeWorkspace(plan.WorkspacePath, plan.WorkspaceFolders); err != nil {
			rollback()
			return domain.CreateResult{}, err
		}
	}

	rootRecord := domain.RegistryRecord{
		Name:     plan.NormalizedName,
		Branch:   plan.Root.Branch,
		Path:     plan.Root.Path,
		RepoRoot: plan.Root.Repo.RepoRoot,
		Status:   "active",
	}
	if err := s.registry.Upsert(rootRecord); err != nil {
		rollback()
		return domain.CreateResult{}, err
	}
	persisted = append(persisted, rootRecord.Name)

	for _, pkg := range plan.Packages {
		record := domain.RegistryRecord{
			Name:     fmt.Sprintf("%s__pkg__%s", plan.NormalizedName, pkg.Repo.Name),
			Branch:   pkg.Branch,
			Path:     pkg.Path,
			RepoRoot: pkg.Repo.RepoRoot,
			Status:   "active",
		}
		if err := s.registry.Upsert(record); err != nil {
			rollback()
			return domain.CreateResult{}, err
		}
		persisted = append(persisted, record.Name)
	}

	selected := make([]string, 0, len(plan.Packages))
	for _, pkg := range plan.Packages {
		selected = append(selected, pkg.Repo.PackageName)
	}
	return domain.CreateResult{
		Record:           rootRecord,
		NextStep:         "cd " + plan.Root.Path,
		SelectedPackages: selected,
		WorkspacePath:    plan.WorkspacePath,
	}, nil
}

func (s *CreateService) createPlannedWorktree(target domain.PlannedWorktree, options domain.CreateApplyOptions) error {
	exists, err := s.git.BranchExists(target.Repo.RepoRoot, target.Branch)
	if err != nil {
		return err
	}

	if exists {
		if options.NonInteractive {
			if !options.ReuseExistingBranch {
				return domain.NewError(
					domain.CategoryInput,
					2,
					"Existing branch reuse requires explicit --reuse-existing-branch in non-interactive mode.",
					"Branch '"+target.Branch+"' already exists. Re-run with --reuse-existing-branch to reuse it safely.",
					nil,
				)
			}
		} else {
			confirmMessage := "Branch '" + target.Branch + "' already exists for repo '" + target.Repo.Name + "'. Reuse it for this worktree?"
			confirmed, confirmErr := s.prompt.Confirm(confirmMessage, false, false)
			if confirmErr != nil {
				return confirmErr
			}
			if !confirmed {
				return domain.NewError(
					domain.CategoryInput,
					2,
					"Create cancelled by user while confirming branch reuse.",
					"Choose another branch or confirm reuse to continue.",
					nil,
				)
			}
		}

		if options.SyncWithRemote {
			if err := s.git.SyncBranchWithRemote(target.Repo.RepoRoot, target.Branch); err != nil {
				return err
			}
		}

		if err := s.git.CreateWorktreeExisting(target.Repo.RepoRoot, target.Path, target.Branch); err != nil {
			return err
		}
		return nil
	}

	startPoint := target.BaseBranch
	if options.SyncWithRemote {
		startPoint, err = s.git.SyncBaseBranch(target.Repo.RepoRoot, target.BaseBranch)
		if err != nil {
			return err
		}
	}
	if err := s.git.CreateWorktreeNew(target.Repo.RepoRoot, target.Path, target.Branch, startPoint); err != nil {
		return err
	}
	return nil
}

func (s *CreateService) resolveRootRepo(repos []domain.DiscoveredFlutterRepo, selector string, nonInteractive bool) (domain.DiscoveredFlutterRepo, error) {
	if selector != "" {
		if repo, ok := findRepoBySelector(repos, selector); ok {
			return repo, nil
		}
		return domain.DiscoveredFlutterRepo{}, domain.NewError(domain.CategoryInput, 2, "Unknown root repository selector: "+selector+".", "Use --root-repo with a discovered repository name or path.", nil)
	}

	choices := make([]string, 0, len(repos))
	for _, repo := range repos {
		choices = append(choices, repoLabel(repo))
	}
	choice, err := s.prompt.SelectOne("Select root repository", choices, nonInteractive)
	if err != nil {
		return domain.DiscoveredFlutterRepo{}, err
	}
	for _, repo := range repos {
		if repoLabel(repo) == choice {
			return repo, nil
		}
	}
	return domain.DiscoveredFlutterRepo{}, domain.NewError(domain.CategoryInput, 2, "Unknown root repository selection: "+choice+".", "Select a repository from the provided choices.", nil)
}

func (s *CreateService) resolvePackageRepos(repos []domain.DiscoveredFlutterRepo, root domain.DiscoveredFlutterRepo, selectors []string, nonInteractive bool) ([]domain.DiscoveredFlutterRepo, error) {
	candidates := []domain.DiscoveredFlutterRepo{}
	for _, repo := range repos {
		if filepath.Clean(repo.RepoRoot) == filepath.Clean(root.RepoRoot) {
			continue
		}
		candidates = append(candidates, repo)
	}
	if len(candidates) == 0 {
		return []domain.DiscoveredFlutterRepo{}, nil
	}

	if len(selectors) == 0 {
		if nonInteractive {
			return []domain.DiscoveredFlutterRepo{}, nil
		}
		choices := []string{}
		for _, c := range candidates {
			choices = append(choices, repoLabel(c))
		}
		selectedChoices, err := s.prompt.SelectPackages("Select package repositories", choices, false)
		if err != nil {
			return nil, err
		}
		selected := []domain.DiscoveredFlutterRepo{}
		for _, ch := range selectedChoices {
			for _, c := range candidates {
				if repoLabel(c) == ch {
					selected = append(selected, c)
					break
				}
			}
		}
		return dedupRepos(selected), nil
	}

	selected := []domain.DiscoveredFlutterRepo{}
	for _, selector := range selectors {
		repo, ok := findRepoBySelector(candidates, selector)
		if !ok {
			return nil, domain.NewError(domain.CategoryInput, 2, "Unknown package selector: "+selector+".", "Use --package with discovered repository name or path.", nil)
		}
		selected = append(selected, repo)
	}
	return dedupRepos(selected), nil
}

func dedupRepos(repos []domain.DiscoveredFlutterRepo) []domain.DiscoveredFlutterRepo {
	seen := map[string]struct{}{}
	out := []domain.DiscoveredFlutterRepo{}
	for _, repo := range repos {
		key := filepath.Clean(repo.RepoRoot)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, repo)
	}
	return out
}

func findRepoBySelector(repos []domain.DiscoveredFlutterRepo, selector string) (domain.DiscoveredFlutterRepo, bool) {
	sel := strings.TrimSpace(selector)
	for _, repo := range repos {
		if repo.Name == sel || repo.PackageName == sel || filepath.Clean(repo.RepoRoot) == filepath.Clean(sel) {
			return repo, true
		}
	}
	return domain.DiscoveredFlutterRepo{}, false
}

func repoLabel(repo domain.DiscoveredFlutterRepo) string {
	return fmt.Sprintf("%s [%s] (%s)", repo.Name, repo.PackageName, repo.RepoRoot)
}

func buildOverrideContent(root domain.PlannedWorktree, packages []domain.PlannedWorktree) string {
	lines := []string{"dependency_overrides:"}
	if len(packages) == 0 {
		lines = append(lines, "  {}")
		return strings.Join(lines, "\n") + "\n"
	}
	for _, pkg := range packages {
		rel, err := filepath.Rel(root.Path, pkg.Path)
		if err != nil {
			rel = pkg.Path
		}
		lines = append(lines, "  "+pkg.Repo.PackageName+":")
		lines = append(lines, "    path: "+filepath.ToSlash(rel))
	}
	return strings.Join(lines, "\n") + "\n"
}

func buildWorkspaceFolders(root domain.PlannedWorktree, packages []domain.PlannedWorktree, container string) []string {
	folders := []string{workspaceFolderPath(container, root.Path)}
	for _, pkg := range packages {
		folders = append(folders, workspaceFolderPath(container, pkg.Path))
	}
	folders = dedupStringsPreservingOrder(folders)
	if len(folders) > 1 {
		sort.Strings(folders[1:])
	}
	return folders
}

func workspaceFolderPath(container, target string) string {
	rel, err := filepath.Rel(container, target)
	if err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(target)
}

func dedupStringsPreservingOrder(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func writeWorkspace(path string, folders []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return domain.NewError(domain.CategoryPersistence, 5, "Failed to create workspace directory.", path, err)
	}
	type folder struct {
		Path string `json:"path"`
	}
	payload := struct {
		Folders []folder `json:"folders"`
	}{Folders: []folder{}}
	for _, f := range folders {
		payload.Folders = append(payload.Folders, folder{Path: f})
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return domain.NewError(domain.CategoryPersistence, 5, "Failed to serialize VSCode workspace file.", path, err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		return domain.NewError(domain.CategoryPersistence, 5, "Failed to write VSCode workspace file.", path, err)
	}
	return nil
}

func ensureGitignoreContains(repoPath, entry string) error {
	gitignorePath := filepath.Join(repoPath, ".gitignore")

	content := ""
	if b, err := os.ReadFile(gitignorePath); err == nil {
		content = string(b)
		for _, line := range strings.Split(content, "\n") {
			if strings.TrimSpace(line) == entry {
				return nil
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += entry + "\n"

	return os.WriteFile(gitignorePath, []byte(content), 0o644)
}
