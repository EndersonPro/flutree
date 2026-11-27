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

// RepoSummary is one Flutter repository found in a folder, as agents see it.
type RepoSummary struct {
	Name          string `json:"name"`
	PackageName   string `json:"package_name"`
	RepoRoot      string `json:"repo_root"`
	CurrentBranch string `json:"current_branch"`
}

// RepoSDKPins reports the Flutter SDK versions a repository pins per manager.
type RepoSDKPins struct {
	FVMVersion string `json:"fvm_version,omitempty"`
	PuroEnv    string `json:"puro_env,omitempty"`
}

// RepoInspection describes one repository for planning a workspace.
type RepoInspection struct {
	Path          string `json:"path"`
	IsFlutter     bool   `json:"is_flutter"`
	Reason        string `json:"reason,omitempty"`
	Name          string `json:"name,omitempty"`
	PackageName   string `json:"package_name,omitempty"`
	RepoRoot      string `json:"repo_root,omitempty"`
	CurrentBranch string `json:"current_branch,omitempty"`
	// LocalBranches are the local branch names.
	LocalBranches []string    `json:"local_branches,omitempty"`
	SDK           RepoSDKPins `json:"sdk_pin"`
	// PathDependencies maps a pubspec dependency (dependencies and
	// dev_dependencies) to the `path:` it declares, exactly as written.
	PathDependencies map[string]string `json:"path_dependencies"`
}

// BranchList is the set of start points available in a repository.
type BranchList struct {
	RepoPath string   `json:"repo_path"`
	Current  string   `json:"current"`
	Local    []string `json:"local"`
	// Remote are remote-tracking branches such as origin/main (no network).
	Remote []string `json:"remote"`
}

// WorktreeStatus is the live state of one worktree of a workspace.
type WorktreeStatus struct {
	Name     string `json:"name"`
	Role     string `json:"role"`
	Path     string `json:"path"`
	RepoRoot string `json:"repo_root"`
	Branch   string `json:"branch"`
	Exists   bool   `json:"exists"`
	Dirty    bool   `json:"dirty"`
	// BaseBranch, Ahead and Behind compare the worktree HEAD with the base it
	// was created from. They are omitted when the registry has no base or the
	// base no longer resolves; AheadBehindNote then says why.
	BaseBranch      string `json:"base_branch,omitempty"`
	Ahead           *int   `json:"ahead,omitempty"`
	Behind          *int   `json:"behind,omitempty"`
	AheadBehindNote string `json:"ahead_behind_note,omitempty"`
}

// OverridesInfo describes the generated pubspec_overrides.yaml of a workspace.
type OverridesInfo struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	// Entries maps an overridden package to its spec (a path string, a version
	// string, or the decoded mapping for git/hosted specs).
	Entries map[string]any `json:"entries"`
}

// WorkspaceStatus is the state of a managed workspace: its worktrees, git
// dirtiness and generated files.
type WorkspaceStatus struct {
	Name          string           `json:"name"`
	Branch        string           `json:"branch"`
	Status        string           `json:"status"`
	ContainerPath string           `json:"container_path"`
	WorkspaceFile string           `json:"workspace_file,omitempty"`
	Root          WorktreeStatus   `json:"root"`
	Packages      []WorktreeStatus `json:"packages"`
	Overrides     OverridesInfo    `json:"pubspec_overrides"`
}

type RegistryRecord struct {
	Name      string `json:"name"`
	Branch    string `json:"branch"`
	Path      string `json:"path"`
	RepoRoot  string `json:"repo_root"`
	CreatedAt string `json:"created_at,omitempty"`
	Status    string `json:"status"`
	// BaseBranch is the base the worktree's branch was created from, as the
	// user gave it. Optional: records written before it existed omit it.
	BaseBranch string `json:"base_branch,omitempty"`
}

type RegistryDocument struct {
	Version int              `json:"version"`
	Records []RegistryRecord `json:"records"`
}

type UserScopeConfig struct {
	Root string `json:"root,omitempty"`
}

type UserConfigDocument struct {
	Version   int                 `json:"version"`
	Scope     UserScopeConfig     `json:"scope"`
	Workspace UserWorkspaceConfig `json:"workspace"`
	SDK       UserSDKConfig       `json:"sdk"`
}

// UserSDKConfig holds the preferred Flutter SDK manager. Empty means auto.
type UserSDKConfig struct {
	Manager SDKManager `json:"manager,omitempty"`
}

// SDKManager selects which Flutter SDK manager prepares new worktrees.
type SDKManager string

const (
	SDKManagerAuto SDKManager = "auto"
	SDKManagerFVM  SDKManager = "fvm"
	SDKManagerPuro SDKManager = "puro"
	SDKManagerNone SDKManager = "none"
)

// SDKPins are the SDK versions a repository declares for each manager.
type SDKPins struct {
	FVMVersion string
	PuroEnv    string
}

// SDKSetupResult reports how the SDK was prepared in one worktree.
type SDKSetupResult struct {
	Repo    string `json:"repo"`
	Path    string `json:"path"`
	Manager string `json:"manager"`
	Version string `json:"version"`
	Status  string `json:"status"` // configured | failed
	Message string `json:"message,omitempty"`
}

// UserWorkspaceConfig holds persisted defaults for the generated VSCode
// workspace. A nil Generate means "not configured" (defaults to true).
type UserWorkspaceConfig struct {
	Generate *bool `json:"generate,omitempty"`
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
	Name           string
	Branch         string
	BaseBranch     string
	ExecutionScope string
	// Repos is an optional explicit candidate set. When non-empty it replaces
	// the discovery of ExecutionScope (repositories may live in unrelated folders).
	Repos             []DiscoveredFlutterRepo
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
	Record           RegistryRecord   `json:"record"`
	NextStep         string           `json:"next_step"`
	SelectedPackages []string         `json:"selected_packages"`
	WorkspacePath    string           `json:"workspace_path"`
	SDKSetup         []SDKSetupResult `json:"sdk_setup,omitempty"`
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
	WorkspaceName  string           `json:"workspace_name"`
	AddedRepos     []string         `json:"added_repos"`
	OverridePath   string           `json:"override_path"`
	SelectedBranch string           `json:"selected_branch"`
	SDKSetup       []SDKSetupResult `json:"sdk_setup,omitempty"`
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
