package git

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/EndersonPro/flutree/internal/domain"
)

type Gateway struct{}

func (g *Gateway) EnsureRepo() (string, error) {
	out, err := g.run("", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	repo := strings.TrimSpace(out)
	if _, statErr := os.Stat(repo); statErr != nil {
		return "", domain.NewError(domain.CategoryPrecondition, 3, "Git reported a repository root that does not exist.", repo, statErr)
	}
	return repo, nil
}

func (g *Gateway) ListWorktrees(repoRoot string) ([]domain.GitWorktreeEntry, error) {
	out, err := g.run(repoRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parseWorktrees(out), nil
}

func (g *Gateway) CreateWorktree(repoRoot, path, branch, baseBranch string) error {
	_, err := g.run(repoRoot, "worktree", "add", "-b", branch, path, baseBranch)
	return err
}

func (g *Gateway) RemoveWorktree(repoRoot, path string, force bool) error {
	args := []string{"worktree", "remove", path}
	if force {
		args = append(args, "--force")
	}
	_, err := g.run(repoRoot, args...)
	return err
}

func (g *Gateway) IsDirty(path string) (bool, error) {
	out, err := g.run("", "-C", path, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

func (g *Gateway) DiscoverFlutterRepos(scope string) ([]domain.DiscoveredFlutterRepo, error) {
	scope = domain.NormalizePath(scope)
	info, err := os.Stat(scope)
	if err != nil || !info.IsDir() {
		return nil, domain.NewError(domain.CategoryPrecondition, 3, "Execution scope does not exist or is not a directory.", scope, err)
	}

	candidates := map[string]struct{}{}
	if _, err := os.Stat(filepath.Join(scope, ".git")); err == nil {
		candidates[scope] = struct{}{}
	}

	_ = filepath.WalkDir(scope, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() || d.Name() != ".git" {
			return nil
		}
		candidates[filepath.Dir(path)] = struct{}{}
		return filepath.SkipDir
	})

	repos := make([]domain.DiscoveredFlutterRepo, 0)
	keys := make([]string, 0, len(candidates))
	for k := range candidates {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, root := range keys {
		pkgName, ok := readPubspecName(filepath.Join(root, "pubspec.yaml"))
		if !ok {
			continue
		}
		repos = append(repos, domain.DiscoveredFlutterRepo{
			Name:        filepath.Base(root),
			RepoRoot:    root,
			PackageName: pkgName,
		})
	}

	if len(repos) == 0 {
		return nil, domain.NewError(domain.CategoryPrecondition, 3, "No Flutter repositories were discovered in execution scope.", "A repository is considered Flutter when it has pubspec.yaml at repo root.", nil)
	}

	return repos, nil
}

func (g *Gateway) run(cwd string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), nil
	}
	details := strings.TrimSpace(string(out))
	if strings.Contains(strings.ToLower(details), "not a git repository") {
		return "", domain.NewError(domain.CategoryPrecondition, 3, "Current directory is not inside a Git repository.", "Run the command from an existing repo root or child path.", err)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return "", domain.NewError(domain.CategoryGit, 4, fmt.Sprintf("Git command failed: git %s", strings.Join(args, " ")), details, err)
	}
	return "", domain.NewError(domain.CategoryGit, 4, "Failed to execute git binary.", err.Error(), err)
}

func parseWorktrees(output string) []domain.GitWorktreeEntry {
	entries := make([]domain.GitWorktreeEntry, 0)
	current := domain.GitWorktreeEntry{}
	has := false

	flush := func() {
		if has && current.Path != "" {
			entries = append(entries, current)
		}
		current = domain.GitWorktreeEntry{}
		has = false
	}

	s := bufio.NewScanner(strings.NewReader(output))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "worktree ") {
			flush()
			current.Path = strings.TrimPrefix(line, "worktree ")
			has = true
			continue
		}
		if strings.HasPrefix(line, "HEAD ") {
			current.Head = strings.TrimPrefix(line, "HEAD ")
			continue
		}
		if strings.HasPrefix(line, "branch refs/heads/") {
			current.Branch = strings.TrimPrefix(line, "branch refs/heads/")
			continue
		}
		if line == "bare" {
			current.IsBare = true
			continue
		}
		if line == "detached" {
			current.IsDetached = true
			continue
		}
		if strings.HasPrefix(line, "locked") {
			current.IsLocked = true
			if line != "locked" {
				current.PruneReason = strings.TrimSpace(strings.TrimPrefix(line, "locked"))
			}
		}
	}
	flush()
	return entries
}

func readPubspecName(path string) (string, bool) {
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer file.Close()

	s := bufio.NewScanner(file)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, "name:") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		name = strings.Trim(name, "\"'")
		return name, name != ""
	}
	return "", false
}
