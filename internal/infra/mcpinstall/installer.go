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

// DefaultInstaller returns an Installer pre-loaded with all three client mergers
// and the real binary resolver. Use this in the CLI dispatch layer.
func DefaultInstaller() *Installer {
	home := "" // empty → real home dir
	return NewInstaller(
		[]Merger{
			NewClaudeMerger(home),
			NewOpenCodeMerger(home),
			NewCodexMerger(home),
		},
		RealExecResolver(),
	)
}

// Run iterates the configured mergers, applies the client filter, and returns
// one InstallResult per targeted client.
//
// clients is an optional list of client names to restrict the run. If empty,
// all mergers are considered. An unknown name causes an immediate error before
// any write.
//
// force is passed through to each Merger.Merge call.
func (inst *Installer) Run(clients []string, force bool) ([]InstallResult, error) {
	// Validate filter list before any I/O.
	if err := inst.validateFilter(clients); err != nil {
		return nil, err
	}

	absCmd, err := inst.execResolver()
	if err != nil {
		return nil, fmt.Errorf("resolve binary path: %w", err)
	}

	var results []InstallResult
	for _, m := range inst.mergers {
		if !inst.isTargeted(m.Name(), clients) {
			continue
		}

		detected, detectErr := m.Detect()
		if detectErr != nil {
			// I/O error during detection — surface as OutcomeError so the
			// caller knows something went wrong rather than silently skipping.
			results = append(results, InstallResult{
				Client:  m.Name(),
				Status:  OutcomeError,
				Message: detectErr.Error(),
			})
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

		outcome, mergeErr := m.Merge(absCmd, force)
		result := InstallResult{
			Client: m.Name(),
			Status: outcome,
		}
		if mergeErr != nil {
			result.Message = mergeErr.Error()
		}
		// Attach config path if the merger exposes it.
		if cp, ok := m.(MergerWithConfigPath); ok {
			result.ConfigPath = cp.ConfigPath()
		}
		results = append(results, result)
	}
	return results, nil
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
