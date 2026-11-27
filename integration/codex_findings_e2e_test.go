//go:build !windows

package integration_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Finding 1: complete_worktree and pubget gate on confirm=true for clients
// that cannot elicit, so their published input schemas must expose it.
func TestCodexFinding1ConfirmIsInToolSchemas(t *testing.T) {
	var tools struct {
		Tools []struct {
			Name        string `json:"name"`
			InputSchema struct {
				Properties map[string]struct {
					Type string `json:"type"`
				} `json:"properties"`
			} `json:"inputSchema"`
		} `json:"tools"`
	}
	raw := mcpRPC(t, t.TempDir(), testEnv(t.TempDir()), "tools/list", map[string]any{})
	if err := json.Unmarshal(raw, &tools); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, tool := range tools.Tools {
		if tool.Name != "complete_worktree" && tool.Name != "pubget" {
			continue
		}
		seen[tool.Name] = true
		if prop, ok := tool.InputSchema.Properties["confirm"]; !ok || prop.Type != "boolean" {
			t.Errorf("%s schema must expose a boolean confirm property, got %+v", tool.Name, tool.InputSchema.Properties)
		}
	}
	if len(seen) != 2 {
		t.Fatalf("tools/list is missing complete_worktree or pubget: %v", seen)
	}
}

// Finding 2: path-selector keys of --package-base resolve to the repository
// they name, like the matching --package selector does.
func TestCodexFinding2PackageBaseKeyAsRelativePath(t *testing.T) {
	g := newGraphScope(t, "codex-rel", graphSDK, map[string]string{"pkg-a": ""})
	pkgA := filepath.Join(g.scope, "pkg-a")
	runGit(t, pkgA, "checkout", "-b", "feat-base")
	writeFile(t, filepath.Join(pkgA, "only-on-feat-base.txt"), "x\n")
	runGit(t, pkgA, "add", ".")
	runGit(t, pkgA, "commit", "-m", "feat base")
	runGit(t, pkgA, "checkout", "main")

	runE2E(t, g.root, testEnv(g.home), "create", g.name, "--scope", g.scope, "--root-repo", "root-app",
		"--package", "../pkg-a", "--package-base", "../pkg-a=feat-base", "--yes", "--non-interactive").requireCode(t, 0)
	if !exists(filepath.Join(g.pkgWT("pkg-a"), "only-on-feat-base.txt")) {
		t.Fatalf("pkg-a must be created from feat-base, not the default branch")
	}
}

func TestCodexFinding2PackageBaseKeyAsHomePath(t *testing.T) {
	g := newGraphScope(t, "codex-home", graphSDK, nil)
	pkgB := filepath.Join(g.home, "code", "pkg-b")
	initRepoWithPubspec(t, pkgB, "pkg_b", "")
	runGit(t, pkgB, "checkout", "-b", "feat-base")
	writeFile(t, filepath.Join(pkgB, "only-on-feat-base.txt"), "x\n")
	runGit(t, pkgB, "add", ".")
	runGit(t, pkgB, "commit", "-m", "feat base")
	runGit(t, pkgB, "checkout", "main")

	runE2E(t, g.root, testEnv(g.home), "create", g.name, "--scope", g.scope, "--root-repo", "root-app",
		"--package", "~/code/pkg-b", "--package-base", "~/code/pkg-b=feat-base", "--yes", "--non-interactive").requireCode(t, 0)
	if !exists(filepath.Join(g.pkgWT("pkg-b"), "only-on-feat-base.txt")) {
		t.Fatalf("pkg-b must be created from feat-base, not the default branch")
	}
}

func TestCodexFinding2AddRepoPackageBaseKeyAsRelativePath(t *testing.T) {
	g := newGraphScope(t, "codex-add", graphSDK, map[string]string{"pkg-a": "", "pkg-c": ""})
	g.create(t, "pkg-a")
	pkgC := filepath.Join(g.scope, "pkg-c")
	runGit(t, pkgC, "checkout", "-b", "feat-base")
	writeFile(t, filepath.Join(pkgC, "only-on-feat-base.txt"), "x\n")
	runGit(t, pkgC, "add", ".")
	runGit(t, pkgC, "commit", "-m", "feat base")
	runGit(t, pkgC, "checkout", "main")

	runE2E(t, g.root, testEnv(g.home), "add-repo", g.name, "--scope", g.scope, "--repo", "../pkg-c",
		"--package-base", "../pkg-c=feat-base", "--non-interactive").requireCode(t, 0)
	if !exists(filepath.Join(g.pkgWT("pkg-c"), "only-on-feat-base.txt")) {
		t.Fatalf("pkg-c must be attached from feat-base, not the default branch")
	}
}

// Finding 3: in the interactive wizard, path selectors outside the scope are
// inspected and selected instead of being dropped.
func TestCodexFinding3WizardSelectsPathSelectorsOutsideScope(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	ws := filepath.Join(t.TempDir(), "ws")
	initRepoWithPackageName(t, filepath.Join(ws, "aaa-app"), "aaa_app")
	initRepoWithPackageName(t, filepath.Join(ws, "bbb-core"), "bbb_core")
	outside := t.TempDir()
	extRoot := filepath.Join(outside, "ext-root")
	extPkg := filepath.Join(outside, "ext-pkg")
	initRepoWithPackageName(t, extRoot, "ext_root")
	initRepoWithPackageName(t, extPkg, "ext_pkg")
	runGit(t, extPkg, "branch", "develop")

	res := runCLIWithPTY(t, bin, ws, testEnv(home), []string{
		"Step 1 - Workspace name::" + keyEnter,
		"Step 2 - Choose root repository::" + keyEnter,
		"Step 3 - Select packages::" + keyEnter,
		"Workspace branch::" + keyEnter,
		"Root base branch::" + keyEnter,
		"Generate .code-workspace file::" + keyEnter,
		"Base branch for package 'ext-pkg'::" + keyEnter,
		"Step 5 - Final review::" + keyEnter,
		"Update local branches from origin::n\n",
	}, "create", "feature-ext", "--branch", "feature/ext", "--scope", ws, "--root-repo", extRoot, "--package", extPkg)
	if res.code != 0 {
		t.Fatalf("expected success, got %d\nstdout: %s\nstderr: %s", res.code, res.stdout, res.stderr)
	}
	base := worktreeBase(t, home)
	for _, p := range []string{filepath.Join(base, "root", "ext-root"), filepath.Join(base, "packages", "ext-pkg")} {
		if !exists(p) {
			t.Fatalf("expected worktree %s", p)
		}
	}
	if exists(filepath.Join(base, "root", "aaa-app")) || exists(filepath.Join(base, "packages", "bbb-core")) {
		t.Fatalf("scope repositories must not be selected instead of the explicit ones")
	}
}

// Finding 4: add-repo refreshes generated graph entries, but a user's own
// edit of pubspec_overrides.yaml keeps winning.
func TestCodexFinding4AddRepoRefreshesGeneratedGraphOverrides(t *testing.T) {
	rootBody := graphSDK + "dependencies:\n" + pathDep("helper", "../repo-b/packages/helper")
	g := newGraphScope(t, "codex-graph", rootBody, map[string]string{"pkg-a": "", "repo-b": "", "repo-c": ""})
	g.create(t, "pkg-a")

	before := readOverrides(t, g.rootWT())
	if target := overridePathTarget(t, g.rootWT(), before["helper"]); target != filepath.Join(g.scope, "repo-b", "packages", "helper") {
		t.Fatalf("helper resolves to %s before attaching repo-b", target)
	}

	runE2E(t, g.root, testEnv(g.home), "add-repo", g.name, "--scope", g.scope, "--repo", "repo-b",
		"--package-base", "repo-b=main", "--non-interactive").requireCode(t, 0)
	after := readOverrides(t, g.rootWT())
	if target := overridePathTarget(t, g.rootWT(), after["helper"]); target != filepath.Join(g.pkgWT("repo-b"), "packages", "helper") {
		t.Errorf("helper resolves to %s, want it inside the repo-b worktree", target)
	}

	// The user redirects helper and adds an entry of their own.
	custom := filepath.Join(t.TempDir(), "my-helper")
	path := filepath.Join(g.rootWT(), "pubspec_overrides.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(data), "  helper:", "  helper:\n    path: "+custom+"\n  helper_old:", 1)
	edited += "  user_dep:\n    path: " + custom + "\n"
	writeFile(t, path, edited)

	runE2E(t, g.root, testEnv(g.home), "add-repo", g.name, "--scope", g.scope, "--repo", "repo-c",
		"--package-base", "repo-c=main", "--non-interactive").requireCode(t, 0)
	final := readOverrides(t, g.rootWT())
	for _, name := range []string{"helper", "user_dep"} {
		if target := overridePathTarget(t, g.rootWT(), final[name]); target != custom {
			t.Errorf("%s resolves to %s, want the user's %s", name, target, custom)
		}
	}
}
