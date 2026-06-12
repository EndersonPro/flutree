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
)

// Merger is the per-client seam. Each client (claude-code, opencode, codex)
// implements this interface so the orchestrator and tests stay decoupled.
type Merger interface {
	// Name returns the client identifier string, e.g. "claude-code".
	Name() string
	// Detect returns true when the client appears to be installed on this host.
	Detect() bool
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
