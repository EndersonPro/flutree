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
	Category ErrorCategory
	Message  string
	Hint     string
	Code     int
	Cause    error
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
	Name         string
	Branch       string
	Path         string
	RepoRoot     string
	Status       string
	PackageCount int
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
	Record        RegistryRecord
	RemovedBranch bool
	NextStep      string
	StaleCleaned  bool
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
	Name string
	Path string
	Tool PubTool
	Role string
}

type PubGetResult struct {
	WorkspaceName string
	Root          PubGetRepoResult
	Packages      []PubGetRepoResult
	Force         bool
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
	Record           RegistryRecord
	NextStep         string
	SelectedPackages []string
	WorkspacePath    string
}

type AddRepoInput struct {
	WorkspaceName     string
	ExecutionScope    string
	RepoSelectors     []string
	PackageBaseBranch map[string]string
	RootFiles         []string
	NonInteractive    bool
}

type AddRepoResult struct {
	WorkspaceName  string
	AddedRepos     []string
	OverridePath   string
	SelectedBranch string
}

type UpdateInput struct {
	Check bool
	Apply bool
}

type UpdateResult struct {
	Mode         string
	Outdated     bool
	Current      string
	Latest       string
	UpgradeNotes string
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
