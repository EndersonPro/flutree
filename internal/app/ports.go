package app

import "github.com/EndersonPro/flutree/internal/domain"

type GitPort interface {
	EnsureRepo() (string, error)
	ListWorktrees(repoRoot string) ([]domain.GitWorktreeEntry, error)
	CreateWorktree(repoRoot, path, branch, baseBranch string) error
	CreateWorktreeNew(repoRoot, path, branch, startPoint string) error
	CreateWorktreeExisting(repoRoot, path, branch string) error
	BranchExists(repoRoot, branch string) (bool, error)
	RemoteBranchExists(repoRoot, branch string) (bool, error)
	SyncBranchWithRemote(repoRoot, branch string) error
	SyncBaseBranch(repoRoot, baseBranch string) (string, error)
	RemoveWorktree(repoRoot, path string, force bool) error
	// PruneWorktrees drops git metadata of worktrees whose directory is gone.
	PruneWorktrees(repoRoot string) error
	IsDirty(path string) (bool, error)
	// EnsureLocalExclude adds pattern to the repository's local, never
	// committed ignore file (.git/info/exclude) when it is not there yet.
	EnsureLocalExclude(worktreePath, pattern string) error
	DiscoverFlutterRepos(scope string) ([]domain.DiscoveredFlutterRepo, error)
	// InspectFlutterRepo reports whether path itself is a Flutter repository,
	// using the same rules and fields as DiscoverFlutterRepos.
	InspectFlutterRepo(path string) (domain.DiscoveredFlutterRepo, bool, error)
	// BaseBranchExists reports whether base resolves to a commit locally
	// (branch, tag, SHA) or as origin/<base>, without network access.
	BaseBranchExists(repoRoot, branch string) (bool, error)
	// DeleteBranch force-deletes a local branch (never a checked-out one).
	DeleteBranch(repoRoot, branch string) error
	// CurrentBranch returns the checked-out branch ("" when HEAD is detached).
	CurrentBranch(repoRoot string) (string, error)
	// ListBranches returns local branches and origin/* remote-tracking branches
	// without touching the network.
	ListBranches(repoRoot string) (local, remote []string, err error)
	// AheadBehind counts the commits of the worktree HEAD missing from base
	// (ahead) and the commits of base missing from HEAD (behind). base is
	// resolved like a worktree start point: a local commit-ish, else
	// origin/<base>. It returns an error when base cannot be resolved.
	AheadBehind(worktreePath, base string) (ahead, behind int, err error)
}

// SDKPort reads Flutter SDK pins and runs the SDK manager in a worktree.
type SDKPort interface {
	// Pins reads the pins declared in the worktree, falling back to the source
	// repository for files that are not committed (puro gitignores its own).
	Pins(worktreePath, sourceRepoRoot string) (domain.SDKPins, error)
	Use(worktreePath string, manager domain.SDKManager, version string) error
}

type RegistryPort interface {
	ListRecords() ([]domain.RegistryRecord, error)
	Get(name string) (domain.RegistryRecord, error)
	Upsert(record domain.RegistryRecord) error
	Remove(name string) (domain.RegistryRecord, error)
	MarkCompleted(name string) (domain.RegistryRecord, error)
}

type ConfigPort interface {
	Load() (domain.UserConfigDocument, error)
	Save(domain.UserConfigDocument) error
}

type PromptPort interface {
	Confirm(message string, nonInteractive, assumeYes bool) (bool, error)
	ConfirmWithToken(message, token string, nonInteractive, assumeYes bool) (bool, error)
	SelectOne(message string, choices []string, nonInteractive bool) (string, error)
	SelectPackages(message string, choices []string, nonInteractive bool) ([]string, error)
	AskText(message, defaultValue string, nonInteractive bool) (string, error)
}

type PubPort interface {
	DetectTool(repoPath string) (domain.PubTool, error)
	Clean(repoPath string, tool domain.PubTool) error
	RemoveLock(repoPath string) error
	Get(repoPath string, tool domain.PubTool) error
}

type UpdatePort interface {
	CheckBrewInstalled() error
	CheckOutdated(packageName string) (bool, string, string, error)
	Upgrade(packageName string) (string, error)
}
