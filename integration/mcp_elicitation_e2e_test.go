//go:build !windows

package integration_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// elicitRequest is one elicitation/create request the server sent to the
// simulated client.
type elicitRequest struct {
	Message string
	Schema  map[string]any
}

func (r elicitRequest) property(name string) (map[string]any, bool) {
	props, _ := r.Schema["properties"].(map[string]any)
	p, ok := props[name].(map[string]any)
	return p, ok
}

// elicitScript answers an elicitation request with a JSON-RPC result such as
// {"action":"accept","content":{...}}, {"action":"decline"} or
// {"action":"cancel"}.
type elicitScript func(elicitRequest) map[string]any

func accept(content map[string]any) map[string]any {
	return map[string]any{"action": "accept", "content": content}
}

var (
	declineAnswer = map[string]any{"action": "decline"}
	cancelAnswer  = map[string]any{"action": "cancel"}
)

// mcpClient is a minimal JSON-RPC client over the stdio of `flutree mcp serve`.
// It answers server-initiated elicitation requests with a script.
type mcpClient struct {
	t      *testing.T
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	lines  chan string
	script elicitScript
	nextID int

	// asked records every elicitation request, in order.
	asked []elicitRequest
}

const mcpClientDeadline = 30 * time.Second

// startMCPClient spawns the server and performs the handshake. When elicit is
// true the client declares the elicitation capability.
func startMCPClient(t *testing.T, cwd string, env []string, elicit bool, script elicitScript) *mcpClient {
	t.Helper()
	cmd := exec.Command(buildCLI(t), "mcp", "serve")
	cmd.Dir = cwd
	cmd.Env = env
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	c := &mcpClient{t: t, cmd: cmd, stdin: stdin, lines: make(chan string, 64), script: script, nextID: 1}
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
		for sc.Scan() {
			c.lines <- sc.Text()
		}
		close(c.lines)
	}()
	t.Cleanup(c.close)

	caps := "{}"
	if elicit {
		caps = `{"elicitation":{}}`
	}
	c.send(map[string]any{
		"jsonrpc": "2.0", "id": 0, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    json.RawMessage(caps),
			"clientInfo":      map[string]any{"name": "e2e-elicit", "version": "0.0.1"},
		},
	})
	c.await(0)
	c.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	return c
}

func (c *mcpClient) send(msg map[string]any) {
	c.t.Helper()
	raw, err := json.Marshal(msg)
	if err != nil {
		c.t.Fatal(err)
	}
	if _, err := fmt.Fprintf(c.stdin, "%s\n", raw); err != nil {
		c.t.Fatalf("write to server: %v", err)
	}
}

func (c *mcpClient) close() {
	_ = c.stdin.Close()
	done := make(chan struct{})
	go func() { _ = c.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = c.cmd.Process.Kill()
		<-done
	}
}

// await reads frames until the response with the given id arrives, answering
// elicitation requests on the way. It fails the test at the deadline.
func (c *mcpClient) await(id int) json.RawMessage {
	c.t.Helper()
	deadline := time.After(mcpClientDeadline)
	for {
		select {
		case line, ok := <-c.lines:
			if !ok {
				c.t.Fatalf("server closed stdout while waiting for response %d", id)
			}
			var frame struct {
				ID     *float64        `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			if err := json.Unmarshal([]byte(line), &frame); err != nil {
				c.t.Fatalf("server line is not JSON-RPC: %q", line)
			}
			if frame.Method == "elicitation/create" && frame.ID != nil {
				c.answerElicitation(*frame.ID, frame.Params)
				continue
			}
			if frame.Method != "" || frame.ID == nil || int(*frame.ID) != id {
				continue // notification or unrelated frame
			}
			if len(frame.Error) > 0 {
				c.t.Fatalf("JSON-RPC error for request %d: %s", id, frame.Error)
			}
			return frame.Result
		case <-deadline:
			c.t.Fatalf("no response to request %d within %s", id, mcpClientDeadline)
		}
	}
}

func (c *mcpClient) answerElicitation(id float64, params json.RawMessage) {
	c.t.Helper()
	var p struct {
		Message         string         `json:"message"`
		RequestedSchema map[string]any `json:"requestedSchema"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		c.t.Fatalf("bad elicitation params: %v", err)
	}
	req := elicitRequest{Message: p.Message, Schema: p.RequestedSchema}
	c.asked = append(c.asked, req)
	if c.script == nil {
		c.t.Fatalf("server sent an unexpected elicitation request: %q", p.Message)
	}
	c.send(map[string]any{"jsonrpc": "2.0", "id": id, "result": c.script(req)})
}

// request performs one JSON-RPC request and returns its result.
func (c *mcpClient) request(method string, params any) json.RawMessage {
	c.t.Helper()
	c.nextID++
	id := c.nextID
	c.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	return c.await(id)
}

// callTool calls a tool and returns its isError flag and decoded JSON text.
func (c *mcpClient) callTool(tool string, args map[string]any) (bool, map[string]any) {
	c.t.Helper()
	raw := c.request("tools/call", map[string]any{"name": tool, "arguments": args})
	var res mcpToolResult
	if err := json.Unmarshal(raw, &res); err != nil {
		c.t.Fatalf("decode tool result: %v\n%s", err, raw)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.text(c.t)), &out); err != nil {
		c.t.Fatalf("%s text is not a JSON object: %v\n%s", tool, err, res.text(c.t))
	}
	return res.IsError, out
}

// confirmScript answers a yes/no confirmation form with the given action.
func confirmScript(answer map[string]any) elicitScript {
	return func(r elicitRequest) map[string]any {
		if _, ok := r.property("confirm"); !ok {
			return cancelAnswer
		}
		return answer
	}
}

func workspaceContainer(ws e2eWorkspace, name string) string {
	return filepath.Join(ws.home, "Documents", "worktrees", name)
}

// addDevelopBranch gives a repo a develop branch one commit ahead of main and
// leaves main checked out.
func addDevelopBranch(t *testing.T, repo string) string {
	t.Helper()
	runGit(t, repo, "checkout", "-b", "develop")
	runGit(t, repo, "commit", "--allow-empty", "-m", "develop work")
	sha := strings.TrimSpace(runGit(t, repo, "rev-parse", "develop"))
	runGit(t, repo, "checkout", "main")
	return sha
}

// ── create_workspace with an elicitation-capable client ─────────────────────

func TestE2EMCPElicitationAcceptCreatesWorkspace(t *testing.T) {
	ws := planFixture(t)
	c := startMCPClient(t, ws.rootRepo, testEnv(ws.home), true, confirmScript(accept(map[string]any{"confirm": true})))

	isErr, out := c.callTool("create_workspace", planArgs(ws, "feature-yes"))
	if isErr || out["created"] != true || out["status"] != "created" {
		t.Fatalf("accepted create_workspace = %v", out)
	}
	if len(c.asked) != 1 {
		t.Fatalf("expected exactly one question, got %d", len(c.asked))
	}
	q := c.asked[0]
	if !strings.Contains(q.Message, "feature-yes") || !strings.Contains(q.Message, "feature/feature-yes") || !strings.Contains(q.Message, "core-pkg") {
		t.Errorf("the question must name what will be created, got %q", q.Message)
	}
	if prop, _ := q.property("confirm"); prop["type"] != "boolean" {
		t.Errorf("confirmation must be a boolean form field, schema = %v", q.Schema)
	}

	container := workspaceContainer(ws, "feature-yes")
	if !exists(filepath.Join(container, "root", "root-app", "pubspec_overrides.yaml")) || !exists(filepath.Join(container, "packages", "core-pkg")) {
		t.Error("accepted create_workspace did not build the worktrees")
	}
	if rows := listManaged(t, ws.home); len(rows) != 1 || rows[0].Name != "feature-yes" || rows[0].PackageCount != 1 {
		t.Errorf("registry after accept = %+v", rows)
	}
}

func TestE2EMCPElicitationDeclineAndCancelCreateNothing(t *testing.T) {
	cases := []struct {
		name   string
		answer map[string]any
		status string
	}{
		{"decline", declineAnswer, "declined_by_user"},
		{"cancel", cancelAnswer, "cancelled_by_user"},
		{"accept with confirm=false", accept(map[string]any{"confirm": false}), "declined_by_user"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ws := planFixture(t)
			c := startMCPClient(t, ws.rootRepo, testEnv(ws.home), true, confirmScript(tc.answer))

			isErr, out := c.callTool("create_workspace", planArgs(ws, "feature-no"))
			if isErr {
				t.Fatalf("a user decline is a normal result, not an error: %v", out)
			}
			if out["created"] != false || out["status"] != tc.status || out["user_declined"] != true {
				t.Errorf("declined result = %v", out)
			}
			if msg, _ := out["message"].(string); !strings.Contains(msg, "Nothing was executed") {
				t.Errorf("the result must say nothing was executed, got %q", msg)
			}
			assertNothingCreated(t, ws, workspaceContainer(ws, "feature-no"))
			if out := runGit(t, ws.rootRepo, "branch", "--list", "feature/*"); strings.TrimSpace(out) != "" {
				t.Errorf("no branch may be created, got %s", out)
			}
		})
	}
}

func TestE2EMCPElicitationSkippedWhenAgentConfirmed(t *testing.T) {
	ws := planFixture(t)
	c := startMCPClient(t, ws.rootRepo, testEnv(ws.home), true, nil) // any question fails the test
	args := planArgs(ws, "feature-agent")
	args["confirm"] = true

	if isErr, out := c.callTool("create_workspace", args); isErr || out["created"] != true {
		t.Fatalf("create_workspace confirm=true = %v", out)
	}
	if len(c.asked) != 0 {
		t.Errorf("confirm=true with every input present must not ask the user, asked %d", len(c.asked))
	}
}

func TestE2EMCPElicitationAsksForMissingBaseBranch(t *testing.T) {
	ws := planFixture(t)
	developSHA := addDevelopBranch(t, ws.rootRepo)
	args := planArgs(ws, "feature-base")
	delete(args, "base_branch")

	c := startMCPClient(t, ws.rootRepo, testEnv(ws.home), true, func(r elicitRequest) map[string]any {
		if prop, ok := r.property("base_branch"); ok {
			enum, _ := prop["enum"].([]any)
			got := fmt.Sprint(enum)
			if !strings.Contains(got, "develop") || !strings.Contains(got, "main") {
				t.Errorf("base_branch should offer the repo branches, got %v", enum)
			}
			return accept(map[string]any{"base_branch": "develop"})
		}
		return accept(map[string]any{"confirm": true})
	})

	isErr, out := c.callTool("create_workspace", args)
	if isErr || out["created"] != true {
		t.Fatalf("create_workspace after eliciting base_branch = %v", out)
	}
	if len(c.asked) != 2 {
		t.Fatalf("expected the base branch question and the confirmation, got %d questions", len(c.asked))
	}
	if got := strings.TrimSpace(runGit(t, ws.rootRepo, "rev-parse", "feature/feature-base")); got != developSHA {
		t.Errorf("the root branch must start from the elicited base develop (%s), starts at %s", developSHA, got)
	}
}

func TestE2EMCPElicitationAsksForPackagesAndTheirBase(t *testing.T) {
	ws := planFixture(t)
	addDevelopBranch(t, ws.rootRepo)
	pkgDevelop := addDevelopBranch(t, filepath.Join(ws.scope, "core-pkg"))
	args := map[string]any{"name": "feature-pk", "root_repo": "root-app", "scope": ws.scope}

	var sawPackages, sawPackageBase bool
	c := startMCPClient(t, ws.rootRepo, testEnv(ws.home), true, func(r elicitRequest) map[string]any {
		if pkgProp, ok := r.property("packages"); ok {
			sawPackages = true
			items, _ := pkgProp["items"].(map[string]any)
			if enum := fmt.Sprint(items["enum"]); !strings.Contains(enum, "core-pkg") || strings.Contains(enum, "root-app") {
				t.Errorf("packages must offer the discovered repos except the root, got %v", items["enum"])
			}
			return accept(map[string]any{"base_branch": "main", "packages": []any{"core-pkg"}})
		}
		if _, ok := r.property("core-pkg"); ok {
			sawPackageBase = true
			return accept(map[string]any{"core-pkg": "develop"})
		}
		return accept(map[string]any{"confirm": true})
	})

	isErr, out := c.callTool("create_workspace", args)
	if isErr || out["created"] != true {
		t.Fatalf("create_workspace = %v", out)
	}
	if !sawPackages || !sawPackageBase {
		t.Fatalf("expected a packages question and a package base question (packages=%v, base=%v)", sawPackages, sawPackageBase)
	}
	if got := strings.TrimSpace(runGit(t, filepath.Join(ws.scope, "core-pkg"), "rev-parse", "feature/feature-pk")); got != pkgDevelop {
		t.Errorf("the package branch must start from the elicited develop (%s), starts at %s", pkgDevelop, got)
	}
}

func TestE2EMCPElicitationDeclinedInputsStopPlanning(t *testing.T) {
	ws := planFixture(t)
	args := planArgs(ws, "feature-stop")
	delete(args, "base_branch")
	c := startMCPClient(t, ws.rootRepo, testEnv(ws.home), true, func(elicitRequest) map[string]any { return declineAnswer })

	isErr, out := c.callTool("plan_create", args)
	if isErr || out["user_declined"] != true {
		t.Fatalf("a declined input question must return a declined result, got %v", out)
	}
	assertNothingCreated(t, ws, workspaceContainer(ws, "feature-stop"))
}

// ── complete_worktree ────────────────────────────────────────────────────────

func TestE2EMCPCompleteWorktreeAsksFirst(t *testing.T) {
	ws := createWorkspace(t, "feature-done", []string{"core-pkg"}, "")
	env := testEnv(ws.home)

	c := startMCPClient(t, t.TempDir(), env, true, confirmScript(declineAnswer))
	isErr, out := c.callTool("complete_worktree", map[string]any{"name": ws.name})
	if isErr || out["user_declined"] != true || out["completed"] != false {
		t.Fatalf("declined complete_worktree = %v", out)
	}
	if len(c.asked) != 1 {
		t.Fatalf("expected one question, got %d", len(c.asked))
	}
	msg := c.asked[0].Message
	for _, want := range []string{ws.rootWT, ws.pkgWTs["core-pkg"], "feature/feature-done", "branches are kept"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the question must list %q, got:\n%s", want, msg)
		}
	}
	if !exists(ws.rootWT) || !exists(ws.pkgWTs["core-pkg"]) {
		t.Fatal("a declined complete_worktree must keep the worktrees")
	}
	if rows := listManaged(t, ws.home); len(rows) != 1 {
		t.Fatalf("a declined complete_worktree must keep the registry entry: %+v", rows)
	}

	// Accepting removes them, even without confirm from the agent; confirm=true
	// does not skip the question for a destructive action.
	c2 := startMCPClient(t, t.TempDir(), env, true, confirmScript(accept(map[string]any{"confirm": true})))
	isErr, out = c2.callTool("complete_worktree", map[string]any{"name": ws.name, "confirm": true})
	if isErr {
		t.Fatalf("accepted complete_worktree failed: %v", out)
	}
	if len(c2.asked) != 1 {
		t.Errorf("complete_worktree must always ask a client that can elicit, asked %d", len(c2.asked))
	}
	if exists(ws.rootWT) || exists(ws.pkgWTs["core-pkg"]) {
		t.Error("an accepted complete_worktree must remove the worktrees")
	}
	if rows := listManaged(t, ws.home); len(rows) != 0 {
		t.Errorf("registry after accepted complete_worktree = %+v", rows)
	}
}

// ── clients without elicitation ──────────────────────────────────────────────

func TestE2EMCPWithoutElicitationCreateReturnsNeedsUserInput(t *testing.T) {
	ws := planFixture(t)
	c := startMCPClient(t, ws.rootRepo, testEnv(ws.home), false, nil) // any elicitation request fails the test

	isErr, out := c.callTool("create_workspace", planArgs(ws, "feature-fallback"))
	if isErr {
		t.Fatalf("needs_user_input is a normal result, not an error: %v", out)
	}
	if out["needs_user_input"] != true || out["created"] != false {
		t.Fatalf("expected needs_user_input and created=false, got %v", out)
	}
	questions := asSlice(t, out["questions"])
	if len(questions) == 0 {
		t.Fatalf("questions must tell the agent what to ask: %v", out)
	}
	q := asMap(t, questions[0])
	if q["field"] != "confirm" || q["type"] != "boolean" || !strings.Contains(fmt.Sprint(q["prompt"]), "feature-fallback") {
		t.Errorf("question = %v", q)
	}
	if instruction, _ := out["instruction"].(string); !strings.Contains(instruction, "confirm=true") || !strings.Contains(instruction, "ask") {
		t.Errorf("instruction must tell the agent to ask the user and call again with confirm=true, got %q", instruction)
	}
	if _, ok := out["plan"]; !ok {
		t.Error("the plan must still be returned to show the user")
	}
	assertNothingCreated(t, ws, workspaceContainer(ws, "feature-fallback"))

	args := planArgs(ws, "feature-fallback")
	args["confirm"] = true
	if isErr, out := c.callTool("create_workspace", args); isErr || out["created"] != true {
		t.Fatalf("create_workspace confirm=true without elicitation = %v", out)
	}
	if rows := listManaged(t, ws.home); len(rows) != 1 || rows[0].Name != "feature-fallback" {
		t.Errorf("registry = %+v", rows)
	}
}

func TestE2EMCPWithoutElicitationCompleteNeedsConfirm(t *testing.T) {
	ws := createWorkspace(t, "feature-keep", nil, "")
	c := startMCPClient(t, t.TempDir(), testEnv(ws.home), false, nil)

	for _, args := range []map[string]any{
		{"name": ws.name},
		{"name": ws.name, "force": true},
	} {
		isErr, out := c.callTool("complete_worktree", args)
		if isErr || out["needs_user_input"] != true || out["completed"] != false {
			t.Fatalf("complete_worktree %v without confirm = %v", args, out)
		}
		if len(asSlice(t, out["questions"])) == 0 {
			t.Errorf("questions missing: %v", out)
		}
		if !exists(ws.rootWT) {
			t.Fatal("nothing may be removed without confirmation")
		}
	}

	if isErr, out := c.callTool("complete_worktree", map[string]any{"name": ws.name, "confirm": true}); isErr {
		t.Fatalf("complete_worktree confirm=true failed: %v", out)
	}
	if exists(ws.rootWT) {
		t.Error("complete_worktree confirm=true must remove the worktree")
	}
}

func TestE2EMCPWithoutElicitationPubgetForceNeedsConfirm(t *testing.T) {
	ws := createWorkspace(t, "feature-pub", nil, "")
	c := startMCPClient(t, t.TempDir(), testEnv(ws.home), false, nil)

	isErr, out := c.callTool("pubget", map[string]any{"name": ws.name, "force": true})
	if isErr || out["needs_user_input"] != true || out["executed"] != false {
		t.Fatalf("pubget force=true without confirm = %v", out)
	}
}

func TestE2EMCPWithoutElicitationMissingInputsStayInputErrors(t *testing.T) {
	ws := planFixture(t)
	args := planArgs(ws, "feature-miss")
	delete(args, "base_branch")
	c := startMCPClient(t, ws.rootRepo, testEnv(ws.home), false, nil)

	raw := c.request("tools/call", map[string]any{"name": "plan_create", "arguments": args})
	var res mcpToolResult
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(res.text(t), "base_branch") || !strings.Contains(res.text(t), `"hint"`) {
		t.Errorf("a missing base_branch without elicitation must be an input error with a hint, got %s", res.text(t))
	}
}

// ── prompts and instructions ─────────────────────────────────────────────────

func TestE2EMCPPromptsListAndGet(t *testing.T) {
	cwd, env := t.TempDir(), testEnv(t.TempDir())

	var list struct {
		Prompts []struct {
			Name      string `json:"name"`
			Arguments []struct {
				Name     string `json:"name"`
				Required bool   `json:"required"`
			} `json:"arguments"`
		} `json:"prompts"`
	}
	if err := json.Unmarshal(mcpRPC(t, cwd, env, "prompts/list", map[string]any{}), &list); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, p := range list.Prompts {
		got[p.Name] = true
		for _, a := range p.Arguments {
			if a.Name == "workspace" && !a.Required {
				t.Errorf("prompt %s: workspace must be required", p.Name)
			}
		}
	}
	for _, want := range []string{"create_workspace_flow", "add_package_flow", "finish_workspace_flow"} {
		if !got[want] {
			t.Errorf("prompts/list is missing %s: %v", want, got)
		}
	}
	if len(list.Prompts) != 3 {
		t.Errorf("expected exactly 3 prompts, got %d", len(list.Prompts))
	}

	promptText := func(name string, args map[string]string) string {
		raw := mcpRPC(t, cwd, env, "prompts/get", map[string]any{"name": name, "arguments": args})
		var res struct {
			Messages []struct {
				Role    string `json:"role"`
				Content struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(raw, &res); err != nil || len(res.Messages) == 0 {
			t.Fatalf("prompts/get %s: bad result (%v): %s", name, err, raw)
		}
		var text string
		for _, m := range res.Messages {
			text += m.Content.Text
		}
		if strings.TrimSpace(text) == "" {
			t.Fatalf("prompts/get %s returned empty messages", name)
		}
		return text
	}

	create := promptText("create_workspace_flow", map[string]string{"name": "feature-x", "root_path": "/code/app"})
	for _, want := range []string{"plan_create", "create_workspace", "confirm=true", "discover_repos", "list_branches", "workspace_status", "pubget", "feature-x", "/code/app"} {
		if !strings.Contains(create, want) {
			t.Errorf("create_workspace_flow must mention %q", want)
		}
	}
	if bare := promptText("create_workspace_flow", nil); !strings.Contains(bare, "plan_create") {
		t.Error("create_workspace_flow works without arguments")
	}
	add := promptText("add_package_flow", map[string]string{"workspace": "feature-x"})
	for _, want := range []string{"add_repo", "workspace_name", "package_base", "workspace_status", "feature-x"} {
		if !strings.Contains(add, want) {
			t.Errorf("add_package_flow must mention %q", want)
		}
	}
	finish := promptText("finish_workspace_flow", map[string]string{"workspace": "feature-x"})
	for _, want := range []string{"complete_worktree", "workspace_status", "dirty", "confirm=true", "feature-x"} {
		if !strings.Contains(finish, want) {
			t.Errorf("finish_workspace_flow must mention %q", want)
		}
	}

	// A required argument that is missing is a JSON-RPC error.
	out := runMCPServe(t, buildCLI(t), cwd, env, mcpInitialize+`{"jsonrpc":"2.0","id":2,"method":"prompts/get","params":{"name":"finish_workspace_flow"}}`+"\n")
	if !strings.Contains(out.stdout, `"error"`) {
		t.Errorf("finish_workspace_flow without workspace must fail: %s", out.stdout)
	}
}

func TestE2EMCPInstructionsDescribeAskingTheUser(t *testing.T) {
	instructions, caps := mcpInitializeResult(t, t.TempDir(), testEnv(t.TempDir()))
	for _, want := range []string{"needs_user_input", "elicitation", "declined_by_user", "create_workspace_flow", "add_package_flow", "finish_workspace_flow"} {
		if !strings.Contains(instructions, want) {
			t.Errorf("instructions must mention %q", want)
		}
	}
	if _, ok := caps["prompts"]; !ok {
		t.Errorf("prompts capability missing: %v", caps)
	}
}
