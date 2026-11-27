package mcpinstall

import (
	"fmt"
	"strings"
)

// Installer orchestrates detection and merging across all AI coding clients.
type Installer struct {
	mergers      []Merger
	execResolver ExecResolverFunc
}

// NewInstaller builds an Installer from a list of Merger implementations and
// an ExecResolverFunc that provides the absolute binary path.
// Pass RealExecResolver() in production code; ExecResolver(path) in tests.
func NewInstaller(mergers []Merger, resolver ExecResolverFunc) *Installer {
	return &Installer{mergers: mergers, execResolver: resolver}
}

// DefaultInstaller returns an Installer pre-loaded with every client merger
// and the real binary resolver. Use this in the CLI dispatch layer.
func DefaultInstaller() *Installer {
	home := "" // empty → real home dir
	return NewInstaller(
		[]Merger{
			NewClaudeMerger(home),
			NewOpenCodeMerger(home),
			NewCodexMerger(home),
			NewCursorMerger(home),
			NewVSCodeMerger(home),
			NewGeminiMerger(home),
			NewClaudeDesktopMerger(home),
			NewWindsurfMerger(home),
		},
		RealExecResolver(),
	)
}

// Run installs with default options (user scope, no dry run). It is kept for
// callers that only need a client filter and the force flag.
func (inst *Installer) Run(clients []string, force bool) ([]InstallResult, error) {
	return inst.Install(Options{Clients: clients, Force: force})
}

// Install iterates the targeted mergers and returns one InstallResult per
// client. An unknown client name fails before any write. With DryRun nothing
// is written: Status is the outcome that would happen and Entry holds the
// exact entry.
func (inst *Installer) Install(opts Options) ([]InstallResult, error) {
	if err := inst.validateFilter(opts.Clients); err != nil {
		return nil, err
	}
	absCmd, err := inst.execResolver()
	if err != nil {
		return nil, fmt.Errorf("resolve binary path: %w", err)
	}
	return inst.each(opts, func(m Merger) InstallResult {
		res := InstallResult{Client: m.Name(), DryRun: opts.DryRun}
		c, isClient := m.(Client)
		if opts.DryRun {
			// A dry run must never reach Merge, which writes the config.
			if !isClient {
				res.Status = OutcomeError
				res.Message = "dry run is not supported for this client; nothing was written"
				return res
			}
			return dryInstall(c, absCmd, opts.Force, res)
		}
		outcome, mergeErr := m.Merge(absCmd, opts.Force)
		res.Status = outcome
		if mergeErr != nil {
			res.Message = mergeErr.Error()
		}
		if cp, ok := m.(MergerWithConfigPath); ok {
			res.ConfigPath = cp.ConfigPath()
		}
		return res
	})
}

func dryInstall(c Client, absCmd string, force bool, res InstallResult) InstallResult {
	res.ConfigPath = c.ConfigPath()
	entry, err := c.Preview(absCmd)
	if err != nil {
		res.Status, res.Message = OutcomeError, err.Error()
		return res
	}
	res.Entry = entry
	res.Status = OutcomeConfigured
	if !force {
		if _, found, err := c.EntryCommand(); err == nil && found {
			res.Status = OutcomeAlreadyExists
		}
	}
	return res
}

// Uninstall removes only the flutree entry from each targeted, detected client.
// With DryRun nothing is written: Status is removed or not_found.
func (inst *Installer) Uninstall(opts Options) ([]InstallResult, error) {
	if err := inst.validateFilter(opts.Clients); err != nil {
		return nil, err
	}
	return inst.each(opts, func(m Merger) InstallResult {
		res := InstallResult{Client: m.Name(), DryRun: opts.DryRun}
		c, ok := m.(Client)
		if !ok {
			res.Status, res.Message = OutcomeError, "client does not support uninstall"
			return res
		}
		res.ConfigPath = c.ConfigPath()
		if opts.DryRun {
			if _, found, err := c.EntryCommand(); err != nil {
				res.Status, res.Message = OutcomeError, err.Error()
			} else if found {
				res.Status = OutcomeRemoved
			} else {
				res.Status = OutcomeNotFound
			}
			return res
		}
		out, err := c.Remove()
		res.Status = out
		if err != nil {
			res.Message = err.Error()
		}
		return res
	})
}

// each applies scope handling and detection, then calls fn for targeted
// clients that are detected and support the scope.
func (inst *Installer) each(opts Options, fn func(Merger) InstallResult) ([]InstallResult, error) {
	var results []InstallResult
	for _, m := range inst.mergers {
		if !inst.isTargeted(m.Name(), opts.Clients) {
			continue
		}
		if c, ok := m.(Client); ok {
			scoped, supported := c.WithScope(opts.Scope, opts.Cwd)
			if !supported {
				results = append(results, unsupportedScope(m.Name(), opts.Scope))
				continue
			}
			m = scoped
		} else if opts.Scope == ScopeProject {
			results = append(results, unsupportedScope(m.Name(), opts.Scope))
			continue
		}

		detected, detectErr := m.Detect()
		if detectErr != nil {
			results = append(results, InstallResult{Client: m.Name(), Status: OutcomeError, Message: detectErr.Error()})
			continue
		}
		if !detected {
			results = append(results, InstallResult{
				Client:  m.Name(),
				Status:  OutcomeNotInstalled,
				Message: fmt.Sprintf("%s not detected on this system", m.Name()),
			})
			continue
		}
		results = append(results, fn(m))
	}
	return results, nil
}

func unsupportedScope(name string, scope Scope) InstallResult {
	return InstallResult{
		Client:  name,
		Status:  OutcomeUnsupportedScope,
		Message: fmt.Sprintf("%s has no %s-scope config", name, scope),
	}
}

// ClientCheck is the doctor report for one client.
type ClientCheck struct {
	Client       string `json:"client"`
	Detected     bool   `json:"detected"`
	ConfigPath   string `json:"config_path,omitempty"`
	EntryPresent bool   `json:"entry_present"`
	Command      string `json:"command,omitempty"`
	CommandOK    bool   `json:"command_ok"`
	Message      string `json:"message,omitempty"`
}

// Diagnose inspects each targeted client without writing anything.
func (inst *Installer) Diagnose(opts Options) ([]ClientCheck, error) {
	if err := inst.validateFilter(opts.Clients); err != nil {
		return nil, err
	}
	var checks []ClientCheck
	for _, m := range inst.mergers {
		if !inst.isTargeted(m.Name(), opts.Clients) {
			continue
		}
		chk := ClientCheck{Client: m.Name()}
		c, ok := m.(Client)
		if ok {
			c, ok = c.WithScope(opts.Scope, opts.Cwd)
		}
		if !ok {
			chk.Message = fmt.Sprintf("no %s-scope config", opts.Scope)
			checks = append(checks, chk)
			continue
		}
		detected, err := c.Detect()
		if err != nil {
			chk.Message = err.Error()
			checks = append(checks, chk)
			continue
		}
		chk.Detected = detected
		chk.ConfigPath = c.ConfigPath()
		cmd, found, err := c.EntryCommand()
		switch {
		case err != nil:
			chk.Message = err.Error()
		case found:
			chk.EntryPresent, chk.Command = true, cmd
			chk.CommandOK = commandExecutable(cmd)
		}
		checks = append(checks, chk)
	}
	return checks, nil
}

// ValidateClients checks that every name in clients is a known merger name.
// It returns a descriptive error when an unknown name is found; nil otherwise.
// Callers may use this before Run to assign the correct error category: an
// unknown client is a user-input error, while Run's other failure modes are
// runtime/unexpected errors.
func (inst *Installer) ValidateClients(clients []string) error {
	return inst.validateFilter(clients)
}

// validateFilter checks that every name in clients is a known merger name.
func (inst *Installer) validateFilter(clients []string) error {
	if len(clients) == 0 {
		return nil
	}
	known := inst.knownNames()
	for _, name := range clients {
		if !contains(known, name) {
			return fmt.Errorf(
				"unknown client %q; valid values: %s",
				name,
				strings.Join(known, ", "),
			)
		}
	}
	return nil
}

// isTargeted returns true when name should be processed: either the filter
// list is empty (no restriction) or the name appears in the list.
func (inst *Installer) isTargeted(name string, filter []string) bool {
	if len(filter) == 0 {
		return true
	}
	return contains(filter, name)
}

func (inst *Installer) knownNames() []string {
	names := make([]string, len(inst.mergers))
	for i, m := range inst.mergers {
		names[i] = m.Name()
	}
	return names
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
