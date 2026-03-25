package integration_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func buildCLI(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "flutree")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/flutree")
	cmd.Dir = projectRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, string(out))
	}
	return bin
}

func projectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve caller")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
	if _, err := os.Stat(root); err != nil {
		t.Fatal(err)
	}
	return root
}

type runResult struct {
	code   int
	stdout string
	stderr string
}

func runCLI(t *testing.T, bin, cwd string, env []string, stdin string, args ...string) runResult {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = cwd
	cmd.Env = env
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		code = 1
		if ee, ok := err.(*exec.ExitError); ok && ee.ProcessState != nil {
			code = ee.ProcessState.ExitCode()
		} else if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		}
		return runResult{code: code, stderr: string(out)}
	}
	return runResult{code: code, stdout: string(out)}
}

func runGit(t *testing.T, cwd string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
	return string(out)
}

func initRepo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, path, "init")
	runGit(t, path, "config", "user.email", "flutree@example.com")
	runGit(t, path, "config", "user.name", "Flutree Tests")
	if err := os.WriteFile(filepath.Join(path, "README.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "pubspec.yaml"), []byte("name: sample\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, path, "add", "README.md", "pubspec.yaml")
	runGit(t, path, "commit", "-m", "init")
	runGit(t, path, "checkout", "-B", "main")
}

func testEnv(home string) []string {
	env := os.Environ()
	env = append(env, "HOME="+home)
	return env
}

func writeRegistry(t *testing.T, home string, payload any) {
	t.Helper()
	regPath := filepath.Join(home, "Documents", "worktrees", ".worktrees_registry.json")
	if err := os.MkdirAll(filepath.Dir(regPath), 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(regPath, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCLIHelpListsExpectedCommands(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	res := runCLI(t, bin, projectRoot(t), testEnv(home), "", "--help")
	if res.code != 0 {
		t.Fatalf("expected 0, got %d (%s)", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "create") || !strings.Contains(res.stdout, "list") || !strings.Contains(res.stdout, "complete") {
		t.Fatalf("unexpected help output: %s", res.stdout)
	}
}

func TestUnknownCommandExitsNonZero(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	res := runCLI(t, bin, projectRoot(t), testEnv(home), "", "unknown-command")
	if res.code == 0 {
		t.Fatalf("expected non-zero exit")
	}
	if !strings.Contains(res.stderr, "No such command") {
		t.Fatalf("unexpected stderr: %s", res.stderr)
	}
}

func TestListWorksOutsideGitRepoUsingGlobalRegistry(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	writeRegistry(t, home, map[string]any{
		"version": 1,
		"records": []map[string]any{
			{
				"name":      "feature-login",
				"branch":    "feature/login",
				"path":      "/tmp/worktrees/feature-login",
				"repo_root": "/tmp/repo",
				"status":    "active",
			},
		},
	})
	outside := filepath.Join(t.TempDir(), "outside")
	_ = os.MkdirAll(outside, 0o755)
	res := runCLI(t, bin, outside, testEnv(home), "", "list")
	if res.code != 0 {
		t.Fatalf("expected 0, got %d (%s)", res.code, res.stderr)
	}
	if strings.Contains(res.stdout+res.stderr, "Current directory is not inside a Git repository") {
		t.Fatalf("unexpected precondition output: %s %s", res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, "feature-login") {
		t.Fatalf("missing registry entry in output: %s", res.stdout)
	}
}

func TestNonInteractiveCreateRequiresYes(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	repo := filepath.Join(scope, "root-app")
	initRepo(t, repo)

	res := runCLI(
		t, bin, repo, testEnv(home), "",
		"create", "feature-login",
		"--branch", "feature/login",
		"--scope", scope,
		"--root-repo", "root-app",
		"--non-interactive",
	)
	if res.code != 2 {
		t.Fatalf("expected 2, got %d (%s)", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, "Final confirmation token required in non-interactive mode") {
		t.Fatalf("unexpected stderr: %s", res.stderr)
	}
}

func TestInteractiveCreateWithYesStillRequiresToken(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	repo := filepath.Join(scope, "root-app")
	initRepo(t, repo)

	res := runCLI(
		t, bin, repo, testEnv(home), "NOPE\n",
		"create", "feature-login",
		"--branch", "feature/login",
		"--scope", scope,
		"--root-repo", "root-app",
		"--yes",
	)
	if res.code != 2 {
		t.Fatalf("expected 2, got %d (%s)", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, "Create cancelled before execution") {
		t.Fatalf("unexpected stderr: %s", res.stderr)
	}
}

func TestCompleteWorksOutsideRepoAndRetainsBranch(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	repo := filepath.Join(scope, "root-app")
	initRepo(t, repo)

	create := runCLI(
		t, bin, repo, testEnv(home), "",
		"create", "feature-login",
		"--branch", "feature/login",
		"--scope", scope,
		"--root-repo", "root-app",
		"--yes",
		"--non-interactive",
	)
	if create.code != 0 {
		t.Fatalf("create failed: %d %s", create.code, create.stderr)
	}

	outside := filepath.Join(t.TempDir(), "outside")
	_ = os.MkdirAll(outside, 0o755)
	complete := runCLI(t, bin, outside, testEnv(home), "", "complete", "feature-login", "--yes", "--force")
	if complete.code != 0 {
		t.Fatalf("complete failed: %d %s", complete.code, complete.stderr)
	}
	if strings.Contains(complete.stdout+complete.stderr, "Current directory is not inside a Git repository") {
		t.Fatalf("unexpected precondition output")
	}
	branches := runGit(t, repo, "branch", "--list", "feature/login")
	if !strings.Contains(branches, "feature/login") {
		t.Fatalf("expected branch retained, got: %s", branches)
	}
}
