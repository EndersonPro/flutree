//go:build !windows

package integration_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const mcpInitialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"e2e","version":"0.0.1"}}}` + "\n"

type mcpToolResult struct {
	IsError bool `json:"isError"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// callMCPTool runs `flutree mcp serve`, calls one tool and returns its result.
func callMCPTool(t *testing.T, cwd string, env []string, tool string, args map[string]any) mcpToolResult {
	t.Helper()
	params, err := json.Marshal(map[string]any{"name": tool, "arguments": args})
	if err != nil {
		t.Fatal(err)
	}
	input := mcpInitialize + fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":%s}`, params) + "\n"
	res := runMCPServe(t, buildCLI(t), cwd, env, input)

	for _, line := range strings.Split(strings.TrimSpace(res.stdout), "\n") {
		var frame struct {
			ID     float64         `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			t.Fatalf("mcp stdout line is not JSON-RPC: %q", line)
		}
		if frame.ID != 2 {
			continue
		}
		if len(frame.Error) > 0 {
			t.Fatalf("tools/call %s returned JSON-RPC error: %s", tool, frame.Error)
		}
		var out mcpToolResult
		if err := json.Unmarshal(frame.Result, &out); err != nil {
			t.Fatalf("decode tool result: %v\n%s", err, frame.Result)
		}
		return out
	}
	t.Fatalf("no tools/call response in stdout:\n%s\nstderr:\n%s", res.stdout, res.stderr)
	return mcpToolResult{}
}

func (r mcpToolResult) text(t *testing.T) string {
	t.Helper()
	if len(r.Content) == 0 {
		t.Fatalf("tool result has no content: %+v", r)
	}
	return r.Content[0].Text
}

func TestE2EMCPListWorktreesReturnsManagedWorkspace(t *testing.T) {
	ws := createWorkspace(t, "feature-mcp", []string{"core-pkg"}, "")

	res := callMCPTool(t, t.TempDir(), testEnv(ws.home), "list_worktrees", map[string]any{"global_scope": true})
	if res.IsError {
		t.Fatalf("list_worktrees failed: %s", res.text(t))
	}
	var rows []listedRecord
	if err := json.Unmarshal([]byte(res.text(t)), &rows); err != nil {
		t.Fatalf("list_worktrees text is not JSON: %v\n%s", err, res.text(t))
	}
	if len(rows) != 1 || rows[0].Name != ws.name || rows[0].PackageCount != 1 {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestE2EMCPCreateAndCompleteWorktree(t *testing.T) {
	home := t.TempDir()
	scope := t.TempDir()
	repo := scope + "/root-app"
	initRepoWithPackageName(t, repo, "root_app")
	env := testEnv(home)

	created := callMCPTool(t, repo, env, "create_worktree", map[string]any{
		"name": "feature-mcp", "root_repo": "root-app", "scope": scope, "base_branch": "main",
	})
	if created.IsError {
		t.Fatalf("create_worktree failed: %s", created.text(t))
	}
	if rows := listManaged(t, home); len(rows) != 1 || rows[0].Name != "feature-mcp" {
		t.Fatalf("after create_worktree, list = %+v", rows)
	}

	completed := callMCPTool(t, t.TempDir(), env, "complete_worktree", map[string]any{"name": "feature-mcp", "force": true, "confirm": true})
	if completed.IsError {
		t.Fatalf("complete_worktree failed: %s", completed.text(t))
	}
	if rows := listManaged(t, home); len(rows) != 0 {
		t.Fatalf("after complete_worktree, list = %+v", rows)
	}
}

func TestE2EMCPToolErrorsAreReportedAsToolResults(t *testing.T) {
	res := callMCPTool(t, t.TempDir(), testEnv(t.TempDir()), "pubget", map[string]any{"name": "ghost"})
	if !res.IsError {
		t.Fatalf("expected isError for unknown workspace, got: %+v", res)
	}
	if !strings.Contains(res.text(t), "ghost") {
		t.Fatalf("expected error text to name the workspace, got: %s", res.text(t))
	}
}

func TestE2EMCPConfigRoundTripAndVersion(t *testing.T) {
	home := t.TempDir()
	scope := t.TempDir()
	env := testEnv(home)

	set := callMCPTool(t, home, env, "set_config", map[string]any{"key": "scope.root", "value": scope})
	if set.IsError {
		t.Fatalf("set_config failed: %s", set.text(t))
	}
	got := callMCPTool(t, home, env, "get_config", map[string]any{"key": "scope.root"})
	if got.IsError || !strings.Contains(got.text(t), scope) {
		t.Fatalf("get_config = %+v", got)
	}
	cli := runE2E(t, home, env, "config", "get", "scope.root")
	cli.requireCode(t, 0)
	if strings.TrimSpace(cli.stdout) != scope {
		t.Fatalf("CLI does not see the value set over MCP: %q", cli.stdout)
	}

	version := callMCPTool(t, home, env, "get_version", nil)
	if version.IsError || version.text(t) == "" {
		t.Fatalf("get_version = %+v", version)
	}
}
