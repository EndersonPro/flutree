package app

import "github.com/EndersonPro/flutree/internal/domain"

// sdkTarget is a freshly created worktree whose SDK must be prepared.
type sdkTarget struct {
	repo       string
	path       string
	sourceRoot string
}

// setupSDKs runs the preferred SDK manager in every target that declares a pin
// for it. Failures are reported per worktree and never undo the worktrees: the
// workspace stays usable and the user can rerun the manager by hand.
func setupSDKs(port SDKPort, preferenceFn func() (domain.SDKManager, error), targets []sdkTarget) []domain.SDKSetupResult {
	if port == nil {
		return nil
	}
	preference := domain.SDKManagerAuto
	if preferenceFn != nil {
		p, err := preferenceFn()
		if err != nil {
			return []domain.SDKSetupResult{{Status: "failed", Message: "read sdk.manager: " + err.Error()}}
		}
		preference = p
	}
	if preference == domain.SDKManagerNone {
		return nil
	}
	results := []domain.SDKSetupResult{}
	for _, target := range targets {
		pins, err := port.Pins(target.path, target.sourceRoot)
		if err != nil {
			results = append(results, domain.SDKSetupResult{Repo: target.repo, Path: target.path, Status: "failed", Message: err.Error()})
			continue
		}
		manager, version := chooseSDK(preference, pins)
		if manager == "" {
			continue
		}
		result := domain.SDKSetupResult{Repo: target.repo, Path: target.path, Manager: string(manager), Version: version, Status: "configured"}
		if err := port.Use(target.path, manager, version); err != nil {
			result.Status = "failed"
			result.Message = err.Error()
		}
		results = append(results, result)
	}
	return results
}

// chooseSDK picks the manager for one repo. auto prefers fvm when pinned,
// then puro; a forced manager only runs when the repo pins a version for it.
func chooseSDK(preference domain.SDKManager, pins domain.SDKPins) (domain.SDKManager, string) {
	switch preference {
	case domain.SDKManagerFVM:
		if pins.FVMVersion != "" {
			return domain.SDKManagerFVM, pins.FVMVersion
		}
	case domain.SDKManagerPuro:
		if pins.PuroEnv != "" {
			return domain.SDKManagerPuro, pins.PuroEnv
		}
	case domain.SDKManagerAuto, "":
		if pins.FVMVersion != "" {
			return domain.SDKManagerFVM, pins.FVMVersion
		}
		if pins.PuroEnv != "" {
			return domain.SDKManagerPuro, pins.PuroEnv
		}
	}
	return "", ""
}
