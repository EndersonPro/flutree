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
	if !strings.Contains(res.stdout, "create") || !strings.Contains(res.stdout, "list") || !strings.Contains(res.stdout, "complete") {
		t.Fatalf("unexpected help output: %s", res.stdout)
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
			name:     "create long help",
			args:     []string{"create", "--help"},
			contains: []string{"flutree create <name> [options]", "--branch", "--root-repo", "--package", "--package-base", "--copy-root-file"},
		},
		{
			name:     "create short help",
			args:     []string{"create", "-h"},
			contains: []string{"flutree create <name> [options]", "--branch", "--root-repo", "--package"},
		},
		{
			name:     "add-repo help",
			args:     []string{"add-repo", "--help"},
			contains: []string{"flutree add-repo <workspace> [options]", "--repo", "--package-base", "--copy-root-file"},
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
			name:     "list help",
			args:     []string{"list", "--help"},
			contains: []string{"flutree list [options]", "--all"},
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
