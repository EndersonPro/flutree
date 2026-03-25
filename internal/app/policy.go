package app

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var nonSafe = regexp.MustCompile(`[^a-z0-9]+`)

func normalizeWorktreeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = nonSafe.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	if name == "" {
		return "workspace"
	}
	return name
}

func normalizeBranchName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "main"
	}
	return name
}

func defaultBranchFor(name string) string {
	return "feature/" + normalizeWorktreeName(name)
}

func destinationRoot() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Clean("./worktrees")
	}
	return filepath.Join(home, "Documents", "worktrees")
}
