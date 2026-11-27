// Package sdk reads Flutter SDK pins (fvm, puro) and runs the manager in a
// worktree.
package sdk

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/EndersonPro/flutree/internal/domain"
)

type Gateway struct{}

// Pins reads fvm (.fvmrc, legacy .fvm/fvm_config.json) and puro (.puro.json)
// pins from the worktree first, then from the source repository.
func (g *Gateway) Pins(worktreePath, sourceRepoRoot string) (domain.SDKPins, error) {
	dirs := []string{worktreePath}
	if sourceRepoRoot != "" && filepath.Clean(sourceRepoRoot) != filepath.Clean(worktreePath) {
		dirs = append(dirs, sourceRepoRoot)
	}
	var pins domain.SDKPins
	var err error
	if pins.FVMVersion, err = firstJSONString(dirs, []pinFile{{".fvmrc", "flutter"}, {filepath.Join(".fvm", "fvm_config.json"), "flutterSdkVersion"}}); err != nil {
		return pins, err
	}
	if pins.PuroEnv, err = firstJSONString(dirs, []pinFile{{".puro.json", "env"}}); err != nil {
		return pins, err
	}
	return pins, nil
}

// Use runs `fvm use <version> --force --skip-pub-get` or `puro use <env>` in
// the worktree. pub get is left to `flutree pubget`.
func (g *Gateway) Use(worktreePath string, manager domain.SDKManager, version string) error {
	var args []string
	switch manager {
	case domain.SDKManagerFVM:
		args = []string{"fvm", "use", version, "--force", "--skip-pub-get"}
	case domain.SDKManagerPuro:
		args = []string{"puro", "use", version}
	default:
		return fmt.Errorf("unsupported SDK manager %q", manager)
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = worktreePath
	out, err := cmd.CombinedOutput()
	if err != nil {
		details := strings.TrimSpace(string(out))
		if details == "" {
			details = err.Error()
		}
		return fmt.Errorf("%s: %s", strings.Join(args, " "), details)
	}
	return nil
}

type pinFile struct {
	name string
	key  string
}

func firstJSONString(dirs []string, files []pinFile) (string, error) {
	for _, dir := range dirs {
		for _, f := range files {
			path := filepath.Join(dir, f.name)
			b, err := os.ReadFile(path)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return "", err
			}
			var doc map[string]any
			if err := json.Unmarshal(b, &doc); err != nil {
				return "", fmt.Errorf("parse %s: %w", path, err)
			}
			if v, ok := doc[f.key].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v), nil
			}
		}
	}
	return "", nil
}
