package app

import "github.com/EndersonPro/flutree/internal/domain"

type GitPort interface {
	EnsureRepo() (string, error)
	ListWorktrees(repoRoot string) ([]domain.GitWorktreeEntry, error)
	CreateWorktree(repoRoot, path, branch, baseBranch string) error
	CreateWorktreeNew(repoRoot, path, branch, startPoint string) error
	CreateWorktreeExisting(repoRoot, path, branch string) error
	BranchExists(repoRoot, branch string) (bool, error)
	SyncBranchWithRemote(repoRoot, branch string) error
	SyncBaseBranch(repoRoot, baseBranch string) (string, error)
	RemoveWorktree(repoRoot, path string, force bool) error
	IsDirty(path string) (bool, error)
	DiscoverFlutterRepos(scope string) ([]domain.DiscoveredFlutterRepo, error)
}

type RegistryPort interface {
	ListRecords() ([]domain.RegistryRecord, error)
	Get(name string) (domain.RegistryRecord, error)
	Upsert(record domain.RegistryRecord) error
	Remove(name string) (domain.RegistryRecord, error)
	MarkCompleted(name string) (domain.RegistryRecord, error)
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
