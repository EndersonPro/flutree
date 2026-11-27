package app

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/EndersonPro/flutree/internal/domain"
)

// InspectService answers the read-only questions an agent asks before and
// after creating a workspace: which repositories exist, what a repository
// looks like, which branches it offers, and how a workspace is doing. It never
// writes anything.
type InspectService struct {
	git      GitPort
	sdk      SDKPort
	registry RegistryPort
}

func NewInspectService(git GitPort, sdk SDKPort, registry RegistryPort) *InspectService {
	return &InspectService{git: git, sdk: sdk, registry: registry}
}

// DiscoverRepos lists the Flutter repositories under scope with their branch.
func (s *InspectService) DiscoverRepos(scope string) ([]domain.RepoSummary, error) {
	if strings.TrimSpace(scope) == "" {
		return nil, domain.NewError(domain.CategoryInput, 2,
			"Scope is required.", "Pass the folder that contains the Flutter repositories.", nil)
	}
	repos, err := s.git.DiscoverFlutterRepos(scope)
	if err != nil {
		return nil, err
	}
	out := make([]domain.RepoSummary, 0, len(repos))
	for _, repo := range repos {
		branch, _ := s.git.CurrentBranch(repo.RepoRoot)
		out = append(out, domain.RepoSummary{
			Name:          repo.Name,
			PackageName:   repo.PackageName,
			RepoRoot:      repo.RepoRoot,
			CurrentBranch: branch,
		})
	}
	return out, nil
}

// InspectRepo describes the repository at path. A directory that is not a
// Flutter repository is a valid answer (is_flutter=false); a path that does
// not exist is an input error.
func (s *InspectService) InspectRepo(path string) (domain.RepoInspection, error) {
	if strings.TrimSpace(path) == "" {
		return domain.RepoInspection{}, domain.NewError(domain.CategoryInput, 2,
			"Path is required.", "Pass the path of the repository to inspect.", nil)
	}
	root := domain.NormalizePath(path)
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return domain.RepoInspection{}, domain.NewError(domain.CategoryInput, 2,
			"Path does not exist or is not a directory.", root, err)
	}
	repo, ok, err := s.git.InspectFlutterRepo(root)
	if err != nil {
		return domain.RepoInspection{}, err
	}
	result := domain.RepoInspection{Path: root, PathDependencies: map[string]string{}}
	if !ok {
		result.Reason = "Not a Flutter repository: it needs a .git entry and a pubspec.yaml with a name at the folder root."
		return result, nil
	}
	result.IsFlutter = true
	result.Name = repo.Name
	result.PackageName = repo.PackageName
	result.RepoRoot = repo.RepoRoot

	if result.CurrentBranch, err = s.git.CurrentBranch(repo.RepoRoot); err != nil {
		return domain.RepoInspection{}, err
	}
	if result.LocalBranches, _, err = s.git.ListBranches(repo.RepoRoot); err != nil {
		return domain.RepoInspection{}, err
	}
	if s.sdk != nil {
		if pins, err := s.sdk.Pins(repo.RepoRoot, repo.RepoRoot); err == nil {
			result.SDK = domain.RepoSDKPins{FVMVersion: pins.FVMVersion, PuroEnv: pins.PuroEnv}
		}
	}
	pubspec := filepath.Join(repo.RepoRoot, "pubspec.yaml")
	for _, section := range []string{"dependencies", "dev_dependencies"} {
		set, err := readPubspecSection(pubspec, section)
		if err != nil {
			return domain.RepoInspection{}, domain.NewError(domain.CategoryInput, 2,
				"pubspec.yaml could not be parsed.", err.Error(), err)
		}
		for name, spec := range set {
			if dir, isPath := pathSpec(spec); isPath {
				result.PathDependencies[name] = dir
			}
		}
	}
	return result, nil
}

// ListBranches returns the local and origin/* branches of a repository.
func (s *InspectService) ListBranches(repoPath string) (domain.BranchList, error) {
	if strings.TrimSpace(repoPath) == "" {
		return domain.BranchList{}, domain.NewError(domain.CategoryInput, 2,
			"repo_path is required.", "Pass the path of the repository.", nil)
	}
	root := domain.NormalizePath(repoPath)
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return domain.BranchList{}, domain.NewError(domain.CategoryInput, 2,
			"Path does not exist or is not a directory.", root, err)
	}
	local, remote, err := s.git.ListBranches(root)
	if err != nil {
		return domain.BranchList{}, err
	}
	current, err := s.git.CurrentBranch(root)
	if err != nil {
		return domain.BranchList{}, err
	}
	return domain.BranchList{RepoPath: root, Current: current, Local: local, Remote: remote}, nil
}

// WorkspaceStatus reports the worktrees of a managed workspace with their git
// dirtiness and the generated pubspec_overrides.yaml.
//
// Ahead/behind versus the base branch is deliberately not reported: the
// registry does not record the base each worktree was created from, so any
// number would be a guess.
func (s *InspectService) WorkspaceStatus(name string) (domain.WorkspaceStatus, error) {
	records, err := s.registry.ListRecords()
	if err != nil {
		return domain.WorkspaceStatus{}, err
	}
	lookup := strings.TrimSpace(name)
	var root *domain.RegistryRecord
	for i := range records {
		if records[i].Name == lookup || records[i].Name == normalizeWorktreeName(lookup) {
			if _, isPkg := splitPackageRecordName(records[i].Name); !isPkg {
				root = &records[i]
				break
			}
		}
	}
	if root == nil {
		return domain.WorkspaceStatus{}, domain.NewError(domain.CategoryPrecondition, 3,
			"Workspace '"+lookup+"' is not registered.",
			"Call list_worktrees to see the registered workspaces.", nil)
	}

	status := domain.WorkspaceStatus{
		Name:          root.Name,
		Branch:        root.Branch,
		Status:        root.Status,
		ContainerPath: filepath.Dir(filepath.Dir(root.Path)),
		Root:          s.worktreeStatus(*root, "root"),
		Packages:      []domain.WorktreeStatus{},
	}
	prefix := root.Name + "__pkg__"
	for _, rec := range records {
		if strings.HasPrefix(rec.Name, prefix) {
			status.Packages = append(status.Packages, s.worktreeStatus(rec, "package"))
		}
	}
	sort.Slice(status.Packages, func(i, j int) bool { return status.Packages[i].Name < status.Packages[j].Name })

	workspaceFile := filepath.Join(status.ContainerPath, root.Name+".code-workspace")
	if _, err := os.Stat(workspaceFile); err == nil {
		status.WorkspaceFile = workspaceFile
	}

	overridePath := filepath.Join(root.Path, overrideFileName)
	status.Overrides = domain.OverridesInfo{Path: overridePath, Entries: map[string]any{}}
	if _, err := os.Stat(overridePath); err == nil {
		status.Overrides.Exists = true
		if set, err := readOverridesSection(overridePath); err == nil {
			for pkg, node := range set {
				var spec any
				if node.Decode(&spec) == nil {
					status.Overrides.Entries[pkg] = spec
				}
			}
		}
	}
	return status, nil
}

func (s *InspectService) worktreeStatus(rec domain.RegistryRecord, role string) domain.WorktreeStatus {
	name := rec.Name
	if pkg, isPkg := splitPackageRecordName(rec.Name); isPkg {
		name = strings.TrimPrefix(rec.Name, pkg+"__pkg__")
	}
	out := domain.WorktreeStatus{Name: name, Role: role, Path: rec.Path, RepoRoot: rec.RepoRoot, Branch: rec.Branch}
	if info, err := os.Stat(rec.Path); err == nil && info.IsDir() {
		out.Exists = true
		out.Dirty, _ = s.git.IsDirty(rec.Path)
	}
	return out
}
