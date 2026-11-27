//go:build !windows

package integration_test

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// pkgScope is a scope with a root app plus package repos that live in
// different folders, so a path dependency between them is not a sibling path.
type pkgScope struct {
	home     string
	scope    string
	rootRepo string
	name     string
}

type pkgRepo struct {
	dir  string // path relative to the scope
	name string // pubspec package name
	body string // pubspec body after the name line
}

func (r pkgRepo) folder() string { return filepath.Base(r.dir) }

func newPkgScope(t *testing.T, name, rootBody string, repos ...pkgRepo) pkgScope {
	t.Helper()
	s := pkgScope{home: t.TempDir(), scope: filepath.Join(t.TempDir(), "workspace"), name: name}
	s.rootRepo = filepath.Join(s.scope, "root-app")
	initRepoWithPubspec(t, s.rootRepo, "root_app", rootBody)
	for _, r := range repos {
		initRepoWithPubspec(t, filepath.Join(s.scope, r.dir), r.name, r.body)
	}
	return s
}

func (s pkgScope) env() []string {
	return append(testEnv(s.home), "PUB_CACHE="+filepath.Join(s.home, ".pub-cache"))
}

func (s pkgScope) createArgs(selected ...string) []string {
	args := []string{"create", s.name, "--scope", s.scope, "--root-repo", "root-app", "--yes", "--non-interactive"}
	for _, pkg := range selected {
		args = append(args, "--package", pkg, "--package-base", pkg+"=main")
	}
	return args
}

func (s pkgScope) create(t *testing.T, selected ...string) {
	t.Helper()
	runE2E(t, s.rootRepo, s.env(), s.createArgs(selected...)...).requireCode(t, 0)
}

func (s pkgScope) pkgWT(folder string) string {
	return filepath.Join(s.home, "Documents", "worktrees", s.name, "packages", folder)
}

func (s pkgScope) rootWT() string {
	return filepath.Join(s.home, "Documents", "worktrees", s.name, "root", "root-app")
}

func commitAndPush(t *testing.T, repo, message string) {
	t.Helper()
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", message)
	runGit(t, repo, "push", "origin", "main")
}

func overrideKeys(t *testing.T, worktree string) []string {
	t.Helper()
	keys := []string{}
	for name := range readOverrides(t, worktree) {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	return keys
}

const dartSDK = "environment:\n  sdk: ^3.0.0\n"

// Case A: pkg_b reaches pkg_a through a path that is not a sibling path, so only
// a package-level override lets `flutree pubget` resolve it from the worktrees.
func TestE2EPackageOverridesRealDartResolvesFromWorktrees(t *testing.T) {
	dart, err := exec.LookPath("dart")
	if err != nil {
		t.Skip("dart is not on PATH; real package override resolution test skipped")
	}
	s := newPkgScope(t, "feature-pkgovr", dartSDK+"dependencies:\n  pkg_b:\n    path: ../pkgs/pkg-b\n",
		pkgRepo{"libs/pkg-a", "pkg_a", dartSDK},
		pkgRepo{"pkgs/pkg-b", "pkg_b", dartSDK + "dependencies:\n  pkg_a:\n    path: ../../libs/pkg-a\n"},
	)
	writeFile(t, filepath.Join(s.scope, "libs/pkg-a/lib/pkg_a.dart"), "String aSource() => 'a-source';\n")
	commitAndPush(t, filepath.Join(s.scope, "libs/pkg-a"), "lib")
	writeFile(t, filepath.Join(s.scope, "pkgs/pkg-b/lib/pkg_b.dart"), "import 'package:pkg_a/pkg_a.dart';\n\nString bSource() => 'b+${aSource()}';\n")
	writeFile(t, filepath.Join(s.scope, "pkgs/pkg-b/bin/main.dart"), "import 'package:pkg_b/pkg_b.dart';\n\nvoid main() => print(bSource());\n")
	commitAndPush(t, filepath.Join(s.scope, "pkgs/pkg-b"), "lib")
	writeFile(t, filepath.Join(s.rootRepo, "lib/root.dart"), "import 'package:pkg_b/pkg_b.dart';\n\nString rootSource() => bSource();\n")
	commitAndPush(t, s.rootRepo, "lib")

	s.create(t, "pkg-a", "pkg-b")

	bWT, aWT := s.pkgWT("pkg-b"), s.pkgWT("pkg-a")
	got := readOverrides(t, bWT)
	if target := overridePathTarget(t, bWT, got["pkg_a"]); target != aWT {
		t.Fatalf("pkg-b override for pkg_a resolves to %s, want the pkg-a worktree %s", target, aWT)
	}

	runE2E(t, s.home, s.env(), "pubget", s.name).requireCode(t, 0)

	writeFile(t, filepath.Join(aWT, "lib/pkg_a.dart"), "String aSource() => 'a-worktree';\n")
	if out := runDart(t, dart, bWT, s.env(), "run", "bin/main.dart"); !strings.Contains(out, "b+a-worktree") {
		t.Fatalf("pkg-b resolved pkg_a from the source repo instead of its worktree; dart run output:\n%s", out)
	}
}

// Case C: a git or hosted dependency on a selected package resolves from the
// pub cache or the remote unless the package file overrides it.
func TestE2EPackageOverridesCoverGitAndHostedDependencies(t *testing.T) {
	cases := map[string]func(scope string) string{
		"git": func(scope string) string {
			return "  pkg_a:\n    git:\n      url: file://" + filepath.Join(scope, "libs/pkg-a") + "\n      ref: main\n"
		},
		"hosted": func(string) string { return "  pkg_a: ^1.0.0\n" },
	}
	for name, dep := range cases {
		t.Run(name, func(t *testing.T) {
			s := newPkgScope(t, "feature-"+name, dartSDK,
				pkgRepo{"libs/pkg-a", "pkg_a", dartSDK},
				pkgRepo{"pkgs/pkg-b", "pkg_b", ""},
			)
			bRepo := filepath.Join(s.scope, "pkgs/pkg-b")
			writeFile(t, filepath.Join(bRepo, "pubspec.yaml"), "name: pkg_b\n"+dartSDK+"dependencies:\n"+dep(s.scope))
			commitAndPush(t, bRepo, "dep")

			s.create(t, "pkg-a", "pkg-b")

			bWT := s.pkgWT("pkg-b")
			if target := overridePathTarget(t, bWT, readOverrides(t, bWT)["pkg_a"]); target != s.pkgWT("pkg-a") {
				t.Fatalf("pkg_a resolves to %s, want the pkg-a worktree %s", target, s.pkgWT("pkg-a"))
			}
		})
	}
}

func TestE2EPackageOverridesAreTransitiveOverSelectedPackages(t *testing.T) {
	s := newPkgScope(t, "feature-trans", dartSDK,
		pkgRepo{"libs/pkg-a", "pkg_a", dartSDK},
		pkgRepo{"pkgs/pkg-b", "pkg_b", dartSDK + "dependencies:\n  pkg_a:\n    path: ../../libs/pkg-a\n"},
		pkgRepo{"apps/pkg-c", "pkg_c", dartSDK + "dependencies:\n  pkg_b: ^1.0.0\n"},
		pkgRepo{"libs/pkg-d", "pkg_d", dartSDK},
	)
	s.create(t, "pkg-a", "pkg-b", "pkg-c", "pkg-d")

	if got := strings.Join(overrideKeys(t, s.pkgWT("pkg-c")), ","); got != "pkg_a,pkg_b" {
		t.Errorf("pkg-c overrides = %s, want pkg_a,pkg_b (pkg_d is outside its graph)", got)
	}
	if got := strings.Join(overrideKeys(t, s.pkgWT("pkg-b")), ","); got != "pkg_a" {
		t.Errorf("pkg-b overrides = %s, want pkg_a only", got)
	}
	for _, leaf := range []string{"pkg-a", "pkg-d"} {
		if exists(filepath.Join(s.pkgWT(leaf), "pubspec_overrides.yaml")) {
			t.Errorf("%s depends on no selected package and must get no override file", leaf)
		}
	}
	cWT := s.pkgWT("pkg-c")
	if target := overridePathTarget(t, cWT, readOverrides(t, cWT)["pkg_a"]); target != s.pkgWT("pkg-a") {
		t.Errorf("pkg-c override for pkg_a resolves to %s, want %s", target, s.pkgWT("pkg-a"))
	}
}

func TestE2EPackageOverridesCarryThePackagesOwnDependencyOverrides(t *testing.T) {
	bBody := dartSDK + `dependencies:
  local:
    path: packages/local
dependency_overrides:
  intl: 0.19.0
  local:
    path: packages/local
  shared:
    path: ../../shared-lib
`
	s := newPkgScope(t, "feature-own", dartSDK,
		pkgRepo{"libs/pkg-a", "pkg_a", dartSDK},
		pkgRepo{"pkgs/pkg-b", "pkg_b", bBody},
	)
	s.create(t, "pkg-a", "pkg-b")

	bWT := s.pkgWT("pkg-b")
	got := readOverrides(t, bWT)
	if got["intl"] != "0.19.0" {
		t.Errorf("intl = %#v, want 0.19.0", got["intl"])
	}
	if target := overridePathTarget(t, bWT, got["local"]); target != filepath.Join(bWT, "packages", "local") {
		t.Errorf("in-repo local resolves to %s, want it kept inside the worktree", target)
	}
	if target := overridePathTarget(t, bWT, got["shared"]); target != filepath.Join(s.scope, "shared-lib") {
		t.Errorf("shared resolves to %s, want the rebased %s", target, filepath.Join(s.scope, "shared-lib"))
	}
	if _, ok := got["pkg_a"]; ok {
		t.Errorf("pkg_a is outside pkg-b's graph and must not be overridden: %#v", got)
	}
}

func TestE2EPackageOverridesStayOutOfGitAndCompleteWithoutForce(t *testing.T) {
	s := newPkgScope(t, "feature-clean", dartSDK,
		pkgRepo{"libs/pkg-a", "pkg_a", dartSDK},
		pkgRepo{"pkgs/pkg-b", "pkg_b", dartSDK + "dependencies:\n  pkg_a:\n    path: ../../libs/pkg-a\n"},
	)
	s.create(t, "pkg-a", "pkg-b")

	bWT := s.pkgWT("pkg-b")
	if !exists(filepath.Join(bWT, "pubspec_overrides.yaml")) {
		t.Fatal("pkg-b should have a pubspec_overrides.yaml")
	}
	for _, folder := range []string{"pkg-a", "pkg-b"} {
		if status := gitStatusPorcelain(t, s.pkgWT(folder)); status != "" {
			t.Errorf("%s worktree must be clean after create, got:\n%s", folder, status)
		}
	}
	if !isGitIgnored(t, bWT, "pubspec_overrides.yaml") {
		t.Error("pkg-b pubspec_overrides.yaml must be git-ignored")
	}
	runE2E(t, s.home, s.env(), "complete", s.name, "--yes", "--non-interactive").requireCode(t, 0)
	if exists(bWT) {
		t.Error("complete without --force must remove the pkg-b worktree")
	}
}

func TestE2EAddRepoRegeneratesPackageOverridesAndKeepsUserEdits(t *testing.T) {
	s := newPkgScope(t, "feature-add", dartSDK,
		pkgRepo{"libs/pkg-a", "pkg_a", dartSDK},
		pkgRepo{"pkgs/pkg-b", "pkg_b", dartSDK + "dependencies:\n  pkg_a:\n    path: ../../libs/pkg-a\n"},
	)
	s.create(t, "pkg-b")

	bWT := s.pkgWT("pkg-b")
	// Before pkg-a is attached it resolves from the source checkout.
	if target := overridePathTarget(t, bWT, readOverrides(t, bWT)["pkg_a"]); target != filepath.Join(s.scope, "libs/pkg-a") {
		t.Fatalf("before add-repo pkg_a resolves to %s, want the source repo", target)
	}
	// A user adds an entry by hand.
	writeFile(t, filepath.Join(bWT, "pubspec_overrides.yaml"),
		"dependency_overrides:\n  pkg_a:\n    path: "+filepath.Join(s.scope, "libs/pkg-a")+"\n  custom: 9.9.9\n")

	runE2E(t, s.rootRepo, s.env(), "add-repo", s.name, "--scope", s.scope, "--repo", "pkg-a",
		"--package-base", "pkg-a=main", "--non-interactive").requireCode(t, 0)

	got := readOverrides(t, bWT)
	if target := overridePathTarget(t, bWT, got["pkg_a"]); target != s.pkgWT("pkg-a") {
		t.Errorf("after add-repo pkg_a resolves to %s, want the pkg-a worktree %s", target, s.pkgWT("pkg-a"))
	}
	if got["custom"] != "9.9.9" {
		t.Errorf("the user-edited custom entry was lost: %#v", got)
	}
	if status := gitStatusPorcelain(t, bWT); status != "" {
		t.Errorf("pkg-b worktree must stay clean after add-repo, got:\n%s", status)
	}
}

func TestE2EPackageOverridesSkipPubWorkspacePackages(t *testing.T) {
	s := newPkgScope(t, "feature-ws", dartSDK,
		pkgRepo{"libs/pkg-a", "pkg_a", dartSDK},
		pkgRepo{"pkgs/pkg-b", "pkg_b", dartSDK + "resolution: workspace\ndependencies:\n  pkg_a:\n    path: ../../libs/pkg-a\n"},
	)
	args := append(s.createArgs("pkg-a", "pkg-b"), "--json")
	res := runE2E(t, s.rootRepo, s.env(), args...)
	res.requireCode(t, 0)

	if exists(filepath.Join(s.pkgWT("pkg-b"), "pubspec_overrides.yaml")) {
		t.Error("a package using pub workspaces must get no override file")
	}
	var out struct {
		PackageOverrides []struct {
			Repo, Path, Status, Note string
		} `json:"package_overrides"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &out); err != nil {
		t.Fatalf("create --json is not JSON: %v\n%s", err, res.stdout)
	}
	if len(out.PackageOverrides) != 1 || out.PackageOverrides[0].Repo != "pkg-b" ||
		out.PackageOverrides[0].Status != "skipped" || !strings.Contains(out.PackageOverrides[0].Note, "workspace") {
		t.Fatalf("package_overrides = %+v, want one skipped pkg-b entry with a workspace note", out.PackageOverrides)
	}
}

func TestE2EPackageOverridesReportedInJSONResults(t *testing.T) {
	s := newPkgScope(t, "feature-json", dartSDK,
		pkgRepo{"libs/pkg-a", "pkg_a", dartSDK},
		pkgRepo{"pkgs/pkg-b", "pkg_b", dartSDK + "dependencies:\n  pkg_a:\n    path: ../../libs/pkg-a\n"},
	)
	res := runE2E(t, s.rootRepo, s.env(), append(s.createArgs("pkg-b"), "--json")...)
	res.requireCode(t, 0)
	var created struct {
		PackageOverrides []struct{ Repo, Status string } `json:"package_overrides"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &created); err != nil {
		t.Fatal(err)
	}
	if len(created.PackageOverrides) != 1 || created.PackageOverrides[0].Repo != "pkg-b" || created.PackageOverrides[0].Status != "written" {
		t.Fatalf("create package_overrides = %+v", created.PackageOverrides)
	}

	res = runE2E(t, s.rootRepo, s.env(), "add-repo", s.name, "--scope", s.scope, "--repo", "pkg-a",
		"--package-base", "pkg-a=main", "--non-interactive", "--json")
	res.requireCode(t, 0)
	var added struct {
		PackageOverrides []struct{ Repo, Status string } `json:"package_overrides"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &added); err != nil {
		t.Fatal(err)
	}
	if len(added.PackageOverrides) != 1 || added.PackageOverrides[0].Repo != "pkg-b" || added.PackageOverrides[0].Status != "written" {
		t.Fatalf("add-repo package_overrides = %+v", added.PackageOverrides)
	}
}

func TestE2EMCPPlanCreateAndStatusCoverPackageOverrides(t *testing.T) {
	s := newPkgScope(t, "feature-mcp-pkg", dartSDK,
		pkgRepo{"libs/pkg-a", "pkg_a", dartSDK},
		pkgRepo{"pkgs/pkg-b", "pkg_b", dartSDK + "dependencies:\n  pkg_a:\n    path: ../../libs/pkg-a\n"},
	)
	planned := toolJSON(t, s.rootRepo, s.env(), "plan_create", map[string]any{
		"name":         s.name,
		"root_repo":    "root-app",
		"packages":     []string{"pkg-a", "pkg-b"},
		"base_branch":  "main",
		"package_base": map[string]string{"pkg-a": "main", "pkg-b": "main"},
		"scope":        s.scope,
	})
	entries := asSlice(t, planned["package_overrides"])
	if len(entries) != 1 {
		t.Fatalf("plan_create package_overrides = %v, want exactly the pkg-b file", planned["package_overrides"])
	}
	entry := asMap(t, entries[0])
	if entry["repo"] != "pkg-b" || entry["path"] != filepath.Join(s.pkgWT("pkg-b"), "pubspec_overrides.yaml") ||
		!strings.Contains(entry["content"].(string), "pkg_a:") {
		t.Fatalf("plan_create pkg-b override = %v", entry)
	}

	s.create(t, "pkg-a", "pkg-b")
	status := toolJSON(t, t.TempDir(), s.env(), "workspace_status", map[string]any{"name": s.name})
	byName := map[string]map[string]any{}
	for _, raw := range asSlice(t, status["packages"]) {
		pkg := asMap(t, raw)
		byName[pkg["name"].(string)] = asMap(t, pkg["pubspec_overrides"])
	}
	if byName["pkg-b"]["exists"] != true {
		t.Errorf("pkg-b override must exist: %v", byName["pkg-b"])
	}
	if byName["pkg-a"]["exists"] != false {
		t.Errorf("pkg-a has no override file: %v", byName["pkg-a"])
	}
	if _, ok := asMap(t, byName["pkg-b"]["entries"])["pkg_a"]; !ok {
		t.Errorf("pkg-b entries must list pkg_a: %v", byName["pkg-b"])
	}
}
