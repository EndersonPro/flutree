package domain

import "path/filepath"

type ErrorCategory string

const (
	CategoryUnexpected   ErrorCategory = "unexpected"
	CategoryInput        ErrorCategory = "input"
	CategoryPrecondition ErrorCategory = "precondition"
	CategoryGit          ErrorCategory = "git"
	CategoryPersistence  ErrorCategory = "persistence"
)

type AppError struct {
	Category ErrorCategory `json:"category"`
	Message  string        `json:"message"`
	Hint     string        `json:"hint,omitempty"`
	Code     int           `json:"code"`
	Cause    error         `json:"-"`
}

func (e *AppError) Error() string { return e.Message }
func (e *AppError) Unwrap() error { return e.Cause }

func NewError(category ErrorCategory, code int, message, hint string, cause error) *AppError {
	return &AppError{Category: category, Code: code, Message: message, Hint: hint, Cause: cause}
}

type DiscoveredFlutterRepo struct {
	Name        string
	RepoRoot    string
	PackageName string
}

type RegistryRecord struct {
	Name      string `json:"name"`
	Branch    string `json:"branch"`
	Path      string `json:"path"`
	RepoRoot  string `json:"repo_root"`
	CreatedAt string `json:"created_at,omitempty"`
	Status    string `json:"status"`
}

type RegistryDocument struct {
	Version int              `json:"version"`
	Records []RegistryRecord `json:"records"`
}

type UserScopeConfig struct {
	Root string `json:"root,omitempty"`
}

type UserConfigDocument struct {
	Version int             `json:"version"`
	Scope   UserScopeConfig `json:"scope"`
}

type GitWorktreeEntry struct {
	Path        string
	Head        string
	Branch      string
	IsBare      bool
	IsDetached  bool
	IsLocked    bool
	PruneReason string
}

type ListRow struct {
	Name         string `json:"name"`
	Branch       string `json:"branch"`
	Path         string `json:"path"`
	RepoRoot     string `json:"repo_root"`
	Status       string `json:"status"`
	PackageCount int    `json:"package_count"`
}

type ListInput struct {
	ShowAll     bool
	GlobalScope bool
}

type CompleteInput struct {
	Name           string
	Yes            bool
	Force          bool
	NonInteractive bool
}

type CompleteResult struct {
	Record        RegistryRecord `json:"record"`
	RemovedBranch bool           `json:"removed_branch"`
	NextStep      string         `json:"next_step"`
	StaleCleaned  bool           `json:"stale_cleaned"`
}

type PubTool string

const (
	PubToolFlutter PubTool = "flutter"
	PubToolDart    PubTool = "dart"
)

type PubGetInput struct {
	Name  string
	Force bool
}

type PubGetRepoResult struct {
	Name string  `json:"name"`
	Path string  `json:"path"`
	Tool PubTool `json:"tool"`
	Role string  `json:"role"`
}

type PubGetResult struct {
	WorkspaceName string             `json:"workspace_name"`
	Root          PubGetRepoResult   `json:"root"`
	Packages      []PubGetRepoResult `json:"packages"`
	Force         bool               `json:"force"`
}

type CleanInput struct {
	Force bool
}

type CleanResult struct {
	Record      RegistryRecord `json:"record"`
	Tool        PubTool        `json:"tool"`
	Force       bool           `json:"force"`
	LockRemoved bool           `json:"lock_removed"`
}

type CreateInput struct {
	Name              string
	Branch            string
	BaseBranch        string
	ExecutionScope    string
	RootSelector      string
	NoPackage         bool
	PackageSelectors  []string
	PackageBaseBranch map[string]string
	RootFiles         []string
	GenerateWorkspace bool
	Yes               bool
	NonInteractive    bool
}

type PlannedWorktree struct {
	Repo       DiscoveredFlutterRepo
	Role       string
	Path       string
	Branch     string
	BaseBranch string
}

type CreateDryPlan struct {
	NormalizedName   string
	ContainerPath    string
	Root             PlannedWorktree
	Packages         []PlannedWorktree
	RootFiles        []string
	OverridePath     string
	OverrideContent  string
	WorkspacePath    string
	WorkspaceFolders []string
}

type CreateApplyOptions struct {
	NonInteractive      bool
	ReuseExistingBranch bool
	SyncWithRemote      bool
}

type CreateResult struct {
	Record           RegistryRecord `json:"record"`
	NextStep         string         `json:"next_step"`
	SelectedPackages []string       `json:"selected_packages"`
	WorkspacePath    string         `json:"workspace_path"`
}

type AddRepoSyncPolicy string

const (
	AddRepoSyncAuto   AddRepoSyncPolicy = "auto"
	AddRepoSyncAlways AddRepoSyncPolicy = "always"
	AddRepoSyncNever  AddRepoSyncPolicy = "never"
)

type AddRepoInput struct {
	WorkspaceName       string
	ExecutionScope      string
	RepoSelectors       []string
	PackageBranchSource map[string]string
	PackageBaseBranch   map[string]string
	RootFiles           []string
	SyncPolicy          AddRepoSyncPolicy
	ReuseExistingBranch bool
	NonInteractive      bool
}

type AddRepoResult struct {
	WorkspaceName  string   `json:"workspace_name"`
	AddedRepos     []string `json:"added_repos"`
	OverridePath   string   `json:"override_path"`
	SelectedBranch string   `json:"selected_branch"`
}

type UpdateInput struct {
	Check bool
	Apply bool
}

type UpdateResult struct {
	Mode         string `json:"mode"`
	Outdated     bool   `json:"outdated"`
	Current      string `json:"current"`
	Latest       string `json:"latest"`
	UpgradeNotes string `json:"upgrade_notes,omitempty"`
}

func NormalizePath(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return abs
}
