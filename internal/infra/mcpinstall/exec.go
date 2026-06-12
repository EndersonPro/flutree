package mcpinstall

import (
	"os"
	"path/filepath"
)

// resolveExecutable returns the absolute path of the running binary, resolving
// symlinks so Homebrew shims (which are symlinks into the Cellar) yield the real
// binary path rather than the shim path.
func resolveExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		// Fall back to the unresolved path if EvalSymlinks fails (e.g. the
		// symlink target no longer exists on a partially-installed Homebrew keg).
		return exe, nil
	}
	return resolved, nil
}
