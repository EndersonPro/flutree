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
	sdk      SDKPort
	sdkPref  func() (domain.SDKManager, error)
}

// WithSDK enables SDK manager setup (fvm/puro) for created worktrees.
// preference is read on every run so a changed sdk.manager applies at once.
func (s *CreateService) WithSDK(sdk SDKPort, preference func() (domain.SDKManager, error)) *CreateService {
	s.sdk = sdk
	s.sdkPref = preference
	return s
}

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

	// An explicit candidate set (chosen in the wizard) replaces discovery.
	repos := dedupRepos(input.Repos)
	var discoverErr error
	if len(repos) == 0 {
		repos, discoverErr = s.git.DiscoverFlutterRepos(input.ExecutionScope)
		if discoverErr != nil {
			repos = nil
		}
	}
	// A selector that is a path to a Flutter repo outside the scope joins the
	// candidates; selectors are rewritten to the repo's absolute path.
	selectors := append([]string{input.RootSelector}, input.PackageSelectors...)
	repos, selectors, extra := withPathRepos(s.git, repos, selectors)
	if discoverErr != nil && extra == 0 {
		return domain.CreateDryPlan{}, discoverErr
	}
	rootSelector, packageSelectors := selectors[0], selectors[1:]

	rootRepo, err := s.resolveRootRepo(repos, rootSelector, input.NonInteractive)
	if err != nil {
		return domain.CreateDryPlan{}, err
	}

	packages := []domain.DiscoveredFlutterRepo{}
	if !input.NoPackage {
		packages, err = s.resolvePackageRepos(repos, rootRepo, packageSelectors, input.NonInteractive)
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

	// Fail before any side effect when a package base branch is missing.
	for _, plan := range packagePlans {
		ok, err := s.git.BaseBranchExists(plan.Repo.RepoRoot, plan.BaseBranch)
		if err != nil {
			return domain.CreateDryPlan{}, err
		}
		if !ok {
			return domain.CreateDryPlan{}, domain.NewError(
				domain.CategoryInput, 2,
				"Base branch '"+plan.BaseBranch+"' does not exist in "+plan.Repo.Name+".",
				"Pass --package-base "+plan.Repo.Name+"=<branch>.",
				nil,
			)
		}
	}

	// Without packages there is nothing to override: the root pubspec's own
	// dependency_overrides already apply, so no pubspec_overrides.yaml is written.
	overridePath, overrideContent := "", ""
	if len(packagePlans) > 0 {
		overridePath = filepath.Join(rootPlan.Path, overrideFileName)
		overrideContent = buildOverrideContent(rootPlan, packagePlans)
	}

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
	// Branches this run created; a branch that already existed is never listed.
	type branchRef struct{ repoRoot, branch string }
	createdBranches := []branchRef{}
	// Only a container this run creates is removed on failure; one that
	// already existed holds someone else's work.
	_, statErr := os.Stat(plan.ContainerPath)
	containerPreExisted := statErr == nil
	rollback := func() {
		defer func() {
			if !containerPreExisted && plan.ContainerPath != "" {
				_ = os.RemoveAll(plan.ContainerPath)
			}
		}()
		for i := len(persisted) - 1; i >= 0; i-- {
			_, _ = s.registry.Remove(persisted[i])
		}
		for i := len(created) - 1; i >= 0; i-- {
			_ = s.git.RemoveWorktree(created[i].Repo.RepoRoot, created[i].Path, true)
		}
		// Branches go after their worktrees: git refuses a checked-out branch.
		for i := len(createdBranches) - 1; i >= 0; i-- {
			_ = s.git.DeleteBranch(createdBranches[i].repoRoot, createdBranches[i].branch)
		}
	}

	if err := os.MkdirAll(filepath.Dir(plan.Root.Path), 0o755); err != nil {
		rollback()
		return domain.CreateResult{}, domain.NewError(domain.CategoryPersistence, 5, "Failed to create root worktree directory.", plan.Root.Path, err)
	}
	madeBranch, err := s.createPlannedWorktree(plan.Root, options)
	if madeBranch {
		createdBranches = append(createdBranches, branchRef{plan.Root.Repo.RepoRoot, plan.Root.Branch})
	}
	if err != nil {
		rollback()
		return domain.CreateResult{}, err
	}
	created = append(created, plan.Root)
	if err := copyRootFiles(plan.Root.Repo.RepoRoot, plan.Root.Path, plan.RootFiles); err != nil {
		rollback()
		return domain.CreateResult{}, domain.NewError(domain.CategoryPersistence, 5, "Failed to copy root files into root worktree.", plan.Root.Path, err)
	}

	for _, pkg := range plan.Packages {
		if err := os.MkdirAll(filepath.Dir(pkg.Path), 0o755); err != nil {
			rollback()
			return domain.CreateResult{}, domain.NewError(domain.CategoryPersistence, 5, "Failed to create package worktree directory.", pkg.Path, err)
		}
		madeBranch, err := s.createPlannedWorktree(pkg, options)
		if madeBranch {
			createdBranches = append(createdBranches, branchRef{pkg.Repo.RepoRoot, pkg.Branch})
		}
		if err != nil {
			rollback()
			return domain.CreateResult{}, err
		}
		created = append(created, pkg)
		if err := copyRootFiles(pkg.Repo.RepoRoot, pkg.Path, plan.RootFiles); err != nil {
			rollback()
			return domain.CreateResult{}, domain.NewError(domain.CategoryPersistence, 5, "Failed to copy root files into package worktree.", pkg.Path, err)
		}
	}

	if plan.OverridePath != "" {
		if err := writeOverrideFile(plan.OverridePath, plan.Root.Path, plan.Root.Repo.RepoRoot, plan.Packages); err != nil {
			rollback()
			return domain.CreateResult{}, domain.NewError(domain.CategoryPersistence, 5, "Failed to write pubspec_overrides.yaml.", err.Error(), err)
		}
		if err := s.git.EnsureLocalExclude(plan.Root.Path, overrideExcludePattern); err != nil {
			rollback()
			return domain.CreateResult{}, domain.NewError(domain.CategoryPersistence, 5, "Failed to ignore pubspec_overrides.yaml in .git/info/exclude.", err.Error(), err)
		}
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
	targets := []sdkTarget{{repo: plan.Root.Repo.Name, path: plan.Root.Path, sourceRoot: plan.Root.Repo.RepoRoot}}
	for _, pkg := range plan.Packages {
		selected = append(selected, pkg.Repo.PackageName)
		targets = append(targets, sdkTarget{repo: pkg.Repo.Name, path: pkg.Path, sourceRoot: pkg.Repo.RepoRoot})
	}
	return domain.CreateResult{
		Record:           rootRecord,
		NextStep:         "cd " + plan.Root.Path,
		SelectedPackages: selected,
		WorkspacePath:    plan.WorkspacePath,
		SDKSetup:         setupSDKs(s.sdk, s.sdkPref, targets),
	}, nil
}

// createPlannedWorktree creates the worktree for target. The boolean reports
// that this call created the local branch (so a rollback may delete it); it is
// true even when a later step of the call failed.
func (s *CreateService) createPlannedWorktree(target domain.PlannedWorktree, options domain.CreateApplyOptions) (bool, error) {
	exists, err := s.git.BranchExists(target.Repo.RepoRoot, target.Branch)
	if err != nil {
		return false, err
	}

	if exists {
		if options.NonInteractive {
			if !options.ReuseExistingBranch {
				return false, domain.NewError(
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
				return false, confirmErr
			}
			if !confirmed {
				return false, domain.NewError(
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
				return false, err
			}
		}

		return false, s.git.CreateWorktreeExisting(target.Repo.RepoRoot, target.Path, target.Branch)
	}

	// Branch doesn't exist locally — check if it exists on origin
	if options.SyncWithRemote {
		remoteExists, err := s.git.RemoteBranchExists(target.Repo.RepoRoot, target.Branch)
		if err != nil {
			return false, err
		}
		if remoteExists {
			// Fetch and set up local branch from remote
			if err := s.git.SyncBranchWithRemote(target.Repo.RepoRoot, target.Branch); err != nil {
				return false, err
			}
			// Create worktree from the now-existing local branch; the local
			// branch is ours from here on.
			return true, s.git.CreateWorktreeExisting(target.Repo.RepoRoot, target.Path, target.Branch)
		}
	}

	startPoint := target.BaseBranch
	if options.SyncWithRemote {
		startPoint, err = s.git.SyncBaseBranch(target.Repo.RepoRoot, target.BaseBranch)
		if err != nil {
			return false, err
		}
	}
	if err := s.git.CreateWorktreeNew(target.Repo.RepoRoot, target.Path, target.Branch, startPoint); err != nil {
		return false, err
	}
	return true, nil
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

// looksLikePath tells a path selector (./x, ../x, /x, ~/x, a/b) from a repo name.
func looksLikePath(selector string) bool {
	sel := strings.TrimSpace(selector)
	return strings.HasPrefix(sel, "~") || strings.HasPrefix(sel, ".") || strings.ContainsRune(sel, filepath.Separator)
}

// resolvePathSelector expands "~" and makes the path absolute (relative paths
// resolve from the current directory).
func resolvePathSelector(selector string) string {
	sel := strings.TrimSpace(selector)
	if sel == "~" || strings.HasPrefix(sel, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			sel = filepath.Join(home, strings.TrimPrefix(sel, "~"))
		}
	}
	return domain.NormalizePath(sel)
}

// withPathRepos adds to repos every selector that is a path to a Flutter repo
// not already known (for example one outside the discovery scope). Matching
// selectors are rewritten to the repo's absolute path so the usual selector
// matching applies; it returns how many repos were added.
func withPathRepos(git GitPort, repos []domain.DiscoveredFlutterRepo, selectors []string) ([]domain.DiscoveredFlutterRepo, []string, int) {
	out := append([]string(nil), selectors...)
	added := 0
	for i, selector := range out {
		if strings.TrimSpace(selector) == "" || !looksLikePath(selector) {
			continue
		}
		if _, ok := findRepoBySelector(repos, selector); ok {
			continue
		}
		repo, ok, err := git.InspectFlutterRepo(resolvePathSelector(selector))
		if err != nil || !ok {
			continue
		}
		out[i] = repo.RepoRoot
		if _, known := findRepoBySelector(repos, repo.RepoRoot); !known {
			repos = append(repos, repo)
			added++
		}
	}
	return repos, out, added
}
