//go:build !windows

package integration_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// cliBuild holds the single flutree binary shared by the e2e suite.
var cliBuild struct {
	once sync.Once
	dir  string
	err  error
	out  string
}

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "flutree-e2e-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create e2e build dir:", err)
		os.Exit(1)
	}
	cliBuild.dir = dir
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// e2eResult keeps stdout and stderr apart so tests can assert stream discipline
// (for example, JSON on stdout and nothing else).
type e2eResult struct {
	code   int
	stdout string
	stderr string
}

func runE2E(t *testing.T, cwd string, env []string, args ...string) e2eResult {
	t.Helper()
	cmd := exec.Command(buildCLI(t), args...)
	cmd.Dir = cwd
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run flutree %v: %v", args, err)
		}
		code = exitErr.ExitCode()
	}
	return e2eResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func (r e2eResult) requireCode(t *testing.T, want int) {
	t.Helper()
	if r.code != want {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", r.code, want, r.stdout, r.stderr)
	}
}

// fakeToolchain installs fake `flutter` and `dart` executables that append one
// line per invocation ("<tool> <args> @ <cwd>") to a log file. Set exitCode to
// make every invocation fail.
type fakeToolchain struct {
	binDir string
	log    string
}

func newFakeToolchain(t *testing.T, exitCode int) fakeToolchain {
	t.Helper()
	dir := t.TempDir()
	tc := fakeToolchain{binDir: filepath.Join(dir, "bin"), log: filepath.Join(dir, "calls.log")}
	if err := os.MkdirAll(tc.binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"flutter", "dart"} {
		script := fmt.Sprintf("#!/bin/sh\necho \"%s $* @ $(pwd -P)\" >> %q\n", tool, tc.log)
		if exitCode != 0 {
			script += fmt.Sprintf("echo \"%s failed\" >&2\nexit %d\n", tool, exitCode)
		}
		if err := os.WriteFile(filepath.Join(tc.binDir, tool), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return tc
}

// env returns testEnv(home) with the fake toolchain first on PATH.
func (tc fakeToolchain) env(home string) []string {
	return append(testEnv(home), "PATH="+tc.binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func (tc fakeToolchain) calls(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(tc.log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

// e2eWorkspace is a managed flutree workspace created through the real CLI.
type e2eWorkspace struct {
	home     string
	scope    string
	name     string
	rootRepo string // source repository
	rootWT   string // root worktree path
	pkgWTs   map[string]string
}

// createWorkspace builds a scope with a root repo plus the given package repos
// and runs `flutree create` non-interactively. pubspecBody is written to every
// repo's pubspec.yaml before the first commit when non-empty.
func createWorkspace(t *testing.T, name string, packages []string, pubspecBody string) e2eWorkspace {
	t.Helper()
	ws := e2eWorkspace{home: t.TempDir(), scope: filepath.Join(t.TempDir(), "workspace"), name: name, pkgWTs: map[string]string{}}
	ws.rootRepo = filepath.Join(ws.scope, "root-app")
	initRepoWithPubspec(t, ws.rootRepo, "root_app", pubspecBody)
	args := []string{"create", name, "--scope", ws.scope, "--root-repo", "root-app", "--yes", "--non-interactive"}
	if len(packages) == 0 {
		args = append(args, "--no-package")
	}
	for _, pkg := range packages {
		initRepoWithPubspec(t, filepath.Join(ws.scope, pkg), strings.ReplaceAll(pkg, "-", "_"), pubspecBody)
		args = append(args, "--package", pkg, "--package-base", pkg+"=main")
	}
	runE2E(t, ws.rootRepo, testEnv(ws.home), args...).requireCode(t, 0)

	base := filepath.Join(ws.home, "Documents", "worktrees", name)
	ws.rootWT = filepath.Join(base, "root", "root-app")
	for _, pkg := range packages {
		ws.pkgWTs[pkg] = filepath.Join(base, "packages", pkg)
	}
	return ws
}

func initRepoWithPubspec(t *testing.T, path, packageName, pubspecBody string) {
	t.Helper()
	initRepoWithPackageName(t, path, packageName)
	if pubspecBody == "" {
		return
	}
	content := "name: " + packageName + "\n" + pubspecBody
	if err := os.WriteFile(filepath.Join(path, "pubspec.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, path, "commit", "-am", "pubspec")
	runGit(t, path, "push", "origin", "main")
}

func realPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
