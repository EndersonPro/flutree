//go:build !windows

package integration_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// rootPubspecWithOverrides declares every override shape pub supports: a bare
// version, a path (block and flow style), git and hosted.
const rootPubspecWithOverrides = `environment:
  sdk: ^3.0.0
dependencies:
  core:
    path: ../core-pkg
dependency_overrides:
  intl: 0.19.0
  shared:
    path: ../shared
  inline_shared: {path: ../inline-shared}
  remote:
    git:
      url: https://example.com/remote.git
      ref: main
  hosted_pkg:
    hosted: https://pub.example.com
    version: ^1.2.0
`

func readOverrides(t *testing.T, worktree string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(worktree, "pubspec_overrides.yaml"))
	if err != nil {
		t.Fatalf("read pubspec_overrides.yaml: %v", err)
	}
	var doc struct {
		DependencyOverrides map[string]any `yaml:"dependency_overrides"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatalf("pubspec_overrides.yaml is not valid YAML: %v\n%s", err, b)
	}
	return doc.DependencyOverrides
}

// overridePathTarget returns the directory an override's path resolves to,
// relative to the worktree that holds the pubspec_overrides.yaml.
func overridePathTarget(t *testing.T, worktree string, spec any) string {
	t.Helper()
	m, ok := spec.(map[string]any)
	if !ok {
		t.Fatalf("expected a path override map, got %#v", spec)
	}
	p, ok := m["path"].(string)
	if !ok {
		t.Fatalf("expected a path override, got %#v", spec)
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(worktree, p)
	}
	return filepath.Clean(p)
}

func assertCarriedOverrides(t *testing.T, ws e2eWorkspace, got map[string]any) {
	t.Helper()
	if got["intl"] != "0.19.0" {
		t.Errorf("intl override = %#v, want \"0.19.0\"", got["intl"])
	}
	if target := overridePathTarget(t, ws.rootWT, got["shared"]); target != filepath.Join(ws.scope, "shared") {
		t.Errorf("shared override resolves to %s, want %s", target, filepath.Join(ws.scope, "shared"))
	}
	if target := overridePathTarget(t, ws.rootWT, got["inline_shared"]); target != filepath.Join(ws.scope, "inline-shared") {
		t.Errorf("inline_shared override resolves to %s, want %s", target, filepath.Join(ws.scope, "inline-shared"))
	}
	remote, _ := got["remote"].(map[string]any)
	git, _ := remote["git"].(map[string]any)
	if git["url"] != "https://example.com/remote.git" || git["ref"] != "main" {
		t.Errorf("remote git override = %#v", got["remote"])
	}
	hosted, _ := got["hosted_pkg"].(map[string]any)
	if hosted["hosted"] != "https://pub.example.com" || hosted["version"] != "^1.2.0" {
		t.Errorf("hosted override = %#v", got["hosted_pkg"])
	}
}

func TestE2ECreateCarriesRootPubspecDependencyOverrides(t *testing.T) {
	ws := createWorkspaceWithRootPubspec(t, "feature-ovr", []string{"core-pkg"}, rootPubspecWithOverrides)

	got := readOverrides(t, ws.rootWT)
	assertCarriedOverrides(t, ws, got)
	if target := overridePathTarget(t, ws.rootWT, got["core"]); target != ws.pkgWTs["core-pkg"] {
		t.Errorf("core override resolves to %s, want package worktree %s", target, ws.pkgWTs["core-pkg"])
	}
}

func TestE2EAddRepoCarriesRootPubspecDependencyOverrides(t *testing.T) {
	ws := createWorkspaceWithRootPubspec(t, "feature-ovr", nil, rootPubspecWithOverrides)
	initRepoWithPubspec(t, filepath.Join(ws.scope, "core-pkg"), "core", "")

	runE2E(t, ws.rootRepo, testEnv(ws.home),
		"add-repo", ws.name, "--scope", ws.scope, "--repo", "core-pkg",
		"--package-base", "core-pkg=main", "--non-interactive").requireCode(t, 0)

	got := readOverrides(t, ws.rootWT)
	assertCarriedOverrides(t, ws, got)
	pkgWT := filepath.Join(ws.home, "Documents", "worktrees", ws.name, "packages", "core-pkg")
	if target := overridePathTarget(t, ws.rootWT, got["core"]); target != pkgWT {
		t.Errorf("core override resolves to %s, want package worktree %s", target, pkgWT)
	}
}

func TestE2EPackageWorktreeWinsOverRootPubspecOverride(t *testing.T) {
	pubspec := "environment:\n  sdk: ^3.0.0\ndependency_overrides:\n  core: 2.0.0\n"
	ws := createWorkspaceWithRootPubspec(t, "feature-ovr", []string{"core-pkg"}, pubspec)

	got := readOverrides(t, ws.rootWT)
	if target := overridePathTarget(t, ws.rootWT, got["core"]); target != ws.pkgWTs["core-pkg"] {
		t.Fatalf("core override resolves to %s, want package worktree %s", target, ws.pkgWTs["core-pkg"])
	}
}

// TestE2ERealDartResolvesMultiRepoWorkspace builds a real multi-repo
// workspace and resolves it with the Dart SDK: the root must compile against
// the package worktree (not the source repo) and keep its own path override.
func TestE2ERealDartResolvesMultiRepoWorkspace(t *testing.T) {
	dart, err := exec.LookPath("dart")
	if err != nil {
		t.Skip("dart is not on PATH; real multi-repo resolution test skipped")
	}

	home := t.TempDir()
	scope := filepath.Join(t.TempDir(), "workspace")
	writeDartPackage(t, filepath.Join(scope, "shared"), "shared", "String sharedSource() => 'shared-original';\n")
	writeDartPackage(t, filepath.Join(scope, "shared-fork"), "shared", "String sharedSource() => 'shared-fork';\n")

	coreRepo := filepath.Join(scope, "core-pkg")
	initRepoWithPubspec(t, coreRepo, "core", "environment:\n  sdk: ^3.0.0\n")
	writeFile(t, filepath.Join(coreRepo, "lib", "core.dart"), "String coreSource() => 'core-source-repo';\n")
	runGit(t, coreRepo, "add", ".")
	runGit(t, coreRepo, "commit", "-m", "lib")
	runGit(t, coreRepo, "push", "origin", "main")

	rootRepo := filepath.Join(scope, "root-app")
	initRepoWithPubspec(t, rootRepo, "root_app", `environment:
  sdk: ^3.0.0
dependencies:
  core:
    path: ../core-pkg
  shared:
    path: ../shared
dependency_overrides:
  shared:
    path: ../shared-fork
`)
	writeFile(t, filepath.Join(rootRepo, "bin", "main.dart"), `import 'package:core/core.dart';
import 'package:shared/shared.dart';

void main() => print('${coreSource()} ${sharedSource()}');
`)
	runGit(t, rootRepo, "add", ".")
	runGit(t, rootRepo, "commit", "-m", "bin")
	runGit(t, rootRepo, "push", "origin", "main")

	runE2E(t, rootRepo, testEnv(home),
		"create", "feature-real", "--scope", scope, "--root-repo", "root-app",
		"--package", "core-pkg", "--package-base", "core-pkg=main",
		"--yes", "--non-interactive").requireCode(t, 0)

	base := filepath.Join(home, "Documents", "worktrees", "feature-real")
	rootWT := filepath.Join(base, "root", "root-app")
	coreWT := filepath.Join(base, "packages", "core-pkg")
	// Change the package in its worktree only; the root must pick this up.
	writeFile(t, filepath.Join(coreWT, "lib", "core.dart"), "String coreSource() => 'core-worktree';\n")

	dartEnv := append(testEnv(home), "PUB_CACHE="+filepath.Join(home, ".pub-cache"))
	runDart(t, dart, rootWT, dartEnv, "pub", "get", "--offline")
	out := runDart(t, dart, rootWT, dartEnv, "run", "bin/main.dart")
	if !strings.Contains(out, "core-worktree shared-fork") {
		t.Fatalf("root did not resolve the package worktree and the carried override; dart run output:\n%s", out)
	}
}

func writeDartPackage(t *testing.T, dir, name, lib string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "pubspec.yaml"), "name: "+name+"\nenvironment:\n  sdk: ^3.0.0\n")
	writeFile(t, filepath.Join(dir, "lib", name+".dart"), lib)
}

func runDart(t *testing.T, dart, cwd string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command(dart, args...)
	cmd.Dir = cwd
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dart %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}

// createWorkspaceWithRootPubspec is createWorkspace with a custom root
// pubspec body (package repos keep a minimal pubspec).
func createWorkspaceWithRootPubspec(t *testing.T, name string, packages []string, rootPubspec string) e2eWorkspace {
	t.Helper()
	ws := e2eWorkspace{home: t.TempDir(), scope: filepath.Join(t.TempDir(), "workspace"), name: name, pkgWTs: map[string]string{}}
	ws.rootRepo = filepath.Join(ws.scope, "root-app")
	initRepoWithPubspec(t, ws.rootRepo, "root_app", rootPubspec)
	args := []string{"create", name, "--scope", ws.scope, "--root-repo", "root-app", "--yes", "--non-interactive"}
	if len(packages) == 0 {
		args = append(args, "--no-package")
	}
	for _, pkg := range packages {
		initRepoWithPubspec(t, filepath.Join(ws.scope, pkg), strings.ReplaceAll(strings.TrimSuffix(pkg, "-pkg"), "-", "_"), "")
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
