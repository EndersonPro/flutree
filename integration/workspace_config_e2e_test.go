//go:build !windows

package integration_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func workspaceFile(home, name string) string {
	return filepath.Join(home, "Documents", "worktrees", name, name+".code-workspace")
}

func createRootOnly(t *testing.T, home, scope, name string, extra ...string) {
	t.Helper()
	args := append([]string{"create", name, "--scope", scope, "--root-repo", "root-app", "--no-package", "--yes", "--non-interactive"}, extra...)
	runE2E(t, filepath.Join(scope, "root-app"), testEnv(home), args...).requireCode(t, 0)
}

func newRootOnlyScope(t *testing.T) string {
	t.Helper()
	scope := filepath.Join(t.TempDir(), "workspace")
	initRepoWithPackageName(t, filepath.Join(scope, "root-app"), "root_app")
	return scope
}

func TestE2EConfigWorkspaceGenerateRoundTrip(t *testing.T) {
	home := t.TempDir()

	get := runE2E(t, home, testEnv(home), "config", "get", "workspace.generate")
	get.requireCode(t, 0)
	if strings.TrimSpace(get.stdout) != "true" {
		t.Fatalf("unset workspace.generate should report the default true, got %q", get.stdout)
	}

	set := runE2E(t, home, testEnv(home), "config", "set", "workspace.generate", "false")
	set.requireCode(t, 0)
	if strings.TrimSpace(set.stdout) != "false" {
		t.Fatalf("config set stdout = %q, want false", set.stdout)
	}

	jsonGet := runE2E(t, home, testEnv(home), "config", "get", "workspace.generate", "--json")
	jsonGet.requireCode(t, 0)
	var out map[string]string
	if err := json.Unmarshal([]byte(jsonGet.stdout), &out); err != nil || out["key"] != "workspace.generate" || out["value"] != "false" {
		t.Fatalf("config get --json = %q (%v)", jsonGet.stdout, err)
	}
}

func TestE2EConfigWorkspaceGenerateRejectsNonBoolean(t *testing.T) {
	home := t.TempDir()
	res := runE2E(t, home, testEnv(home), "config", "set", "workspace.generate", "maybe")
	res.requireCode(t, 2)
	if !strings.Contains(res.stderr, "[input] Invalid value for workspace.generate.") {
		t.Fatalf("unexpected stderr: %s", res.stderr)
	}
}

func TestE2ECreateWorkspaceFileFollowsConfigAndFlags(t *testing.T) {
	cases := []struct {
		name       string
		configured string // "" leaves workspace.generate unset
		flags      []string
		wantFile   bool
	}{
		{"default generates", "", nil, true},
		{"config false skips", "false", nil, false},
		{"explicit --workspace beats config false", "false", []string{"--workspace"}, true},
		{"config true generates", "true", nil, true},
		{"explicit --no-workspace beats config true", "true", []string{"--no-workspace"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			scope := newRootOnlyScope(t)
			if tc.configured != "" {
				runE2E(t, home, testEnv(home), "config", "set", "workspace.generate", tc.configured).requireCode(t, 0)
			}
			createRootOnly(t, home, scope, "feature-ws", tc.flags...)
			if got := exists(workspaceFile(home, "feature-ws")); got != tc.wantFile {
				t.Fatalf("workspace file exists = %v, want %v", got, tc.wantFile)
			}
		})
	}
}

func TestE2EMCPConfigWorkspaceGenerate(t *testing.T) {
	home := t.TempDir()
	env := testEnv(home)

	set := callMCPTool(t, home, env, "set_config", map[string]any{"key": "workspace.generate", "value": "false"})
	if set.IsError {
		t.Fatalf("set_config failed: %s", set.text(t))
	}
	cli := runE2E(t, home, env, "config", "get", "workspace.generate")
	cli.requireCode(t, 0)
	if strings.TrimSpace(cli.stdout) != "false" {
		t.Fatalf("CLI does not see workspace.generate set over MCP: %q", cli.stdout)
	}
}
