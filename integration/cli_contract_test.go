package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
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

type writerFunc func(p []byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) {
	return f(p)
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

func runCLIWithPTY(t *testing.T, bin, cwd string, env []string, scriptedInputs []string, args ...string) runResult {
	t.Helper()

	cmd := exec.Command(bin, args...)
	cmd.Dir = cwd
	cmd.Env = append(append([]string{}, env...), "TERM=dumb", "COLORTERM=")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		t.Fatalf("failed to start PTY command: %v", err)
	}
	t.Cleanup(func() { _ = ptmx.Close() })

	var (
		mu  sync.Mutex
		buf bytes.Buffer
	)
	readDone := make(chan struct{})
	go func() {
		safeWriter := writerFunc(func(p []byte) (int, error) {
			mu.Lock()
			defer mu.Unlock()
			return buf.Write(p)
		})
		_, _ = io.Copy(safeWriter, ptmx)
		close(readDone)
	}()

	readOutput := func() string {
		mu.Lock()
		defer mu.Unlock()
		return buf.String()
	}
	respondedCursorPos := false
	respondedBackground := false
	for _, step := range scriptedInputs {
		parts := strings.SplitN(step, "::", 2)
		if len(parts) != 2 {
			t.Fatalf("invalid PTY script step %q, expected <waitFor>::<input>", step)
		}
		waitFor := parts[0]
		input := parts[1]

		deadline := time.Now().Add(5 * time.Second)
		for {
			output := readOutput()
			if !respondedCursorPos && strings.Contains(output, "\x1b[6n") {
				if _, err := ptmx.Write([]byte("\x1b[1;1R")); err != nil {
					t.Fatalf("failed to answer cursor position probe: %v", err)
				}
				respondedCursorPos = true
			}
			if !respondedBackground && strings.Contains(output, "\x1b]11;?") {
				if _, err := ptmx.Write([]byte("\x1b]11;rgb:0000/0000/0000\a")); err != nil {
					t.Fatalf("failed to answer background color probe: %v", err)
				}
				respondedBackground = true
			}
			if strings.Contains(output, waitFor) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("timeout waiting for %q in PTY output. Output so far: %s", waitFor, output)
			}
			time.Sleep(20 * time.Millisecond)
		}

		if _, err := ptmx.Write([]byte(input)); err != nil {
			t.Fatalf("failed to write PTY input %q: %v", input, err)
		}
	}

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- cmd.Wait()
	}()

	var waitErr error
	select {
	case waitErr = <-waitDone:
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		waitErr = <-waitDone
		t.Fatalf("timeout waiting for PTY command completion")
	}

	_ = ptmx.Close()
	<-readDone

	output := readOutput()
	if waitErr != nil {
		code := 1
		if ee, ok := waitErr.(*exec.ExitError); ok && ee.ProcessState != nil {
			code = ee.ProcessState.ExitCode()
		} else if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		}
		return runResult{code: code, stderr: output}
	}

	return runResult{code: 0, stdout: output}
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
	initRepoWithPackageName(t, path, "sample")
}

func initRepoWithPackageName(t *testing.T, path, packageName string) {
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
	if err := os.WriteFile(filepath.Join(path, "pubspec.yaml"), []byte("name: "+packageName+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, path, "add", "README.md", "pubspec.yaml")
	runGit(t, path, "commit", "-m", "init")
	runGit(t, path, "checkout", "-B", "main")
	remote := filepath.Join(t.TempDir(), "origin.git")
	runGit(t, filepath.Dir(remote), "init", "--bare", remote)
	runGit(t, path, "remote", "add", "origin", remote)
	runGit(t, path, "push", "-u", "origin", "main")
}

func testEnv(home string) []string {
	env := os.Environ()
	env = append(env, "HOME="+home)
	// Ensure terminal is wide enough for list table output in CI
	env = append(env, "COLUMNS=200")
	return env
}

func testEnvWithPath(home, pathValue string) []string {
	env := testEnv(home)
	env = append(env, "PATH="+pathValue)
	return env
}

func withPath(env []string, dir string) []string {
	next := make([]string, 0, len(env)+1)
	next = append(next, env...)
	next = append(next, "PATH="+dir)
	return next
}

func writeFakeBrew(t *testing.T, dir string, script string) {
	t.Helper()
	path := filepath.Join(dir, "brew")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
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
	want := []string{"create", "config", "list", "complete", "clean", "mcp"}
	for _, cmd := range want {
		if !strings.Contains(res.stdout, cmd) {
			t.Errorf("help output missing command %q: %s", cmd, res.stdout)
		}
	}
	if !strings.Contains(res.stdout, "flutree <subcommand> --help") {
		t.Fatalf("expected subcommand help hint, got: %s", res.stdout)
	}
}

func TestSubcommandHelpContracts(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	_ = os.MkdirAll(outside, 0o755)
	env := testEnvWithPath(home, "")

	cases := []struct {
		name     string
		args     []string
		contains []string
	}{
		{
			name:     "config help",
			args:     []string{"config", "--help"},
			contains: []string{"flutree config set scope.root <path>", "flutree config get scope.root", "scope.root"},
		},
		{
			name:     "create long help",
			args:     []string{"create", "--help"},
			contains: []string{"flutree create <name> [options]", "--branch", "--root-repo", "--no-package", "--package", "--package-base", "--copy-root-file"},
		},
		{
			name:     "create short help",
			args:     []string{"create", "-h"},
			contains: []string{"flutree create <name> [options]", "--branch", "--root-repo", "--no-package", "--package"},
		},
		{
			name:     "add-repo help",
			args:     []string{"add-repo", "--help"},
			contains: []string{"flutree add-repo <workspace> [options]", "--repo", "--package-base", "--package-branch-source", "--sync-policy", "--reuse-existing-branch", "--copy-root-file"},
		},
		{
			name:     "complete help",
			args:     []string{"complete", "--help"},
			contains: []string{"flutree complete <name> [options]", "--yes", "--force"},
		},
		{
			name:     "pubget help",
			args:     []string{"pubget", "--help"},
			contains: []string{"flutree pubget <name> [options]", "--force"},
		},
		{
			name:     "clean help",
			args:     []string{"clean", "--help"},
			contains: []string{"flutree clean [options]", "--force"},
		},
		{
			name:     "list help",
			args:     []string{"list", "--help"},
			contains: []string{"flutree list [options]", "--all", "--global"},
		},
		{
			name:     "update help",
			args:     []string{"update", "--help"},
			contains: []string{"flutree update [options]", "--check", "--apply"},
		},
		{
			name:     "version help",
			args:     []string{"version", "--help"},
			contains: []string{"flutree version", "-h, --help"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := runCLI(t, bin, outside, env, "", tc.args...)
			if res.code != 0 {
				t.Fatalf("expected 0, got %d (%s)", res.code, res.stderr)
			}
			for _, want := range tc.contains {
				if !strings.Contains(res.stdout, want) {
					t.Fatalf("help output missing %q: %s", want, res.stdout)
				}
			}
		})
	}
}

func TestConfigSetGetScopeRootRoundTrip(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := t.TempDir()

	setRes := runCLI(t, bin, projectRoot(t), testEnv(home), "", "config", "set", "scope.root", scope)
	if setRes.code != 0 {
		t.Fatalf("expected config set success, got %d (%s)", setRes.code, setRes.stderr)
	}
	want := filepath.Clean(scope)
	if strings.TrimSpace(setRes.stdout) != want {
		t.Fatalf("expected normalized set output %q, got %q", want, setRes.stdout)
	}

	getRes := runCLI(t, bin, projectRoot(t), testEnv(home), "", "config", "get", "scope.root")
	if getRes.code != 0 {
		t.Fatalf("expected config get success, got %d (%s)", getRes.code, getRes.stderr)
	}
	if strings.TrimSpace(getRes.stdout) != want {
		t.Fatalf("expected persisted get output %q, got %q", want, getRes.stdout)
	}
}

func TestConfigRejectsUnsupportedKeys(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()

	setRes := runCLI(t, bin, projectRoot(t), testEnv(home), "", "config", "set", "other.key", "/tmp")
	if setRes.code != 2 {
		t.Fatalf("expected config set unsupported key to fail with 2, got %d (%s)", setRes.code, setRes.stderr)
	}
	if !strings.Contains(setRes.stderr, "Unsupported config key") {
		t.Fatalf("unexpected set stderr: %s", setRes.stderr)
	}

	getRes := runCLI(t, bin, projectRoot(t), testEnv(home), "", "config", "get", "other.key")
	if getRes.code != 2 {
		t.Fatalf("expected config get unsupported key to fail with 2, got %d (%s)", getRes.code, getRes.stderr)
	}
	if !strings.Contains(getRes.stderr, "Unsupported config key") {
		t.Fatalf("unexpected get stderr: %s", getRes.stderr)
	}
}

func TestConfigSetRejectsInvalidAndNonDirectoryPaths(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	missing := filepath.Join(t.TempDir(), "missing")

	missingRes := runCLI(t, bin, projectRoot(t), testEnv(home), "", "config", "set", "scope.root", missing)
	if missingRes.code != 2 {
		t.Fatalf("expected missing path failure with code 2, got %d (%s)", missingRes.code, missingRes.stderr)
	}
	if !strings.Contains(missingRes.stderr, "does not exist") {
		t.Fatalf("unexpected missing-path stderr: %s", missingRes.stderr)
	}

	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	nonDirRes := runCLI(t, bin, projectRoot(t), testEnv(home), "", "config", "set", "scope.root", file)
	if nonDirRes.code != 2 {
		t.Fatalf("expected non-directory failure with code 2, got %d (%s)", nonDirRes.code, nonDirRes.stderr)
	}
	if !strings.Contains(nonDirRes.stderr, "must be a directory") {
		t.Fatalf("unexpected non-directory stderr: %s", nonDirRes.stderr)
	}

	if runtime.GOOS != "windows" {
		unreachable := filepath.Join(t.TempDir(), "private")
		if err := os.MkdirAll(unreachable, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(unreachable, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(unreachable, 0o700) })

		unreachableRes := runCLI(t, bin, projectRoot(t), testEnv(home), "", "config", "set", "scope.root", unreachable)
		if unreachableRes.code != 2 {
			t.Fatalf("expected unreachable path failure with code 2, got %d (%s)", unreachableRes.code, unreachableRes.stderr)
		}
		if !strings.Contains(unreachableRes.stderr, "not reachable") {
			t.Fatalf("unexpected unreachable-path stderr: %s", unreachableRes.stderr)
		}
	}
}

func TestCreateUsesPersistedScopeWhenFlagOmitted(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	repo := filepath.Join(scope, "root-app")
	initRepo(t, repo)

	setRes := runCLI(t, bin, projectRoot(t), testEnv(home), "", "config", "set", "scope.root", scope)
	if setRes.code != 0 {
		t.Fatalf("config set failed: %d (%s)", setRes.code, setRes.stderr)
	}

	create := runCLI(
		t, bin, repo, testEnv(home), "",
		"create", "feature-login",
		"--root-repo", "root-app",
		"--yes",
		"--non-interactive",
	)
	if create.code != 0 {
		t.Fatalf("create without --scope should use persisted root, got %d (%s)", create.code, create.stderr)
	}
}

func TestCreateExplicitScopeOverridesPersistedScope(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	persistedScope := filepath.Join(t.TempDir(), "persisted")
	explicitScope := filepath.Join(t.TempDir(), "explicit")
	persistedRepo := filepath.Join(persistedScope, "persisted-root")
	explicitRepo := filepath.Join(explicitScope, "explicit-root")
	initRepo(t, persistedRepo)
	initRepo(t, explicitRepo)

	setRes := runCLI(t, bin, projectRoot(t), testEnv(home), "", "config", "set", "scope.root", persistedScope)
	if setRes.code != 0 {
		t.Fatalf("config set failed: %d (%s)", setRes.code, setRes.stderr)
	}

	create := runCLI(
		t, bin, explicitRepo, testEnv(home), "",
		"create", "feature-login",
		"--scope", explicitScope,
		"--root-repo", "explicit-root",
		"--yes",
		"--non-interactive",
	)
	if create.code != 0 {
		t.Fatalf("create with explicit --scope should override persisted root, got %d (%s)", create.code, create.stderr)
	}
}

func TestAddRepoUsesPersistedScopeWhenFlagOmitted(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	rootRepo := filepath.Join(scope, "root-app")
	coreRepo := filepath.Join(scope, "core-pkg")
	initRepoWithPackageName(t, rootRepo, "root_app")
	initRepoWithPackageName(t, coreRepo, "core")

	setRes := runCLI(t, bin, projectRoot(t), testEnv(home), "", "config", "set", "scope.root", scope)
	if setRes.code != 0 {
		t.Fatalf("config set failed: %d (%s)", setRes.code, setRes.stderr)
	}

	create := runCLI(
		t, bin, rootRepo, testEnv(home), "",
		"create", "feature-login",
		"--root-repo", "root-app",
		"--yes",
		"--non-interactive",
	)
	if create.code != 0 {
		t.Fatalf("create failed: %d %s", create.code, create.stderr)
	}

	add := runCLI(
		t, bin, rootRepo, testEnv(home), "",
		"add-repo", "feature-login",
		"--repo", "core-pkg",
		"--non-interactive",
	)
	if add.code != 0 {
		t.Fatalf("add-repo without --scope should use persisted root, got %d (%s)", add.code, add.stderr)
	}
}

func TestAddRepoExplicitScopeOverridesPersistedScope(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	persistedScope := filepath.Join(t.TempDir(), "persisted")
	explicitScope := filepath.Join(t.TempDir(), "explicit")

	persistedRoot := filepath.Join(persistedScope, "root-app")
	explicitRoot := filepath.Join(explicitScope, "root-app")
	explicitCore := filepath.Join(explicitScope, "core-pkg")
	initRepoWithPackageName(t, persistedRoot, "root_app")
	initRepoWithPackageName(t, explicitRoot, "root_app")
	initRepoWithPackageName(t, explicitCore, "core")

	setRes := runCLI(t, bin, projectRoot(t), testEnv(home), "", "config", "set", "scope.root", persistedScope)
	if setRes.code != 0 {
		t.Fatalf("config set failed: %d (%s)", setRes.code, setRes.stderr)
	}

	create := runCLI(
		t, bin, explicitRoot, testEnv(home), "",
		"create", "feature-login",
		"--scope", explicitScope,
		"--root-repo", "root-app",
		"--yes",
		"--non-interactive",
	)
	if create.code != 0 {
		t.Fatalf("create failed: %d %s", create.code, create.stderr)
	}

	add := runCLI(
		t, bin, explicitRoot, testEnv(home), "",
		"add-repo", "feature-login",
		"--scope", explicitScope,
		"--repo", "core-pkg",
		"--non-interactive",
	)
	if add.code != 0 {
		t.Fatalf("add-repo with explicit --scope should override persisted scope, got %d (%s)", add.code, add.stderr)
	}
}

func TestMissingPositionalStillFailsWithoutHelp(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	_ = os.MkdirAll(outside, 0o755)

	create := runCLI(t, bin, outside, testEnv(home), "", "create")
	if create.code != 2 {
		t.Fatalf("expected 2, got %d (%s)", create.code, create.stderr)
	}
	if !strings.Contains(create.stderr, "Missing worktree name") {
		t.Fatalf("unexpected create stderr: %s", create.stderr)
	}

	addRepo := runCLI(t, bin, outside, testEnv(home), "", "add-repo")
	if addRepo.code != 2 {
		t.Fatalf("expected 2, got %d (%s)", addRepo.code, addRepo.stderr)
	}
	if !strings.Contains(addRepo.stderr, "Missing workspace name") {
		t.Fatalf("unexpected add-repo stderr: %s", addRepo.stderr)
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

func TestCreateNoPackageRejectsPackageFlagDeterministically(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	repo := filepath.Join(scope, "root-app")
	initRepo(t, repo)

	run := func() runResult {
		return runCLI(
			t, bin, repo, testEnv(home), "",
			"create", "feature-login",
			"--scope", scope,
			"--root-repo", "root-app",
			"--no-package",
			"--package", "core",
			"--yes",
			"--non-interactive",
		)
	}

	first := run()
	second := run()
	if first.code != 2 || second.code != 2 {
		t.Fatalf("expected exit code 2 for both runs, got %d and %d", first.code, second.code)
	}
	if !strings.Contains(first.stderr, "--no-package") || !strings.Contains(first.stderr, "--package") {
		t.Fatalf("expected deterministic conflict message, got: %s", first.stderr)
	}
	if first.stderr != second.stderr {
		t.Fatalf("expected deterministic stderr for same invalid flags. first=%q second=%q", first.stderr, second.stderr)
	}
}

func TestCreateNoPackageRejectsPackageBaseFlagDeterministically(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	repo := filepath.Join(scope, "root-app")
	initRepo(t, repo)

	res := runCLI(
		t, bin, repo, testEnv(home), "",
		"create", "feature-login",
		"--scope", scope,
		"--root-repo", "root-app",
		"--no-package",
		"--package-base", "core=main",
		"--yes",
		"--non-interactive",
	)
	if res.code != 2 {
		t.Fatalf("expected 2, got %d (%s)", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, "--no-package") || !strings.Contains(res.stderr, "--package-base") {
		t.Fatalf("expected conflict message with incompatible flags, got: %s", res.stderr)
	}
}

func TestCreateNoPackageCreatesRootOnlyArtifacts(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	rootRepo := filepath.Join(scope, "root-app")
	coreRepo := filepath.Join(scope, "core-pkg")
	initRepoWithPackageName(t, rootRepo, "root_app")
	initRepoWithPackageName(t, coreRepo, "core")

	create := runCLI(
		t, bin, rootRepo, testEnv(home), "",
		"create", "feature-login",
		"--branch", "feature/login",
		"--scope", scope,
		"--root-repo", "root-app",
		"--no-package",
		"--yes",
		"--non-interactive",
	)
	if create.code != 0 {
		t.Fatalf("create failed: %d %s", create.code, create.stderr)
	}

	overridePath := filepath.Join(home, "Documents", "worktrees", "feature-login", "root", "root-app", "pubspec_overrides.yaml")
	content, err := os.ReadFile(overridePath)
	if err != nil {
		t.Fatalf("failed to read override file: %v", err)
	}
	if !strings.Contains(string(content), "dependency_overrides:\n  {}") {
		t.Fatalf("expected empty override map in no-package mode, got: %s", string(content))
	}

	pkgPath := filepath.Join(home, "Documents", "worktrees", "feature-login", "packages")
	if _, err := os.Stat(pkgPath); !os.IsNotExist(err) {
		t.Fatalf("expected no packages directory in no-package mode, stat err=%v", err)
	}
}

func TestListGlobalIncludesRowsOutsideCurrentRepo(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()

	repoA := filepath.Join(t.TempDir(), "repo-a")
	repoB := filepath.Join(t.TempDir(), "repo-b")
	initRepo(t, repoA)
	initRepo(t, repoB)
	repoARoot := strings.TrimSpace(runGit(t, repoA, "rev-parse", "--show-toplevel"))
	repoBRoot := strings.TrimSpace(runGit(t, repoB, "rev-parse", "--show-toplevel"))

	writeRegistry(t, home, map[string]any{
		"version": 1,
		"records": []map[string]any{
			{
				"name":      "feature-a",
				"branch":    "feature/a",
				"path":      "/tmp/worktrees/feature-a",
				"repo_root": repoARoot,
				"status":    "active",
			},
			{
				"name":      "feature-b",
				"branch":    "feature/b",
				"path":      "/tmp/worktrees/feature-b",
				"repo_root": repoBRoot,
				"status":    "active",
			},
		},
	})

	defaultRes := runCLI(t, bin, repoA, testEnv(home), "", "list")
	if defaultRes.code != 0 {
		t.Fatalf("expected 0 for default list, got %d (%s)", defaultRes.code, defaultRes.stderr)
	}
	if !strings.Contains(defaultRes.stdout, "feature-a") || strings.Contains(defaultRes.stdout, "feature-b") {
		t.Fatalf("expected current-repo scoped list by default, got: %s", defaultRes.stdout)
	}

	globalRes := runCLI(t, bin, repoA, testEnv(home), "", "list", "--global")
	if globalRes.code != 0 {
		t.Fatalf("expected 0 for global list, got %d (%s)", globalRes.code, globalRes.stderr)
	}
	if !strings.Contains(globalRes.stdout, "feature-a") || !strings.Contains(globalRes.stdout, "feature-b") {
		t.Fatalf("expected global list to include all repo rows, got: %s", globalRes.stdout)
	}
}

func TestListGlobalAllIncludesUnmanagedAcrossRepos(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()

	repoA := filepath.Join(t.TempDir(), "repo-a")
	repoB := filepath.Join(t.TempDir(), "repo-b")
	initRepo(t, repoA)
	initRepo(t, repoB)
	repoARoot := strings.TrimSpace(runGit(t, repoA, "rev-parse", "--show-toplevel"))
	repoBRoot := strings.TrimSpace(runGit(t, repoB, "rev-parse", "--show-toplevel"))

	worktreeA := filepath.Join(t.TempDir(), "wt-a-unmanaged")
	worktreeB := filepath.Join(t.TempDir(), "wt-b-unmanaged")
	runGit(t, repoA, "worktree", "add", "-b", "feature/unmanaged-a", worktreeA, "main")
	runGit(t, repoB, "worktree", "add", "-b", "feature/unmanaged-b", worktreeB, "main")

	writeRegistry(t, home, map[string]any{
		"version": 1,
		"records": []map[string]any{
			{
				"name":      "feature-a",
				"branch":    "feature/a",
				"path":      "/tmp/worktrees/feature-a",
				"repo_root": repoARoot,
				"status":    "active",
			},
			{
				"name":      "feature-b",
				"branch":    "feature/b",
				"path":      "/tmp/worktrees/feature-b",
				"repo_root": repoBRoot,
				"status":    "active",
			},
		},
	})

	res := runCLI(t, bin, repoA, testEnv(home), "", "list", "--global", "--all")
	if res.code != 0 {
		t.Fatalf("expected 0, got %d (%s)", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, filepath.Base(worktreeA)) || !strings.Contains(res.stdout, filepath.Base(worktreeB)) {
		t.Fatalf("expected unmanaged rows from both repositories, got: %s", res.stdout)
	}
}

func TestNonInteractiveCreateRequiresYes(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	repo := filepath.Join(scope, "root-app")
	initRepo(t, repo)
	runGit(t, repo, "branch", "feature/login")

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
	runGit(t, repo, "branch", "feature/login")

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

func TestInteractiveCreatePromptsForRemoteSyncBeforeApply(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	repo := filepath.Join(scope, "root-app")
	initRepo(t, repo)

	res := runCLI(
		t, bin, repo, testEnv(home), "APPLY\ny\n",
		"create", "feature-login",
		"--branch", "feature/login",
		"--scope", scope,
		"--root-repo", "root-app",
	)
	if res.code != 0 {
		t.Fatalf("expected 0, got %d (%s)", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "Update local branches from origin before creating worktrees?") {
		t.Fatalf("expected sync confirmation prompt, got: %s", res.stdout)
	}
}

func TestInteractiveCreateWithSyncDeclinedDoesNotFetchRemote(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	repo := filepath.Join(scope, "root-app")
	initRepo(t, repo)

	runGit(t, repo, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing-origin.git"))

	res := runCLI(
		t, bin, repo, testEnv(home), "APPLY\nN\n",
		"create", "feature-login",
		"--branch", "feature/login",
		"--scope", scope,
		"--root-repo", "root-app",
	)
	if res.code != 0 {
		t.Fatalf("expected 0 when sync declined, got %d (%s)", res.code, res.stderr)
	}
	if strings.Contains(res.stderr, "Failed to sync base branch from origin before creating worktree") {
		t.Fatalf("unexpected remote sync failure when user declined sync: %s", res.stderr)
	}

	rootWorktree := filepath.Join(home, "Documents", "worktrees", "feature-login", "root", "root-app")
	if _, err := os.Stat(rootWorktree); err != nil {
		t.Fatalf("expected root worktree to be created without remote sync, err=%v", err)
	}
}

func TestNonInteractiveCreateRequiresExplicitReuseFlagWhenBranchExists(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	repo := filepath.Join(scope, "root-app")
	initRepo(t, repo)
	runGit(t, repo, "branch", "feature/login")

	res := runCLI(
		t, bin, repo, testEnv(home), "",
		"create", "feature-login",
		"--branch", "feature/login",
		"--scope", scope,
		"--root-repo", "root-app",
		"--yes",
		"--non-interactive",
	)
	if res.code != 2 {
		t.Fatalf("expected 2, got %d (%s)", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, "Existing branch reuse requires explicit --reuse-existing-branch") {
		t.Fatalf("unexpected stderr: %s", res.stderr)
	}
}

func TestNonInteractiveCreateAllowsReuseWithExplicitFlag(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	repo := filepath.Join(scope, "root-app")
	initRepo(t, repo)
	runGit(t, repo, "branch", "feature/login")

	res := runCLI(
		t, bin, repo, testEnv(home), "",
		"create", "feature-login",
		"--branch", "feature/login",
		"--scope", scope,
		"--root-repo", "root-app",
		"--yes",
		"--non-interactive",
		"--reuse-existing-branch",
	)
	if res.code != 0 {
		t.Fatalf("expected 0, got %d (%s)", res.code, res.stderr)
	}
}

func TestCreateAcceptsPackageFlags(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	rootRepo := filepath.Join(scope, "root-app")
	coreRepo := filepath.Join(scope, "core-pkg")
	initRepoWithPackageName(t, rootRepo, "root_app")
	initRepoWithPackageName(t, coreRepo, "core")

	create := runCLI(
		t, bin, rootRepo, testEnv(home), "",
		"create", "feature-login",
		"--branch", "feature/login",
		"--scope", scope,
		"--root-repo", "root-app",
		"--package", "core-pkg",
		"--package-base", "core-pkg=main",
		"--yes",
		"--non-interactive",
	)
	if create.code != 0 {
		t.Fatalf("create failed: %d %s", create.code, create.stderr)
	}

	overridePath := filepath.Join(home, "Documents", "worktrees", "feature-login", "root", "root-app", "pubspec_overrides.yaml")
	content, err := os.ReadFile(overridePath)
	if err != nil {
		t.Fatalf("failed to read override file: %v", err)
	}
	if !strings.Contains(string(content), "core:") || !strings.Contains(string(content), "packages/core-pkg") {
		t.Fatalf("expected create override to include selected package, got: %s", string(content))
	}
}

func TestCreateCopiesEnvFilesByDefault(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	repo := filepath.Join(scope, "root-app")
	initRepoWithPackageName(t, repo, "root_app")

	if err := os.WriteFile(filepath.Join(repo, ".env"), []byte("TOKEN=abc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".env.dev"), []byte("TOKEN=dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".env", ".env.dev")
	runGit(t, repo, "commit", "-m", "add env fixtures")
	runGit(t, repo, "push", "origin", "main")

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

	rootWorktree := filepath.Join(home, "Documents", "worktrees", "feature-login", "root", "root-app")
	if _, err := os.Stat(filepath.Join(rootWorktree, ".env")); err != nil {
		t.Fatalf("expected .env copied, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(rootWorktree, ".env.dev")); err != nil {
		t.Fatalf("expected .env.dev copied, err=%v", err)
	}
}

func TestCreateBranchHasNoUpstreamTracking(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	repo := filepath.Join(scope, "root-app")
	initRepoWithPackageName(t, repo, "root_app")

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

	rootWorktree := filepath.Join(home, "Documents", "worktrees", "feature-login", "root", "root-app")
	upstream := strings.TrimSpace(runGit(t, rootWorktree, "for-each-ref", "--format=%(upstream:short)", "refs/heads/feature/login"))
	if upstream != "" {
		t.Fatalf("expected no upstream tracking for feature/login, got %q", upstream)
	}
}

func TestAddRepoAttachesRepositoryAndUpdatesOverride(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	rootRepo := filepath.Join(scope, "root-app")
	coreRepo := filepath.Join(scope, "core-pkg")
	initRepoWithPackageName(t, rootRepo, "root_app")
	initRepoWithPackageName(t, coreRepo, "core")

	create := runCLI(
		t, bin, rootRepo, testEnv(home), "",
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

	add := runCLI(
		t, bin, rootRepo, testEnv(home), "",
		"add-repo", "feature-login",
		"--scope", scope,
		"--repo", "core-pkg",
		"--non-interactive",
	)
	if add.code != 0 {
		t.Fatalf("add-repo failed: %d %s", add.code, add.stderr)
	}

	overridePath := filepath.Join(home, "Documents", "worktrees", "feature-login", "root", "root-app", "pubspec_overrides.yaml")
	content, err := os.ReadFile(overridePath)
	if err != nil {
		t.Fatalf("failed to read override file: %v", err)
	}
	got := string(content)
	if !strings.Contains(got, "core:") || !strings.Contains(got, "packages/core-pkg") {
		t.Fatalf("override file missing attached repo entry: %s", got)
	}
}

func TestAddRepoSyncPolicyNeverSkipsRemoteSyncAndSucceeds(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	rootRepo := filepath.Join(scope, "root-app")
	coreRepo := filepath.Join(scope, "core-pkg")
	initRepoWithPackageName(t, rootRepo, "root_app")
	initRepoWithPackageName(t, coreRepo, "core")

	create := runCLI(
		t, bin, rootRepo, testEnv(home), "",
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

	runGit(t, coreRepo, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing-origin.git"))

	add := runCLI(
		t, bin, rootRepo, testEnv(home), "",
		"add-repo", "feature-login",
		"--scope", scope,
		"--repo", "core-pkg",
		"--sync-policy", "never",
		"--non-interactive",
	)
	if add.code != 0 {
		t.Fatalf("expected sync-policy=never to succeed, got %d (%s)", add.code, add.stderr)
	}
}

func TestAddRepoInteractiveAutoSyncPromptsBeforeAttach(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	rootRepo := filepath.Join(scope, "root-app")
	coreRepo := filepath.Join(scope, "core-pkg")
	initRepoWithPackageName(t, rootRepo, "root_app")
	initRepoWithPackageName(t, coreRepo, "core")

	create := runCLI(
		t, bin, rootRepo, testEnv(home), "",
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

	add := runCLI(
		t, bin, rootRepo, testEnv(home), "y\n",
		"add-repo", "feature-login",
		"--scope", scope,
		"--repo", "core-pkg",
		"--package-base", "core-pkg=main",
	)
	if add.code != 0 {
		t.Fatalf("expected interactive add-repo success, got %d (%s)", add.code, add.stderr)
	}
	if !strings.Contains(add.stdout, "Update local branches from origin before creating attached worktrees?") {
		t.Fatalf("expected sync prompt in interactive mode, got: %s", add.stdout)
	}
}

func TestAddRepoInteractiveWizardApplyAttachesSelectedRepo(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	rootRepo := filepath.Join(scope, "root-app")
	coreRepo := filepath.Join(scope, "core-pkg")
	initRepoWithPackageName(t, rootRepo, "root_app")
	initRepoWithPackageName(t, coreRepo, "core")

	create := runCLI(
		t, bin, rootRepo, testEnv(home), "",
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

	add := runCLIWithPTY(t, bin, rootRepo, testEnv(home), []string{
		"Step 1 - Select repositories::\r",
		"Step 2 - Review and confirm::\r",
		"Step 3 - Configure branches::\r",
		"Step 3 - Configure branches::\r",
	}, "add-repo", "feature-login", "--scope", scope)
	if add.code != 0 {
		t.Fatalf("expected interactive wizard apply success, got %d (%s)", add.code, add.stderr)
	}

	overridePath := filepath.Join(home, "Documents", "worktrees", "feature-login", "root", "root-app", "pubspec_overrides.yaml")
	content, err := os.ReadFile(overridePath)
	if err != nil {
		t.Fatalf("failed to read override file: %v", err)
	}
	got := string(content)
	if !strings.Contains(got, "core:") || !strings.Contains(got, "packages/core-pkg") {
		t.Fatalf("override file missing attached repo entry after interactive apply: %s", got)
	}
}

func TestAddRepoInteractiveWizardCancelFromReviewHasNoSideEffects(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	rootRepo := filepath.Join(scope, "root-app")
	coreRepo := filepath.Join(scope, "core-pkg")
	initRepoWithPackageName(t, rootRepo, "root_app")
	initRepoWithPackageName(t, coreRepo, "core")

	create := runCLI(
		t, bin, rootRepo, testEnv(home), "",
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

	add := runCLIWithPTY(t, bin, rootRepo, testEnv(home), []string{
		"Step 1 - Select repositories::\r",
		"Step 2 - Review and confirm::\x1b[A\r",
	}, "add-repo", "feature-login", "--scope", scope)
	if add.code != 2 {
		t.Fatalf("expected cancellation error code 2, got %d (%s)", add.code, add.stderr)
	}
	if !strings.Contains(add.stderr, "Add-repo cancelled before execution") {
		t.Fatalf("expected cancellation guidance, got: %s", add.stderr)
	}

	pkgWorktree := filepath.Join(home, "Documents", "worktrees", "feature-login", "packages", "core-pkg")
	if _, err := os.Stat(pkgWorktree); !os.IsNotExist(err) {
		t.Fatalf("expected no package worktree after review cancel, stat err=%v", err)
	}

	overridePath := filepath.Join(home, "Documents", "worktrees", "feature-login", "root", "root-app", "pubspec_overrides.yaml")
	content, err := os.ReadFile(overridePath)
	if err == nil && strings.Contains(string(content), "core:") {
		t.Fatalf("expected no override entry for canceled attach, got: %s", string(content))
	}
}

func TestAddRepoNonInteractiveWithoutRepoSelectorsFailsDeterministically(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	rootRepo := filepath.Join(scope, "root-app")
	coreRepo := filepath.Join(scope, "core-pkg")
	initRepoWithPackageName(t, rootRepo, "root_app")
	initRepoWithPackageName(t, coreRepo, "core")

	create := runCLI(
		t, bin, rootRepo, testEnv(home), "",
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

	add := runCLI(
		t, bin, rootRepo, testEnv(home), "",
		"add-repo", "feature-login",
		"--scope", scope,
		"--non-interactive",
	)
	if add.code != 2 {
		t.Fatalf("expected deterministic non-interactive validation failure, got %d (%s)", add.code, add.stderr)
	}
	if !strings.Contains(add.stderr, "Repository selection is required in non-interactive mode") {
		t.Fatalf("expected missing-selector guidance, got: %s", add.stderr)
	}
}

func TestAddRepoNonInteractiveSyncAlwaysFailsFastAndRollsBack(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	rootRepo := filepath.Join(scope, "root-app")
	coreRepo := filepath.Join(scope, "core-pkg")
	initRepoWithPackageName(t, rootRepo, "root_app")
	initRepoWithPackageName(t, coreRepo, "core")

	create := runCLI(
		t, bin, rootRepo, testEnv(home), "",
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

	runGit(t, coreRepo, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing-origin.git"))

	add := runCLI(
		t, bin, rootRepo, testEnv(home), "",
		"add-repo", "feature-login",
		"--scope", scope,
		"--repo", "core-pkg",
		"--sync-policy", "always",
		"--non-interactive",
	)
	if add.code != 1 {
		t.Fatalf("expected sync-policy=always to fail, got %d (%s)", add.code, add.stderr)
	}
	if !strings.Contains(add.stderr, "Failed to sync") {
		t.Fatalf("expected actionable sync failure, got: %s", add.stderr)
	}

	pkgWorktree := filepath.Join(home, "Documents", "worktrees", "feature-login", "packages", "core-pkg")
	if _, err := os.Stat(pkgWorktree); !os.IsNotExist(err) {
		t.Fatalf("expected rollback to remove package worktree, stat err=%v", err)
	}
}

func TestAddRepoRejectsInvalidPackageBranchSourceFormat(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	_ = os.MkdirAll(outside, 0o755)

	res := runCLI(t, bin, outside, testEnv(home), "", "add-repo", "feature-login", "--package-branch-source", "core-pkg", "--non-interactive")
	if res.code != 2 {
		t.Fatalf("expected 2, got %d (%s)", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, "Invalid --package-branch-source format") {
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

func TestVersionCommandsPrintSameStableValue(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()

	flagVersion := runCLI(t, bin, projectRoot(t), testEnv(home), "", "--version")
	if flagVersion.code != 0 {
		t.Fatalf("expected 0 for --version, got %d (%s)", flagVersion.code, flagVersion.stderr)
	}
	cmdVersion := runCLI(t, bin, projectRoot(t), testEnv(home), "", "version")
	if cmdVersion.code != 0 {
		t.Fatalf("expected 0 for version, got %d (%s)", cmdVersion.code, cmdVersion.stderr)
	}
	if strings.TrimSpace(flagVersion.stdout) == "" {
		t.Fatalf("expected non-empty version output")
	}
	if strings.TrimSpace(flagVersion.stdout) != strings.TrimSpace(cmdVersion.stdout) {
		t.Fatalf("version output mismatch: --version=%q version=%q", flagVersion.stdout, cmdVersion.stdout)
	}
}

func TestUpdateCheckFailsWhenBrewMissing(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()

	res := runCLI(t, bin, projectRoot(t), testEnvWithPath(home, ""), "", "update", "--check")
	if res.code != 1 {
		t.Fatalf("expected exit code 1, got %d (%s)", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, "Homebrew is required for automatic updates") {
		t.Fatalf("unexpected stderr: %s", res.stderr)
	}
}

func TestUpdateCommandsUseBrewCheckAndApplyContracts(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	fakeBin := t.TempDir()

	brewScript := filepath.Join(fakeBin, "brew")
	brewBody := "#!/bin/sh\n" +
		"if [ \"$1\" = \"outdated\" ]; then\n" +
		"  echo '{\"formulae\":[{\"name\":\"flutree\",\"installed_versions\":[\"0.7.0\"],\"current_version\":\"0.8.0\"}]}'\n" +
		"  exit 0\n" +
		"fi\n" +
		"if [ \"$1\" = \"update\" ]; then\n" +
		"  echo 'updated'\n" +
		"  exit 0\n" +
		"fi\n" +
		"if [ \"$1\" = \"upgrade\" ]; then\n" +
		"  echo 'upgraded flutree'\n" +
		"  exit 0\n" +
		"fi\n" +
		"if [ \"$1\" = \"list\" ]; then\n" +
		"  echo 'flutree 0.7.0'\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 0\n"
	if err := os.WriteFile(brewScript, []byte(brewBody), 0o755); err != nil {
		t.Fatalf("failed to write fake brew script: %v", err)
	}

	env := testEnvWithPath(home, fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	check := runCLI(t, bin, projectRoot(t), env, "", "update", "--check")
	if check.code != 0 {
		t.Fatalf("expected update --check success, got %d (%s)", check.code, check.stderr)
	}
	if !strings.Contains(check.stdout, "mode=check") || !strings.Contains(check.stdout, "outdated=true") {
		t.Fatalf("unexpected check output: %s", check.stdout)
	}

	apply := runCLI(t, bin, projectRoot(t), env, "", "update")
	if apply.code != 0 {
		t.Fatalf("expected update apply success, got %d (%s)", apply.code, apply.stderr)
	}
	if !strings.Contains(apply.stdout, "mode=apply") || !strings.Contains(apply.stdout, "outdated=true") {
		t.Fatalf("unexpected apply output: %s", apply.stdout)
	}
}

func TestVersionCommandAndFlagReturnStableOutput(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	env := testEnv(home)

	byCommand := runCLI(t, bin, projectRoot(t), env, "", "version")
	if byCommand.code != 0 {
		t.Fatalf("expected 0, got %d (%s)", byCommand.code, byCommand.stderr)
	}
	byFlag := runCLI(t, bin, projectRoot(t), env, "", "--version")
	if byFlag.code != 0 {
		t.Fatalf("expected 0, got %d (%s)", byFlag.code, byFlag.stderr)
	}
	if strings.TrimSpace(byCommand.stdout) != strings.TrimSpace(byFlag.stdout) {
		t.Fatalf("version outputs mismatch. version=%q flag=%q", byCommand.stdout, byFlag.stdout)
	}
}

func TestUpdateCheckAndApplyContractsWithBrewScript(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	brewBin := t.TempDir()
	writeFakeBrew(t, brewBin, `#!/bin/sh
set -eu
cmd="$1"
shift
case "$cmd" in
  outdated)
    if [ "${BREW_SCENARIO:-up_to_date}" = "outdated" ]; then
      printf '{"formulae":[{"name":"flutree","installed_versions":["1.0.0"],"current_version":"1.0.0","version":"1.1.0"}]}'
    else
      printf '{}'
    fi
    ;;
  list)
    printf 'flutree 1.0.0\n'
    ;;
  update)
    printf 'updated\n'
    ;;
  upgrade)
    printf 'upgraded flutree\n'
    ;;
  *)
    exit 1
    ;;
esac
`)
	env := withPath(testEnv(home), brewBin)

	check := runCLI(t, bin, projectRoot(t), env, "", "update", "--check")
	if check.code != 0 {
		t.Fatalf("expected 0, got %d (%s)", check.code, check.stderr)
	}
	if !strings.Contains(check.stdout, "mode=check outdated=false") {
		t.Fatalf("unexpected check output: %s", check.stdout)
	}

	outdatedEnv := append(env, "BREW_SCENARIO=outdated")
	apply := runCLI(t, bin, projectRoot(t), outdatedEnv, "", "update")
	if apply.code != 0 {
		t.Fatalf("expected 0, got %d (%s)", apply.code, apply.stderr)
	}
	if !strings.Contains(apply.stdout, "mode=apply outdated=true") {
		t.Fatalf("unexpected apply output: %s", apply.stdout)
	}
}

func TestUpdateFailsWhenBrewUnavailable(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	env := withPath(testEnv(home), t.TempDir())

	res := runCLI(t, bin, projectRoot(t), env, "", "update", "--check")
	if res.code != 1 {
		t.Fatalf("expected 1, got %d (%s)", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, "Homebrew is required for automatic updates") {
		t.Fatalf("unexpected stderr: %s", res.stderr)
	}
}

func TestVersionJSONFlag(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()

	res := runCLI(t, bin, projectRoot(t), testEnv(home), "", "version", "--json")
	if res.code != 0 {
		t.Fatalf("expected 0, got %d (%s)", res.code, res.stderr)
	}

	var out map[string]string
	if err := json.Unmarshal([]byte(res.stdout), &out); err != nil {
		t.Fatalf("invalid JSON output: %s\nerr: %v", res.stdout, err)
	}
	if _, ok := out["version"]; !ok {
		t.Fatalf("expected 'version' key in JSON output: %s", res.stdout)
	}
}

func TestListJSONFlag(t *testing.T) {
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

	res := runCLI(t, bin, outside, testEnv(home), "", "list", "--json")
	if res.code != 0 {
		t.Fatalf("expected 0, got %d (%s)", res.code, res.stderr)
	}

	// Should be a JSON array
	var rows []map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &rows); err != nil {
		t.Fatalf("invalid JSON output: %s\nerr: %v", res.stdout, err)
	}
	if len(rows) == 0 {
		t.Fatalf("expected at least one row in JSON output: %s", res.stdout)
	}
	if _, ok := rows[0]["name"]; !ok {
		t.Fatalf("expected 'name' key in first row: %s", res.stdout)
	}
}

func TestConfigSetJSONFlag(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := t.TempDir()

	res := runCLI(t, bin, projectRoot(t), testEnv(home), "", "config", "set", "scope.root", scope, "--json")
	if res.code != 0 {
		t.Fatalf("expected 0, got %d (%s)", res.code, res.stderr)
	}

	var out map[string]string
	if err := json.Unmarshal([]byte(res.stdout), &out); err != nil {
		t.Fatalf("invalid JSON output: %s\nerr: %v", res.stdout, err)
	}
	if _, ok := out["key"]; !ok {
		t.Fatalf("expected 'key' key in JSON output: %s", res.stdout)
	}
	if _, ok := out["value"]; !ok {
		t.Fatalf("expected 'value' key in JSON output: %s", res.stdout)
	}
}

func TestConfigGetJSONFlag(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	scope := t.TempDir()

	// First set a value
	setRes := runCLI(t, bin, projectRoot(t), testEnv(home), "", "config", "set", "scope.root", scope)
	if setRes.code != 0 {
		t.Fatalf("config set failed: %d (%s)", setRes.code, setRes.stderr)
	}

	// Now get with --json
	res := runCLI(t, bin, projectRoot(t), testEnv(home), "", "config", "get", "scope.root", "--json")
	if res.code != 0 {
		t.Fatalf("expected 0, got %d (%s)", res.code, res.stderr)
	}

	var out map[string]string
	if err := json.Unmarshal([]byte(res.stdout), &out); err != nil {
		t.Fatalf("invalid JSON output: %s\nerr: %v", res.stdout, err)
	}
	if out["key"] != "scope.root" {
		t.Fatalf("expected key 'scope.root', got: %s", out["key"])
	}
	if out["value"] == "" {
		t.Fatalf("expected non-empty value in JSON output: %s", res.stdout)
	}
}

func TestErrorJSONOutput(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()

	// Create a valid git repo to run create inside
	scope := t.TempDir()
	repo := filepath.Join(scope, "root-app")
	initRepo(t, repo)

	// Test that errors output as JSON when --json is set
	// Use --non-interactive with missing --root-repo to trigger a validation error
	res := runCLI(t, bin, repo, testEnv(home), "", "create", "feature-test", "--json", "--non-interactive")
	if res.code != 2 {
		t.Fatalf("expected exit code 2, got %d (%s)", res.code, res.stderr)
	}

	// The error should be JSON formatted since --json was passed
	// CombinedOutput merges stdout and stderr, but the JSON error should be in stderr
	var errOut map[string]interface{}
	if err := json.Unmarshal([]byte(res.stderr), &errOut); err != nil {
		t.Fatalf("expected JSON error output, got stderr: %s, stdout: %s", res.stderr, res.stdout)
	}
	if _, ok := errOut["error"]; !ok {
		t.Fatalf("expected 'error' key in JSON output: %s", res.stderr)
	}
}

// runMCPServeResult holds the output of a runMCPServe call.
type runMCPServeResult struct {
	stdout string
	stderr string
	code   int
}

// runMCPServe starts "flutree mcp serve", writes jsonrpcInput to its stdin,
// closes stdin, and returns whatever the process wrote to stdout and stderr.
// It does NOT use runCLI because that helper combines stdout+stderr and we need
// stdout alone to parse JSON-RPC frames.
//
// A 10-second timeout is applied via context so a hung server never blocks the
// suite indefinitely.
func runMCPServe(t *testing.T, bin, cwd string, env []string, jsonrpcInput string) runMCPServeResult {
	t.Helper()
	const timeout = 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "mcp", "serve")
	cmd.Dir = cwd
	cmd.Env = env
	cmd.Stdin = strings.NewReader(jsonrpcInput)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("runMCPServe: process exceeded %s deadline (hung server); partial stdout=%q stderr=%q", timeout, stdoutBuf.String(), stderrBuf.String())
	}
	code := 0
	if err != nil {
		code = 1
		if ee, ok := err.(*exec.ExitError); ok && ee.ProcessState != nil {
			code = ee.ProcessState.ExitCode()
		}
	}
	return runMCPServeResult{
		stdout: stdoutBuf.String(),
		stderr: stderrBuf.String(),
		code:   code,
	}
}

// TestMCPServeRespondsToInitialize verifies that "flutree mcp serve" performs
// the MCP handshake and writes a valid JSON-RPC initialize response to stdout.
func TestMCPServeRespondsToInitialize(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping MCP serve integration test in short mode")
	}
	bin := buildCLI(t)
	home := t.TempDir()

	// Minimal JSON-RPC 2.0 initialize request followed by a newline to end stdin.
	initRequest := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0.0.1"}}}` + "\n"

	res := runMCPServe(t, bin, projectRoot(t), testEnv(home), initRequest)

	// The process may exit with code 0 or 1 (EOF on stdin causes graceful exit);
	// what matters is that stdout contains a valid JSON-RPC response.
	if res.stdout == "" {
		t.Fatalf("mcp serve wrote nothing to stdout; stderr=%q", res.stderr)
	}

	// Parse the first line as JSON-RPC response.
	firstLine := strings.SplitN(strings.TrimSpace(res.stdout), "\n", 2)[0]
	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(firstLine), &resp); err != nil {
		t.Fatalf("mcp serve stdout is not valid JSON on first line: %s\nerr: %v", res.stdout, err)
	}

	// Must be a JSON-RPC response with id:1.
	if resp["jsonrpc"] != "2.0" {
		t.Fatalf("expected jsonrpc:2.0, got: %v", resp["jsonrpc"])
	}
	if resp["id"] == nil {
		t.Fatalf("expected id field in response, got: %v", resp)
	}
}

// TestMCPServeToolsListContainsAll9Tools verifies that a tools/list request
// returns exactly the 9 advertised tool names.
func TestMCPServeToolsListContainsAll9Tools(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping MCP serve integration test in short mode")
	}
	bin := buildCLI(t)
	home := t.TempDir()

	// Send initialize then tools/list. Each request on its own line (newline-delimited JSON-RPC).
	requests := "" +
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0.0.1"}}}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}` + "\n"

	res := runMCPServe(t, bin, projectRoot(t), testEnv(home), requests)

	if res.stdout == "" {
		t.Fatalf("mcp serve wrote nothing to stdout; stderr=%q", res.stderr)
	}

	// Find the tools/list response (id == 2) among all lines.
	var toolsResp map[string]interface{}
	for _, line := range strings.Split(strings.TrimSpace(res.stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var frame map[string]interface{}
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			continue
		}
		// JSON-RPC numbers unmarshal as float64.
		if id, ok := frame["id"].(float64); ok && id == 2 {
			toolsResp = frame
			break
		}
	}
	if toolsResp == nil {
		t.Fatalf("no response with id==2 found in stdout:\n%s", res.stdout)
	}

	result, ok := toolsResp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected result object in tools/list response, got: %v", toolsResp)
	}
	tools, ok := result["tools"].([]interface{})
	if !ok {
		t.Fatalf("expected tools array in result, got: %v", result)
	}

	wantTools := []string{
		"list_worktrees", "create_worktree", "add_repo",
		"complete_worktree", "pubget", "clean_worktree",
		"get_config", "set_config", "get_version",
	}
	if len(tools) != len(wantTools) {
		t.Fatalf("expected %d tools, got %d: %v", len(wantTools), len(tools), tools)
	}

	got := make(map[string]bool)
	for _, tool := range tools {
		if m, ok := tool.(map[string]interface{}); ok {
			if name, ok := m["name"].(string); ok {
				got[name] = true
			}
		}
	}
	for _, name := range wantTools {
		if !got[name] {
			t.Fatalf("expected tool %q in tools/list response; got tools: %v", name, got)
		}
	}
}

// TestMCPServeWritesNothingExtraToStdout verifies that stdout contains only
// valid JSON-RPC frames — no startup banners, log lines, or other output.
func TestMCPServeWritesNothingExtraToStdout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping MCP serve integration test in short mode")
	}
	bin := buildCLI(t)
	home := t.TempDir()

	initRequest := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0.0.1"}}}` + "\n"

	res := runMCPServe(t, bin, projectRoot(t), testEnv(home), initRequest)

	// Every non-empty line on stdout MUST be valid JSON.
	for i, line := range strings.Split(res.stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var v interface{}
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Fatalf("line %d of stdout is not valid JSON (no non-JSON output allowed): %q\nerr: %v", i+1, line, err)
		}
	}
}

// TestMCPServeUnknownSubcommandReturnsError verifies that "flutree mcp unknown"
// exits with a non-zero code and an error message that names the bad token.
func TestMCPServeUnknownSubcommandReturnsError(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()

	const badSub = "unknown"
	res := runCLI(t, bin, projectRoot(t), testEnv(home), "", "mcp", badSub)
	if res.code == 0 {
		t.Fatalf("expected non-zero exit for unknown mcp subcommand, got 0; stdout=%s", res.stdout)
	}
	combined := res.stdout + res.stderr
	if !strings.Contains(combined, badSub) {
		t.Fatalf("expected error output to mention the unknown subcommand %q; got: %s", badSub, combined)
	}
}

// TestMCPServeRejectsJsonFlagSuffix verifies that "flutree mcp serve --json"
// exits with a non-zero code and a clear rejection message.
// stdout must stay clean (no non-JSON lines) so JSON-RPC clients are not confused.
func TestMCPServeRejectsJsonFlagSuffix(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()

	res := runCLI(t, bin, projectRoot(t), testEnv(home), "", "mcp", "serve", "--json")
	if res.code == 0 {
		t.Fatalf("expected non-zero exit for 'mcp serve --json', got 0; stdout=%s stderr=%s", res.stdout, res.stderr)
	}
	combined := res.stdout + res.stderr
	if !strings.Contains(combined, "--json") {
		t.Fatalf("expected rejection message to mention --json; got: %s", combined)
	}
}

// TestMCPServeRejectsJsonFlagPrefix verifies that "flutree --json mcp serve"
// also exits with a non-zero code and a rejection message.
// The global --json flag is meaningful for other commands but meaningless and
// disruptive for mcp serve (whose stdout is JSON-RPC only).
func TestMCPServeRejectsJsonFlagPrefix(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()

	res := runCLI(t, bin, projectRoot(t), testEnv(home), "", "--json", "mcp", "serve")
	if res.code == 0 {
		t.Fatalf("expected non-zero exit for '--json mcp serve', got 0; stdout=%s stderr=%s", res.stdout, res.stderr)
	}
	combined := res.stdout + res.stderr
	if !strings.Contains(combined, "--json") {
		t.Fatalf("expected rejection message to mention --json; got: %s", combined)
	}
}

// --------------------------------------------------------------------------
// mcp install contract tests
// --------------------------------------------------------------------------

// TestMCPInstallInvalidClientExitsNonZero verifies that --client with an
// unknown value causes a non-zero exit and an error message. Because the
// installer validates the client filter before any I/O, no config files are
// written to disk. The test asserts this by confirming the config paths that
// the three bundled mergers would touch are absent from the HOME directory.
func TestMCPInstallInvalidClientExitsNonZero(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()

	res := runCLI(t, bin, projectRoot(t), testEnv(home), "", "mcp", "install", "--client", "foobar")
	if res.code == 0 {
		t.Fatalf("expected non-zero exit for unknown client, got 0; stdout=%s", res.stdout)
	}
	combined := res.stderr + res.stdout
	if !strings.Contains(combined, "foobar") {
		t.Fatalf("expected error message to mention the unknown client name; got: %s", combined)
	}

	// Verify that no config files were written (validateFilter fires before any I/O).
	noWritePaths := []string{
		filepath.Join(home, ".claude.json"),
		filepath.Join(home, ".opencode.json"),
		filepath.Join(home, ".config", "opencode", "config.json"),
		filepath.Join(home, ".config", "codex", "config.toml"),
	}
	for _, p := range noWritePaths {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("expected no file written at %s, but it exists after invalid-client rejection", p)
		}
	}
}

// TestMCPInstallNoClientsDetectedExitsZero verifies that when no known AI
// coding clients are present, the command exits 0 (no error — nothing to do).
func TestMCPInstallNoClientsDetectedExitsZero(t *testing.T) {
	bin := buildCLI(t)
	// Use a completely empty home so neither binary lookup nor config dirs can
	// succeed. Provide a minimal PATH that contains only a shell (for exec.LookPath
	// to work), but does not contain claude/opencode/codex.
	home := t.TempDir()

	res := runCLI(t, bin, projectRoot(t), testEnv(home), "", "mcp", "install")
	if res.code != 0 {
		t.Fatalf("expected exit 0 when no clients detected, got %d; stderr=%s stdout=%s", res.code, res.stderr, res.stdout)
	}
}

// TestMCPInstallClaudeCreatesConfigEntry verifies that when ~/.claude.json does
// not exist but detection passes via the --client flag, the installer creates
// the file with the correct entry shape.
func TestMCPInstallClaudeCreatesConfigEntry(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()

	// Pre-create ~/.claude.json as an empty object so detection succeeds
	// (config file exists path) without needing the binary on PATH.
	claudeJSON := filepath.Join(home, ".claude.json")
	if err := os.WriteFile(claudeJSON, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}

	res := runCLI(t, bin, projectRoot(t), testEnv(home), "", "mcp", "install", "--client", "claude-code")
	if res.code != 0 {
		t.Fatalf("expected exit 0 for claude-code install, got %d; stderr=%s stdout=%s", res.code, res.stderr, res.stdout)
	}

	// Read back the config and verify entry shape.
	content, err := os.ReadFile(claudeJSON)
	if err != nil {
		t.Fatalf("failed to read ~/.claude.json after install: %v", err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(content, &cfg); err != nil {
		t.Fatalf("~/.claude.json is not valid JSON after install: %v\ncontent: %s", err, content)
	}
	mcpServers, ok := cfg["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected mcpServers object in ~/.claude.json, got: %v", cfg)
	}
	flutree, ok := mcpServers["flutree"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected mcpServers.flutree object, got: %v", mcpServers)
	}
	if flutree["type"] != "stdio" {
		t.Fatalf("expected type=stdio, got: %v", flutree["type"])
	}
	args, ok := flutree["args"].([]interface{})
	if !ok || len(args) != 2 || args[0] != "mcp" || args[1] != "serve" {
		t.Fatalf("expected args=[mcp serve], got: %v", flutree["args"])
	}
}

// TestMCPInstallAlreadyExistsSkips verifies that when the entry already exists
// and --force is NOT set, the file is not modified and the result is already_exists.
func TestMCPInstallAlreadyExistsSkips(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()

	// Pre-create ~/.claude.json with the flutree entry already present.
	claudeJSON := filepath.Join(home, ".claude.json")
	initial := `{"mcpServers":{"flutree":{"type":"stdio","command":"/old/path/flutree","args":["mcp","serve"]}}}`
	if err := os.WriteFile(claudeJSON, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}

	res := runCLI(t, bin, projectRoot(t), testEnv(home), "", "mcp", "install", "--client", "claude-code")
	if res.code != 0 {
		t.Fatalf("expected exit 0 for already_exists, got %d; stderr=%s stdout=%s", res.code, res.stderr, res.stdout)
	}

	// File content must be unchanged.
	after, err := os.ReadFile(claudeJSON)
	if err != nil {
		t.Fatalf("failed to read ~/.claude.json: %v", err)
	}
	if string(after) != initial {
		t.Fatalf("expected file unchanged on already_exists, but it was modified:\nbefore: %s\nafter: %s", initial, string(after))
	}

	combined := res.stdout + res.stderr
	if !strings.Contains(combined, "already_exists") && !strings.Contains(combined, "SKIP") {
		t.Fatalf("expected already_exists or SKIP in output, got: %s", combined)
	}
}

// TestMCPInstallForceOverwrites verifies that --force overwrites an existing entry.
func TestMCPInstallForceOverwrites(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()

	claudeJSON := filepath.Join(home, ".claude.json")
	initial := `{"mcpServers":{"flutree":{"type":"stdio","command":"/old/path/flutree","args":["mcp","serve"]}}}`
	if err := os.WriteFile(claudeJSON, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}

	res := runCLI(t, bin, projectRoot(t), testEnv(home), "", "mcp", "install", "--client", "claude-code", "--force")
	if res.code != 0 {
		t.Fatalf("expected exit 0 for force install, got %d; stderr=%s stdout=%s", res.code, res.stderr, res.stdout)
	}

	after, err := os.ReadFile(claudeJSON)
	if err != nil {
		t.Fatalf("failed to read ~/.claude.json: %v", err)
	}
	// The entry must have been updated (command should NOT still be /old/path/flutree).
	var cfg map[string]interface{}
	if err := json.Unmarshal(after, &cfg); err != nil {
		t.Fatalf("~/.claude.json is not valid JSON after --force: %v", err)
	}
	mcpServers := cfg["mcpServers"].(map[string]interface{})
	flutree := mcpServers["flutree"].(map[string]interface{})
	if flutree["command"] == "/old/path/flutree" {
		t.Fatalf("expected command to be updated after --force, still has old value")
	}
}

// TestMCPInstallJSONOutput verifies the --json flag produces a JSON object
// keyed by client name with the expected per-client result shape.
func TestMCPInstallJSONOutput(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()

	// Pre-create ~/.claude.json so claude-code is detected.
	claudeJSON := filepath.Join(home, ".claude.json")
	if err := os.WriteFile(claudeJSON, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}

	res := runCLI(t, bin, projectRoot(t), testEnv(home), "", "mcp", "install", "--client", "claude-code", "--json")
	if res.code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s stdout=%s", res.code, res.stderr, res.stdout)
	}

	var out map[string]interface{}
	if err := json.Unmarshal([]byte(res.stdout), &out); err != nil {
		t.Fatalf("expected JSON object from --json flag, got: %s\nerr: %v", res.stdout, err)
	}
	claudeResult, ok := out["claude-code"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected claude-code key in JSON output, got keys: %v", out)
	}
	if claudeResult["status"] == nil {
		t.Fatalf("expected status field in per-client result, got: %v", claudeResult)
	}
	if claudeResult["client"] == nil {
		t.Fatalf("expected client field in per-client result, got: %v", claudeResult)
	}
}

// TestMCPInstallPreservesExistingKeys verifies that the non-destructive merge
// invariant holds: keys other than mcpServers.flutree are unchanged.
func TestMCPInstallPreservesExistingKeys(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()

	claudeJSON := filepath.Join(home, ".claude.json")
	initial := `{"projects":{"foo":"bar"},"auth":{"token":"secret"},"mcpServers":{"other":{"type":"stdio","command":"other-cmd","args":[]}}}`
	if err := os.WriteFile(claudeJSON, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}

	res := runCLI(t, bin, projectRoot(t), testEnv(home), "", "mcp", "install", "--client", "claude-code")
	if res.code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%s stdout=%s", res.code, res.stderr, res.stdout)
	}

	after, err := os.ReadFile(claudeJSON)
	if err != nil {
		t.Fatalf("failed to read ~/.claude.json: %v", err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(after, &cfg); err != nil {
		t.Fatalf("~/.claude.json is not valid JSON: %v", err)
	}

	// projects and auth keys must be preserved.
	if projects, ok := cfg["projects"].(map[string]interface{}); !ok || projects["foo"] != "bar" {
		t.Fatalf("expected projects.foo=bar to be preserved, got: %v", cfg["projects"])
	}
	if auth, ok := cfg["auth"].(map[string]interface{}); !ok || auth["token"] != "secret" {
		t.Fatalf("expected auth.token=secret to be preserved, got: %v", cfg["auth"])
	}

	// The other MCP server entry must still be there.
	mcpServers, ok := cfg["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected mcpServers to be present: %v", cfg)
	}
	if _, ok := mcpServers["other"]; !ok {
		t.Fatalf("expected mcpServers.other to be preserved: %v", mcpServers)
	}
	// And the new flutree entry must be added.
	if _, ok := mcpServers["flutree"]; !ok {
		t.Fatalf("expected mcpServers.flutree to be added: %v", mcpServers)
	}
}
