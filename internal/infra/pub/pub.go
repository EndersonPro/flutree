package pub

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/EndersonPro/flutree/internal/domain"
)

type Gateway struct{}

func (g *Gateway) DetectTool(repoPath string) (domain.PubTool, error) {
	pubspecPath := filepath.Join(repoPath, "pubspec.yaml")
	b, err := os.ReadFile(pubspecPath)
	if err != nil {
		return "", domain.NewError(
			domain.CategoryPrecondition,
			3,
			"Failed to read pubspec.yaml.",
			pubspecPath,
			err,
		)
	}

	text := strings.ToLower(string(b))
	if strings.Contains(text, "sdk: flutter") || strings.Contains(text, "flutter:") {
		return domain.PubToolFlutter, nil
	}

	return domain.PubToolDart, nil
}

func (g *Gateway) Clean(repoPath string, tool domain.PubTool) error {
	switch tool {
	case domain.PubToolFlutter:
		return g.run(repoPath, "flutter", "clean")
	case domain.PubToolDart:
		if err := os.RemoveAll(filepath.Join(repoPath, ".dart_tool")); err != nil {
			return domain.NewError(
				domain.CategoryPersistence,
				5,
				"Failed to remove .dart_tool directory.",
				repoPath,
				err,
			)
		}
		return nil
	default:
		return domain.NewError(domain.CategoryInput, 2, "Unknown pub tool.", string(tool), nil)
	}
}

func (g *Gateway) RemoveLock(repoPath string) error {
	lockPath := filepath.Join(repoPath, "pubspec.lock")
	err := os.Remove(lockPath)
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return domain.NewError(domain.CategoryPersistence, 5, "Failed to remove pubspec.lock.", lockPath, err)
}

func (g *Gateway) Get(repoPath string, tool domain.PubTool) error {
	switch tool {
	case domain.PubToolFlutter:
		return g.run(repoPath, "flutter", "pub", "get")
	case domain.PubToolDart:
		return g.run(repoPath, "dart", "pub", "get")
	default:
		return domain.NewError(domain.CategoryInput, 2, "Unknown pub tool.", string(tool), nil)
	}
}

func (g *Gateway) run(cwd string, command string, args ...string) error {
	cmd := exec.Command(command, args...)
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	details := strings.TrimSpace(string(out))
	if details == "" {
		details = err.Error()
	}

	return domain.NewError(
		domain.CategoryUnexpected,
		1,
		fmt.Sprintf("Command failed: %s %s", command, strings.Join(args, " ")),
		details,
		err,
	)
}
