//go:build !windows

package integration_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// graphScope is a scope with a root repo plus package repos, each created with
// its own pubspec body. Repos are named "pkg-a" and have package name "pkg_a".
type graphScope struct {
	home  string
	scope string
	root  string
	base  string // worktree container of the workspace
	name  string
}

func newGraphScope(t *testing.T, name, rootBody string, repos map[string]string) graphScope {
	t.Helper()
	g := graphScope{home: t.TempDir(), scope: filepath.Join(t.TempDir(), "workspace"), name: name}
	g.root = filepath.Join(g.scope, "root-app")
	initRepoWithPubspec(t, g.root, "root_app", rootBody)
	for repo, body := range repos {
		initRepoWithPubspec(t, filepath.Join(g.scope, repo), strings.ReplaceAll(repo, "-", "_"), body)
	}
	g.base = filepath.Join(g.home, "Documents", "worktrees", name)
	return g
}

func (g graphScope) create(t *testing.T, selected ...string) {
	t.Helper()
	args := []string{"create", g.name, "--scope", g.scope, "--root-repo", "root-app", "--yes", "--non-interactive"}
	for _, pkg := range selected {
		args = append(args, "--package", pkg, "--package-base", pkg+"=main")
	}
	runE2E(t, g.root, testEnv(g.home), args...).requireCode(t, 0)
}

func (g graphScope) rootWT() string          { return filepath.Join(g.base, "root", "root-app") }
func (g graphScope) pkgWT(pkg string) string { return filepath.Join(g.base, "packages", pkg) }

const graphSDK = "environment:\n  sdk: ^3.0.0\n"

func pathDep(name, rel string) string {
	return "  " + name + ":\n    path: " + rel + "\n"
}

func TestE2EGraphRootPathDepsOutsideWorkspaceAreRebased(t *testing.T) {
	rootBody := graphSDK + "dependencies:\n" + pathDep("pkg_a", "../pkg-a") + pathDep("pkg_b", "../pkg-b") + pathDep("pkg_c", "../pkg-c")
	g := newGraphScope(t, "graph-root", rootBody, map[string]string{"pkg-a": "", "pkg-b": "", "pkg-c": ""})
	g.create(t, "pkg-a")

	got := readOverrides(t, g.rootWT())
	if target := overridePathTarget(t, g.rootWT(), got["pkg_a"]); target != g.pkgWT("pkg-a") {
		t.Errorf("pkg_a resolves to %s, want the worktree %s", target, g.pkgWT("pkg-a"))
	}
	for _, name := range []string{"pkg_b", "pkg_c"} {
		want := filepath.Join(g.scope, strings.ReplaceAll(name, "_", "-"))
		if target := overridePathTarget(t, g.rootWT(), got[name]); target != want {
			t.Errorf("%s resolves to %s, want the source repo %s", name, target, want)
		}
	}
}

func TestE2EGraphRootDevDependenciesAreRebased(t *testing.T) {
	rootBody := graphSDK + "dev_dependencies:\n" + pathDep("pkg_c", "../pkg-c")
	g := newGraphScope(t, "graph-dev", rootBody, map[string]string{"pkg-a": "", "pkg-c": ""})
	g.create(t, "pkg-a")

	if target := overridePathTarget(t, g.rootWT(), readOverrides(t, g.rootWT())["pkg_c"]); target != filepath.Join(g.scope, "pkg-c") {
		t.Errorf("pkg_c resolves to %s, want %s", target, filepath.Join(g.scope, "pkg-c"))
	}
}

func TestE2EGraphSelectedPackageDepOnUnselectedRepo(t *testing.T) {
	g := newGraphScope(t, "graph-pkgdep", graphSDK, map[string]string{
		"pkg-a": "",
		"pkg-b": graphSDK + "dependencies:\n" + pathDep("pkg_x", "../pkg-x"),
		"pkg-x": "",
	})
	g.create(t, "pkg-a", "pkg-b")

	got := readOverrides(t, g.rootWT())
	if target := overridePathTarget(t, g.rootWT(), got["pkg_x"]); target != filepath.Join(g.scope, "pkg-x") {
		t.Errorf("pkg_x resolves to %s, want %s", target, filepath.Join(g.scope, "pkg-x"))
	}
}

func TestE2EGraphSelectedPackageDepOnSelectedPackageUsesWorktree(t *testing.T) {
	g := newGraphScope(t, "graph-pkgpkg", graphSDK, map[string]string{
		"pkg-a": "",
		"pkg-b": graphSDK + "dependencies:\n" + pathDep("pkg_a", "../pkg-a"),
	})
	g.create(t, "pkg-a", "pkg-b")

	got := readOverrides(t, g.rootWT())
	if target := overridePathTarget(t, g.rootWT(), got["pkg_a"]); target != g.pkgWT("pkg-a") {
		t.Errorf("pkg_a resolves to %s, want the worktree %s", target, g.pkgWT("pkg-a"))
	}
}

func TestE2EGraphInRepoRootPathDepNeedsNoRebase(t *testing.T) {
	rootBody := graphSDK + "dependencies:\n" + pathDep("local", "packages/local")
	g := newGraphScope(t, "graph-inrepo", rootBody, map[string]string{"pkg-a": ""})
	g.create(t, "pkg-a")

	got := readOverrides(t, g.rootWT())
	spec, ok := got["local"]
	if !ok {
		return // no override: the relative path already resolves inside the worktree
	}
	if target := overridePathTarget(t, g.rootWT(), spec); target != filepath.Join(g.rootWT(), "packages", "local") {
		t.Errorf("local resolves to %s, want the worktree copy", target)
	}
}

func TestE2EGraphRealDartResolvesWholeGraph(t *testing.T) {
	dart, err := exec.LookPath("dart")
	if err != nil {
		t.Skip("dart is not on PATH; real graph resolution test skipped")
	}

	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	repos := map[string]string{
		"pkg-a": "String aSource() => 'a-source';\n",
		"pkg-b": "import 'package:pkg_a/pkg_a.dart';\n\nString bSource() => 'b-source+' + aSource();\n",
		"pkg-c": "String cSource() => 'c-source';\n",
	}
	deps := map[string]string{
		"pkg-b": "dependencies:\n" + pathDep("pkg_a", "../pkg-a"),
	}
	for repo, lib := range repos {
		name := strings.ReplaceAll(repo, "-", "_")
		dir := filepath.Join(scope, repo)
		initRepoWithPubspec(t, dir, name, graphSDK+deps[repo])
		writeFile(t, filepath.Join(dir, "lib", name+".dart"), lib)
		runGit(t, dir, "add", ".")
		runGit(t, dir, "commit", "-m", "lib")
		runGit(t, dir, "push", "origin", "main")
	}

	rootRepo := filepath.Join(scope, "root-app")
	initRepoWithPubspec(t, rootRepo, "root_app", graphSDK+"dependencies:\n"+
		pathDep("pkg_a", "../pkg-a")+pathDep("pkg_b", "../pkg-b")+pathDep("pkg_c", "../pkg-c"))
	writeFile(t, filepath.Join(rootRepo, "bin", "main.dart"), `import 'package:pkg_a/pkg_a.dart';
import 'package:pkg_b/pkg_b.dart';
import 'package:pkg_c/pkg_c.dart';

void main() => print('${aSource()} ${bSource()} ${cSource()}');
`)
	runGit(t, rootRepo, "add", ".")
	runGit(t, rootRepo, "commit", "-m", "bin")
	runGit(t, rootRepo, "push", "origin", "main")

	runE2E(t, rootRepo, testEnv(home), "create", "graph-real", "--scope", scope, "--root-repo", "root-app",
		"--package", "pkg-a", "--package-base", "pkg-a=main",
		"--package", "pkg-b", "--package-base", "pkg-b=main",
		"--yes", "--non-interactive").requireCode(t, 0)

	base := filepath.Join(home, "Documents", "worktrees", "graph-real")
	rootWT := filepath.Join(base, "root", "root-app")
	writeFile(t, filepath.Join(base, "packages", "pkg-a", "lib", "pkg_a.dart"), "String aSource() => 'a-worktree';\n")
	writeFile(t, filepath.Join(base, "packages", "pkg-b", "lib", "pkg_b.dart"),
		"import 'package:pkg_a/pkg_a.dart';\n\nString bSource() => 'b-worktree+' + aSource();\n")

	dartEnv := append(testEnv(home), "PUB_CACHE="+filepath.Join(home, ".pub-cache"))
	runDart(t, dart, rootWT, dartEnv, "pub", "get", "--offline")
	out := runDart(t, dart, rootWT, dartEnv, "run", "bin/main.dart")
	if want := "a-worktree b-worktree+a-worktree c-source"; !strings.Contains(out, want) {
		t.Fatalf("dart run output = %q, want it to contain %q", out, want)
	}
}
