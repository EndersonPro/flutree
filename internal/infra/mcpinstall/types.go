// Package mcpinstall provides non-destructive installer logic that registers
// the flutree MCP server entry in each detected AI coding client's config file.
package mcpinstall

// Outcome describes the per-client result of a Merge operation.
type Outcome string

const (
	// OutcomeConfigured means the entry was written successfully.
	OutcomeConfigured Outcome = "configured"
	// OutcomeAlreadyExists means the entry was already present; file unchanged.
	OutcomeAlreadyExists Outcome = "already_exists"
	// OutcomeNotInstalled means the client was not detected on this system.
	OutcomeNotInstalled Outcome = "not_installed"
	// OutcomeError means an error prevented the merge; no write occurred.
	OutcomeError Outcome = "error"
	// OutcomeUnsupportedScope means the client has no config for the requested scope.
	OutcomeUnsupportedScope Outcome = "unsupported_scope"
	// OutcomeRemoved means the flutree entry was removed (uninstall).
	OutcomeRemoved Outcome = "removed"
	// OutcomeNotFound means the client is installed but holds no flutree entry (uninstall).
	OutcomeNotFound Outcome = "not_found"
)

// Scope selects which config file of a client is targeted.
type Scope string

const (
	// ScopeUser targets the per-user config (the default).
	ScopeUser Scope = "user"
	// ScopeProject targets the config of the current project directory.
	ScopeProject Scope = "project"
)

// Options controls Install, Uninstall and Diagnose.
type Options struct {
	// Clients restricts the run to these client names; empty means all.
	Clients []string
	// Force overwrites an existing entry on install.
	Force bool
	// DryRun reports what would happen without writing anything.
	DryRun bool
	// Scope defaults to ScopeUser when empty.
	Scope Scope
	// Cwd is the project directory used by ScopeProject.
	Cwd string
}

// Client extends Merger with scope, preview, removal and lookup support. Every
// built-in client implements it; Installer falls back to the plain Merger
// behavior for implementations that do not.
type Client interface {
	Merger
	// ConfigPath returns the config file for the current scope.
	ConfigPath() string
	// WithScope returns a copy targeting the scope, or false when the client
	// has no config for it.
	WithScope(scope Scope, cwd string) (Client, bool)
	// Preview renders the exact entry Merge would write.
	Preview(absCmd string) (string, error)
	// Remove deletes only the flutree entry: OutcomeRemoved or OutcomeNotFound.
	Remove() (Outcome, error)
	// EntryCommand returns the command of the existing flutree entry.
	EntryCommand() (command string, found bool, err error)
}

// Merger is the per-client seam. Each client (claude-code, opencode, codex)
// implements this interface so the orchestrator and tests stay decoupled.
type Merger interface {
	// Name returns the client identifier string, e.g. "claude-code".
	Name() string
	// Detect returns true when the client appears to be installed on this host.
	// A non-nil error indicates an I/O problem (e.g. permission denied on home
	// directory); the caller should treat it as an installation failure rather
	// than "not installed".
	Detect() (bool, error)
	// Merge writes the flutree server entry to the client's config file.
	// absCmd is the absolute path to the flutree binary (never a bare name).
	// force controls whether an existing entry is overwritten.
	Merge(absCmd string, force bool) (Outcome, error)
}

// InstallResult holds the per-client outcome returned by Installer.Run.
type InstallResult struct {
	// Client is the Merger.Name() value for this entry.
	Client string `json:"client"`
	// Status is the outcome of the merge attempt.
	Status Outcome `json:"status"`
	// Message provides human-readable detail; non-empty when Status is OutcomeError.
	Message string `json:"message,omitempty"`
	// ConfigPath is the absolute path of the file that was (or would be) written.
	// Empty when Status is OutcomeNotInstalled.
	ConfigPath string `json:"config_path,omitempty"`
	// DryRun is true when nothing was written; Status is what would happen.
	DryRun bool `json:"dry_run,omitempty"`
	// Entry is the exact entry that would be written (dry-run install only).
	Entry string `json:"entry,omitempty"`
}

// MergerWithConfigPath optionally extends Merger to expose the config path so
// the installer result can include it. Implementations that don't need it can
// omit ConfigPath from their result (the interface remains satisfied by Merger alone).
type MergerWithConfigPath interface {
	Merger
	ConfigPath() string
}

// ExecResolverFunc is a function that returns the absolute path of the flutree
// binary. It is injectable so tests can provide a fixed path without touching
// os.Executable or filepath.EvalSymlinks.
type ExecResolverFunc func() (string, error)

// ExecResolver returns an ExecResolverFunc that always returns the given path.
// Use this in tests instead of the real binary resolver.
func ExecResolver(absPath string) ExecResolverFunc {
	return func() (string, error) { return absPath, nil }
}

// RealExecResolver returns an ExecResolverFunc that resolves the current
// binary's absolute path, following symlinks (handles Homebrew shims).
func RealExecResolver() ExecResolverFunc {
	return func() (string, error) {
		return resolveExecutable()
	}
}
