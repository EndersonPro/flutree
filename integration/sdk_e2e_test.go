//go:build !windows

package integration_test

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// sdkScope builds a scope with root-app and core-pkg. rootFiles/pkgFiles are
// committed into each repo; rootLocal is written to root-app without
// committing it (like puro's gitignored .puro.json).
func sdkScope(t *testing.T, rootFiles, pkgFiles, rootLocal map[string]string) string {
	t.Helper()
	scope := filepath.Join(t.TempDir(), "workspace")
	for repo, files := range map[string]map[string]string{"root-app": rootFiles, "core-pkg": pkgFiles} {
		dir := filepath.Join(scope, repo)
		initRepoWithPackageName(t, dir, strings.ReplaceAll(repo, "-", "_"))
		if len(files) == 0 {
			continue
		}
		for name, content := range files {
			writeFile(t, filepath.Join(dir, name), content)
		}
		runGit(t, dir, "add", ".")
		runGit(t, dir, "commit", "-m", "sdk pin")
		runGit(t, dir, "push", "origin", "main")
	}
	for name, content := range rootLocal {
		writeFile(t, filepath.Join(scope, "root-app", name), content)
	}
	return scope
}

func createWithPackage(t *testing.T, home, scope string, env []string, extra ...string) e2eResult {
	t.Helper()
	args := append([]string{"create", "feature-sdk", "--scope", scope, "--root-repo", "root-app",
		"--package", "core-pkg", "--package-base", "core-pkg=main", "--yes", "--non-interactive"}, extra...)
	return runE2E(t, filepath.Join(scope, "root-app"), env, args...)
}

func sdkCalls(t *testing.T, tc fakeToolchain) []string {
	t.Helper()
	var out []string
	for _, call := range tc.calls(t) {
		if strings.HasPrefix(call, "fvm ") || strings.HasPrefix(call, "puro ") {
			out = append(out, call)
		}
	}
	sort.Strings(out)
	return out
}

func sdkWorktrees(t *testing.T, home string) (string, string) {
	base := filepath.Join(home, "Documents", "worktrees", "feature-sdk")
	return realPath(t, filepath.Join(base, "root", "root-app")), realPath(t, filepath.Join(base, "packages", "core-pkg"))
}

func TestE2ECreateRunsFvmUseInEveryWorktree(t *testing.T) {
	home := t.TempDir()
	scope := sdkScope(t,
		map[string]string{".fvmrc": `{"flutter": "3.24.0"}`},
		map[string]string{".fvm/fvm_config.json": `{"flutterSdkVersion": "3.22.1"}`}, nil)
	tc := newFakeToolchain(t, 0)

	createWithPackage(t, home, scope, tc.env(home)).requireCode(t, 0)

	rootWT, pkgWT := sdkWorktrees(t, home)
	want := []string{
		"fvm use 3.22.1 --force --skip-pub-get @ " + pkgWT,
		"fvm use 3.24.0 --force --skip-pub-get @ " + rootWT,
	}
	sort.Strings(want)
	if got := sdkCalls(t, tc); !equalLines(got, want) {
		t.Fatalf("sdk calls = %q, want %q", got, want)
	}
}

func TestE2ECreateRunsPuroUseFromGitignoredPin(t *testing.T) {
	home := t.TempDir()
	scope := sdkScope(t, nil, nil, map[string]string{".puro.json": `{"env": "stable"}`})
	tc := newFakeToolchain(t, 0)

	createWithPackage(t, home, scope, tc.env(home)).requireCode(t, 0)

	rootWT, _ := sdkWorktrees(t, home)
	want := []string{"puro use stable @ " + rootWT}
	if got := sdkCalls(t, tc); !equalLines(got, want) {
		t.Fatalf("sdk calls = %q, want %q (package has no pin)", got, want)
	}
}

func TestE2ESDKManagerPreference(t *testing.T) {
	pins := map[string]string{".fvmrc": `{"flutter": "3.24.0"}`}
	local := map[string]string{".puro.json": `{"env": "beta"}`}
	cases := []struct {
		manager string
		want    string // expected single root call prefix, "" for none
	}{
		{"auto", "fvm use 3.24.0 --force --skip-pub-get"},
		{"fvm", "fvm use 3.24.0 --force --skip-pub-get"},
		{"puro", "puro use beta"},
		{"none", ""},
	}
	for _, c := range cases {
		t.Run(c.manager, func(t *testing.T) {
			home := t.TempDir()
			scope := sdkScope(t, pins, nil, local)
			tc := newFakeToolchain(t, 0)
			runE2E(t, home, tc.env(home), "config", "set", "sdk.manager", c.manager).requireCode(t, 0)

			createWithPackage(t, home, scope, tc.env(home)).requireCode(t, 0)

			got := sdkCalls(t, tc)
			if c.want == "" {
				if len(got) != 0 {
					t.Fatalf("sdk.manager none must not run a manager, got %q", got)
				}
				return
			}
			rootWT, _ := sdkWorktrees(t, home)
			if !equalLines(got, []string{c.want + " @ " + rootWT}) {
				t.Fatalf("sdk calls = %q, want %q", got, c.want+" @ "+rootWT)
			}
		})
	}
}

func TestE2ESDKSetupFailureKeepsWorkspaceAndReportsIt(t *testing.T) {
	home := t.TempDir()
	scope := sdkScope(t, map[string]string{".fvmrc": `{"flutter": "3.24.0"}`}, nil, nil)
	tc := newFakeToolchain(t, 4)

	res := createWithPackage(t, home, scope, tc.env(home), "--json")
	res.requireCode(t, 0)

	var out struct {
		SDKSetup []struct {
			Repo    string `json:"repo"`
			Manager string `json:"manager"`
			Version string `json:"version"`
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"sdk_setup"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &out); err != nil {
		t.Fatalf("stdout is not a single JSON document: %v\n%s", err, res.stdout)
	}
	if len(out.SDKSetup) != 1 || out.SDKSetup[0].Repo != "root-app" || out.SDKSetup[0].Manager != "fvm" ||
		out.SDKSetup[0].Version != "3.24.0" || out.SDKSetup[0].Status != "failed" || !strings.Contains(out.SDKSetup[0].Message, "fvm failed") {
		t.Fatalf("unexpected sdk_setup: %+v", out.SDKSetup)
	}
	rootWT, _ := sdkWorktrees(t, home)
	if !exists(rootWT) {
		t.Fatalf("worktree must be kept when SDK setup fails")
	}
}

func TestE2EAddRepoRunsSDKSetupForAttachedPackage(t *testing.T) {
	ws := createWorkspace(t, "feature-sdk", nil, "")
	pkg := filepath.Join(ws.scope, "core-pkg")
	initRepoWithPackageName(t, pkg, "core")
	writeFile(t, filepath.Join(pkg, ".fvmrc"), `{"flutter": "3.19.6"}`)
	runGit(t, pkg, "add", ".")
	runGit(t, pkg, "commit", "-m", "pin")
	runGit(t, pkg, "push", "origin", "main")
	tc := newFakeToolchain(t, 0)

	runE2E(t, ws.rootRepo, tc.env(ws.home), "add-repo", ws.name, "--scope", ws.scope, "--repo", "core-pkg",
		"--package-base", "core-pkg=main", "--non-interactive").requireCode(t, 0)

	pkgWT := realPath(t, filepath.Join(ws.home, "Documents", "worktrees", ws.name, "packages", "core-pkg"))
	want := []string{"fvm use 3.19.6 --force --skip-pub-get @ " + pkgWT}
	if got := sdkCalls(t, tc); !equalLines(got, want) {
		t.Fatalf("sdk calls = %q, want %q", got, want)
	}
}

func TestE2EConfigSDKManagerValidation(t *testing.T) {
	home := t.TempDir()
	get := runE2E(t, home, testEnv(home), "config", "get", "sdk.manager")
	get.requireCode(t, 0)
	if strings.TrimSpace(get.stdout) != "auto" {
		t.Fatalf("default sdk.manager = %q, want auto", get.stdout)
	}
	bad := runE2E(t, home, testEnv(home), "config", "set", "sdk.manager", "asdf")
	bad.requireCode(t, 2)
	if !strings.Contains(bad.stderr, "[input] Invalid value for sdk.manager.") {
		t.Fatalf("unexpected stderr: %s", bad.stderr)
	}
}

func TestE2ESDKSetupIsShownInHumanOutput(t *testing.T) {
	home := t.TempDir()
	scope := sdkScope(t, map[string]string{".fvmrc": `{"flutter": "3.24.0"}`}, nil, nil)

	ok := createWithPackage(t, home, scope, newFakeToolchain(t, 0).env(home))
	ok.requireCode(t, 0)
	if !strings.Contains(ok.stdout, "SDK") || !strings.Contains(ok.stdout, "root-app: fvm 3.24.0") {
		t.Fatalf("expected SDK setup line in create output:\n%s", ok.stdout)
	}

	home2 := t.TempDir()
	failed := createWithPackage(t, home2, sdkScope(t, map[string]string{".fvmrc": `{"flutter": "3.24.0"}`}, nil, nil), newFakeToolchain(t, 4).env(home2))
	failed.requireCode(t, 0)
	if !strings.Contains(failed.stderr, "SDK setup failed for root-app") || !strings.Contains(failed.stderr, "fvm use 3.24.0") {
		t.Fatalf("expected an SDK failure warning on stderr:\nstdout:\n%s\nstderr:\n%s", failed.stdout, failed.stderr)
	}
}
