//go:build !windows

package integration_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mcpRPC runs `flutree mcp serve`, performs one JSON-RPC request after the
// handshake and returns its result. A JSON-RPC error fails the test.
func mcpRPC(t *testing.T, cwd string, env []string, method string, params any) json.RawMessage {
	t.Helper()
	body, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	input := mcpInitialize + fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":%q,"params":%s}`, method, body) + "\n"
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
			t.Fatalf("%s returned JSON-RPC error: %s", method, frame.Error)
		}
		return frame.Result
	}
	t.Fatalf("no %s response in stdout:\n%s\nstderr:\n%s", method, res.stdout, res.stderr)
	return nil
}

// mcpInitializeResult returns the result of the handshake itself.
func mcpInitializeResult(t *testing.T, cwd string, env []string) (instructions string, capabilities map[string]any) {
	t.Helper()
	res := runMCPServe(t, buildCLI(t), cwd, env, mcpInitialize)
	for _, line := range strings.Split(strings.TrimSpace(res.stdout), "\n") {
		var frame struct {
			ID     float64 `json:"id"`
			Result struct {
				Instructions string         `json:"instructions"`
				Capabilities map[string]any `json:"capabilities"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(line), &frame); err == nil && frame.ID == 1 {
			return frame.Result.Instructions, frame.Result.Capabilities
		}
	}
	t.Fatalf("no initialize response:\n%s", res.stdout)
	return "", nil
}

// readMCPResource reads one resource and returns its text.
func readMCPResource(t *testing.T, cwd string, env []string, uri string) string {
	t.Helper()
	raw := mcpRPC(t, cwd, env, "resources/read", map[string]any{"uri": uri})
	var out struct {
		Contents []struct {
			URI      string `json:"uri"`
			MIMEType string `json:"mimeType"`
			Text     string `json:"text"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Contents) != 1 {
		t.Fatalf("resources/read %s: bad result (%v): %s", uri, err, raw)
	}
	return out.Contents[0].Text
}

// toolJSON calls a tool, requires success and decodes its JSON text into a map.
func toolJSON(t *testing.T, cwd string, env []string, tool string, args map[string]any) map[string]any {
	t.Helper()
	res := callMCPTool(t, cwd, env, tool, args)
	if res.IsError {
		t.Fatalf("%s failed: %s", tool, res.text(t))
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.text(t)), &out); err != nil {
		t.Fatalf("%s text is not a JSON object: %v\n%s", tool, err, res.text(t))
	}
	return out
}

// toolErrorCategory calls a tool, requires isError and returns the error category.
func toolErrorCategory(t *testing.T, cwd string, env []string, tool string, args map[string]any) string {
	t.Helper()
	res := callMCPTool(t, cwd, env, tool, args)
	if !res.IsError {
		t.Fatalf("%s: expected isError, got %s", tool, res.text(t))
	}
	var payload struct {
		Category string `json:"category"`
		Message  string `json:"message"`
		Hint     string `json:"hint"`
	}
	if err := json.Unmarshal([]byte(res.text(t)), &payload); err != nil || payload.Category == "" || payload.Message == "" {
		t.Fatalf("%s: error is not a category/message payload: %s", tool, res.text(t))
	}
	return payload.Category
}

func asSlice(t *testing.T, v any) []any {
	t.Helper()
	s, ok := v.([]any)
	if !ok {
		t.Fatalf("expected JSON array, got %T (%v)", v, v)
	}
	return s
}

func asMap(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected JSON object, got %T (%v)", v, v)
	}
	return m
}

func TestE2EMCPInitializeAdvertisesInstructionsAndResources(t *testing.T) {
	instructions, caps := mcpInitializeResult(t, t.TempDir(), testEnv(t.TempDir()))
	for _, want := range []string{"plan_create", "create_workspace", "confirm", "pubspec_overrides.yaml", ".git/info/exclude", "sdk.manager", "workspace_status", "complete_worktree", "base branch"} {
		if !strings.Contains(instructions, want) {
			t.Errorf("instructions do not mention %q:\n%s", want, instructions)
		}
	}
	if lines := strings.Count(instructions, "\n"); lines > 100 {
		t.Errorf("instructions should stay concise, got %d lines", lines)
	}
	if _, ok := caps["resources"]; !ok {
		t.Errorf("resources capability missing: %v", caps)
	}
}

func TestE2EMCPListsToolsResourcesAndTemplate(t *testing.T) {
	env := testEnv(t.TempDir())
	cwd := t.TempDir()

	var tools struct {
		Tools []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(mcpRPC(t, cwd, env, "tools/list", map[string]any{}), &tools); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, tool := range tools.Tools {
		got[tool.Name] = tool.Description
	}
	for _, name := range []string{
		"list_worktrees", "create_worktree", "add_repo", "complete_worktree", "pubget", "clean_worktree", "get_config", "set_config", "get_version",
		"discover_repos", "inspect_repo", "list_branches", "plan_create", "create_workspace", "workspace_status",
	} {
		desc, ok := got[name]
		if !ok {
			t.Errorf("tool %q missing from tools/list", name)
		}
		if len(desc) < 40 {
			t.Errorf("tool %q needs a description that says when to use it, got %q", name, desc)
		}
	}
	if len(got) != 15 {
		t.Errorf("expected 15 tools, got %d: %v", len(got), got)
	}
	if !strings.Contains(got["create_worktree"], "create_workspace") {
		t.Errorf("create_worktree must point to create_workspace, got %q", got["create_worktree"])
	}

	var resources struct {
		Resources []struct {
			URI string `json:"uri"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(mcpRPC(t, cwd, env, "resources/list", map[string]any{}), &resources); err != nil {
		t.Fatal(err)
	}
	uris := map[string]bool{}
	for _, r := range resources.Resources {
		uris[r.URI] = true
	}
	for _, want := range []string{"flutree://guide", "flutree://workspaces", "flutree://config"} {
		if !uris[want] {
			t.Errorf("resource %s missing from resources/list: %v", want, uris)
		}
	}

	var templates struct {
		ResourceTemplates []struct {
			URITemplate string `json:"uriTemplate"`
		} `json:"resourceTemplates"`
	}
	if err := json.Unmarshal(mcpRPC(t, cwd, env, "resources/templates/list", map[string]any{}), &templates); err != nil {
		t.Fatal(err)
	}
	if len(templates.ResourceTemplates) != 1 || templates.ResourceTemplates[0].URITemplate != "flutree://workspace/{name}" {
		t.Errorf("unexpected resource templates: %+v", templates.ResourceTemplates)
	}
}

func TestE2EMCPResourcesDescribeAWorkspace(t *testing.T) {
	ws := createWorkspace(t, "feature-res", []string{"core-pkg"}, "")
	env := testEnv(ws.home)
	cwd := t.TempDir()

	guide := readMCPResource(t, cwd, env, "flutree://guide")
	for _, want := range []string{"# ", "plan_create", "pubspec_overrides.yaml", "feature/"} {
		if !strings.Contains(guide, want) {
			t.Errorf("guide does not mention %q", want)
		}
	}

	var rows []listedRecord
	if err := json.Unmarshal([]byte(readMCPResource(t, cwd, env, "flutree://workspaces")), &rows); err != nil {
		t.Fatalf("flutree://workspaces is not the list JSON: %v", err)
	}
	if len(rows) != 1 || rows[0].Name != ws.name || rows[0].PackageCount != 1 {
		t.Fatalf("flutree://workspaces = %+v", rows)
	}

	var status map[string]any
	if err := json.Unmarshal([]byte(readMCPResource(t, cwd, env, "flutree://workspace/"+ws.name)), &status); err != nil {
		t.Fatalf("workspace resource is not JSON: %v", err)
	}
	if status["name"] != ws.name || status["branch"] != "feature/feature-res" {
		t.Errorf("unexpected workspace header: %v", status)
	}
	root := asMap(t, status["root"])
	if root["path"] != ws.rootWT || root["branch"] != "feature/feature-res" || root["repo_root"] != ws.rootRepo || root["dirty"] != false {
		t.Errorf("unexpected root worktree: %v", root)
	}
	pkgs := asSlice(t, status["packages"])
	if len(pkgs) != 1 || asMap(t, pkgs[0])["name"] != "core-pkg" || asMap(t, pkgs[0])["path"] != ws.pkgWTs["core-pkg"] {
		t.Errorf("unexpected packages: %v", pkgs)
	}
	overrides := asMap(t, status["pubspec_overrides"])
	if overrides["exists"] != true {
		t.Errorf("pubspec_overrides should exist: %v", overrides)
	}
	entry := asMap(t, asMap(t, overrides["entries"])["core_pkg"])
	if entry["path"] != "../../packages/core-pkg" {
		t.Errorf("override for core_pkg = %v, want the package worktree relative to the root worktree", entry)
	}
	if file, _ := status["workspace_file"].(string); !strings.HasSuffix(file, "feature-res.code-workspace") {
		t.Errorf("workspace_file = %v", status["workspace_file"])
	}

	// An unknown workspace is a JSON-RPC error, not an empty document.
	if out := runMCPServe(t, buildCLI(t), cwd, env, mcpInitialize+`{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"flutree://workspace/ghost"}}`+"\n"); !strings.Contains(out.stdout, `"error"`) {
		t.Errorf("reading an unknown workspace should fail: %s", out.stdout)
	}
}

func TestE2EMCPConfigResourceShowsEffectiveValues(t *testing.T) {
	home := t.TempDir()
	scope := t.TempDir()
	env := testEnv(home)
	runE2E(t, home, env, "config", "set", "scope.root", scope).requireCode(t, 0)
	runE2E(t, home, env, "config", "set", "sdk.manager", "fvm").requireCode(t, 0)

	var cfg map[string]string
	if err := json.Unmarshal([]byte(readMCPResource(t, home, env, "flutree://config")), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["scope.root"] != scope || cfg["sdk.manager"] != "fvm" || cfg["workspace.generate"] != "true" {
		t.Errorf("flutree://config = %v", cfg)
	}
}

func TestE2EMCPDiscoverReposAndInspectAndBranches(t *testing.T) {
	ws := planFixture(t)
	writeFile(t, filepath.Join(ws.rootRepo, "pubspec.yaml"), "name: root_app\ndependencies:\n  core_pkg:\n    path: ../core-pkg\n  http: ^1.0.0\ndev_dependencies:\n  flutter_test:\n    sdk: flutter\n")
	env := testEnv(ws.home)
	cwd := t.TempDir()

	found := toolJSON(t, cwd, env, "discover_repos", map[string]any{"scope": ws.scope})
	repos := asSlice(t, found["repos"])
	names := map[string]map[string]any{}
	for _, r := range repos {
		m := asMap(t, r)
		names[m["name"].(string)] = m
	}
	if len(names) != 2 || names["root-app"]["package_name"] != "root_app" || names["root-app"]["repo_root"] != ws.rootRepo || names["root-app"]["current_branch"] != "main" {
		t.Errorf("discover_repos = %v", found)
	}
	if got := toolErrorCategory(t, cwd, env, "discover_repos", map[string]any{"scope": filepath.Join(ws.home, "nope")}); got != "precondition" {
		t.Errorf("discover_repos error category = %q", got)
	}

	writeFile(t, filepath.Join(ws.rootRepo, ".fvmrc"), `{"flutter":"3.24.0"}`)
	runGit(t, ws.rootRepo, "branch", "develop")
	insp := toolJSON(t, cwd, env, "inspect_repo", map[string]any{"path": ws.rootRepo})
	if insp["is_flutter"] != true || insp["package_name"] != "root_app" || insp["repo_root"] != ws.rootRepo || insp["current_branch"] != "main" {
		t.Errorf("inspect_repo = %v", insp)
	}
	if pin := asMap(t, insp["sdk_pin"]); pin["fvm_version"] != "3.24.0" {
		t.Errorf("sdk_pin = %v", pin)
	}
	deps := asMap(t, insp["path_dependencies"])
	if len(deps) != 1 || deps["core_pkg"] != "../core-pkg" {
		t.Errorf("path_dependencies = %v (hosted dependencies must not appear)", deps)
	}
	if !strings.Contains(fmt.Sprint(insp["local_branches"]), "develop") {
		t.Errorf("local_branches = %v", insp["local_branches"])
	}

	plain := t.TempDir()
	notFlutter := toolJSON(t, cwd, env, "inspect_repo", map[string]any{"path": plain})
	if notFlutter["is_flutter"] != false || notFlutter["reason"] == "" {
		t.Errorf("inspect_repo on a plain folder = %v", notFlutter)
	}
	if got := toolErrorCategory(t, cwd, env, "inspect_repo", map[string]any{"path": filepath.Join(plain, "missing")}); got != "input" {
		t.Errorf("inspect_repo missing path category = %q", got)
	}

	br := toolJSON(t, cwd, env, "list_branches", map[string]any{"repo_path": ws.rootRepo})
	local, remote := fmt.Sprint(br["local"]), fmt.Sprint(br["remote"])
	if br["current"] != "main" || !strings.Contains(local, "develop") || !strings.Contains(local, "main") || !strings.Contains(remote, "origin/main") || strings.Contains(remote, "HEAD") {
		t.Errorf("list_branches = %v", br)
	}
	if got := toolErrorCategory(t, cwd, env, "list_branches", map[string]any{"repo_path": plain}); got == "" {
		t.Error("list_branches on a non-git folder must fail")
	}
}

func planArgs(ws e2eWorkspace, name string) map[string]any {
	return map[string]any{
		"name":         name,
		"root_repo":    "root-app",
		"packages":     []string{"core-pkg"},
		"base_branch":  "main",
		"package_base": map[string]string{"core-pkg": "main"},
		"scope":        ws.scope,
	}
}

// planFixture is a scope with a root repo and one package, without a workspace.
func planFixture(t *testing.T) e2eWorkspace {
	t.Helper()
	ws := e2eWorkspace{home: t.TempDir(), scope: filepath.Join(t.TempDir(), "scope"), pkgWTs: map[string]string{}}
	ws.rootRepo = filepath.Join(ws.scope, "root-app")
	initRepoWithPackageName(t, ws.rootRepo, "root_app")
	initRepoWithPackageName(t, filepath.Join(ws.scope, "core-pkg"), "core_pkg")
	return ws
}

// assertNothingCreated checks that no workspace folder, registry file or
// worktree exists. (Reading the config may create the default config file, as
// every flutree command does; that is not a workspace side effect.)
func assertNothingCreated(t *testing.T, ws e2eWorkspace, container string) {
	t.Helper()
	if exists(container) {
		t.Errorf("workspace folder %s must not exist", container)
	}
	if exists(filepath.Join(ws.home, "Documents", "worktrees", ".worktrees_registry.json")) {
		t.Error("the registry must not be written")
	}
	if out := runGit(t, ws.rootRepo, "worktree", "list", "--porcelain"); strings.Count(out, "worktree ") != 1 {
		t.Errorf("no worktree may be added to the root repo:\n%s", out)
	}
}

func TestE2EMCPPlanCreateHasNoSideEffects(t *testing.T) {
	ws := planFixture(t)
	env := testEnv(ws.home)
	args := planArgs(ws, "feature-plan")

	plan := toolJSON(t, ws.rootRepo, env, "plan_create", args)
	container := filepath.Join(ws.home, "Documents", "worktrees", "feature-plan")
	if plan["container_path"] != container {
		t.Errorf("container_path = %v, want %s", plan["container_path"], container)
	}
	root := asMap(t, plan["root"])
	if root["path"] != filepath.Join(container, "root", "root-app") || root["branch"] != "feature/feature-plan" || root["base_branch"] != "main" {
		t.Errorf("root plan = %v", root)
	}
	pkgs := asSlice(t, plan["packages"])
	if len(pkgs) != 1 || asMap(t, pkgs[0])["path"] != filepath.Join(container, "packages", "core-pkg") || asMap(t, pkgs[0])["base_branch"] != "main" {
		t.Errorf("package plan = %v", pkgs)
	}
	if override, _ := plan["override_content"].(string); !strings.Contains(override, "core_pkg") || !strings.Contains(override, "../../packages/core-pkg") {
		t.Errorf("override_content = %v", plan["override_content"])
	}
	if file, _ := plan["workspace_file"].(string); !strings.HasSuffix(file, "feature-plan.code-workspace") {
		t.Errorf("workspace_file = %v (generation defaults to on)", plan["workspace_file"])
	}

	assertNothingCreated(t, ws, container)
	if out := runGit(t, ws.rootRepo, "branch", "--list", "feature/*"); strings.TrimSpace(out) != "" {
		t.Errorf("plan_create created a branch: %s", out)
	}
	if rows := listManaged(t, ws.home); len(rows) != 0 {
		t.Errorf("plan_create registered a workspace: %+v", rows)
	}
}

func TestE2EMCPPlanCreateAcceptsPathSelectorsWithoutScope(t *testing.T) {
	ws := planFixture(t)
	env := testEnv(ws.home)
	plan := toolJSON(t, t.TempDir(), env, "plan_create", map[string]any{
		"name":         "feature-paths",
		"root_repo":    ws.rootRepo,
		"packages":     []string{filepath.Join(ws.scope, "core-pkg")},
		"base_branch":  "main",
		"package_base": map[string]string{"core-pkg": "main"},
	})
	if pkgs := asSlice(t, plan["packages"]); len(pkgs) != 1 {
		t.Errorf("plan = %v", plan)
	}
}

func TestE2EMCPPlanCreateErrors(t *testing.T) {
	ws := planFixture(t)
	env := testEnv(ws.home)

	missingBase := planArgs(ws, "feature-err")
	delete(missingBase, "package_base")
	res := callMCPTool(t, ws.rootRepo, env, "plan_create", missingBase)
	if !res.IsError || !strings.Contains(res.text(t), "core-pkg") || !strings.Contains(res.text(t), `"category":"input"`) {
		t.Errorf("a package without base branch must be an input error naming it, got %+v", res)
	}

	badBase := planArgs(ws, "feature-err")
	badBase["package_base"] = map[string]string{"core-pkg": "nope"}
	if got := toolErrorCategory(t, ws.rootRepo, env, "plan_create", badBase); got != "input" {
		t.Errorf("unknown package base category = %q", got)
	}

	noScope := planArgs(ws, "feature-err")
	delete(noScope, "scope")
	if got := toolErrorCategory(t, ws.rootRepo, env, "plan_create", noScope); got != "input" {
		t.Errorf("a name selector without scope category = %q", got)
	}
}

func TestE2EMCPCreateWorkspaceRequiresConfirmation(t *testing.T) {
	ws := planFixture(t)
	env := testEnv(ws.home)
	args := planArgs(ws, "feature-confirm")

	refused := toolJSON(t, ws.rootRepo, env, "create_workspace", args)
	if refused["created"] != false || refused["status"] != "confirmation_required" {
		t.Errorf("without confirm the result must say it was not created: %v", refused)
	}
	if _, ok := refused["plan"]; !ok {
		t.Errorf("the refusal should carry the plan to show the user: %v", refused)
	}
	assertNothingCreated(t, ws, filepath.Join(ws.home, "Documents", "worktrees", "feature-confirm"))
	if out := runGit(t, ws.rootRepo, "branch", "--list", "feature/*"); strings.TrimSpace(out) != "" {
		t.Fatalf("create_workspace without confirm created a branch: %s", out)
	}

	explicitFalse := planArgs(ws, "feature-confirm")
	explicitFalse["confirm"] = false
	if toolJSON(t, ws.rootRepo, env, "create_workspace", explicitFalse)["created"] != false {
		t.Error("confirm=false must not create")
	}

	args["confirm"] = true
	created := toolJSON(t, ws.rootRepo, env, "create_workspace", args)
	if created["created"] != true {
		t.Fatalf("create_workspace with confirm = %v", created)
	}
	if rec := asMap(t, asMap(t, created["result"])["record"]); rec["name"] != "feature-confirm" {
		t.Errorf("result record = %v", rec)
	}
	if rows := listManaged(t, ws.home); len(rows) != 1 || rows[0].Name != "feature-confirm" || rows[0].PackageCount != 1 {
		t.Errorf("after confirm, list = %+v", rows)
	}
	container := filepath.Join(ws.home, "Documents", "worktrees", "feature-confirm")
	if !exists(filepath.Join(container, "root", "root-app", "pubspec_overrides.yaml")) || !exists(filepath.Join(container, "packages", "core-pkg")) {
		t.Error("confirmed create_workspace did not build the worktrees")
	}
}

func TestE2EMCPWorkspaceStatusReportsDirtyWorktrees(t *testing.T) {
	ws := createWorkspace(t, "feature-status", []string{"core-pkg"}, "")
	env := testEnv(ws.home)
	cwd := t.TempDir()

	status := toolJSON(t, cwd, env, "workspace_status", map[string]any{"name": ws.name})
	if root := asMap(t, status["root"]); root["dirty"] != false || root["exists"] != true || root["branch"] != "feature/feature-status" {
		t.Errorf("clean root = %v", root)
	}
	if overrides := asMap(t, status["pubspec_overrides"]); overrides["exists"] != true {
		t.Errorf("pubspec_overrides = %v", overrides)
	}

	writeFile(t, filepath.Join(ws.pkgWTs["core-pkg"], "lib", "new.dart"), "void main() {}\n")
	status = toolJSON(t, cwd, env, "workspace_status", map[string]any{"name": ws.name})
	pkg := asMap(t, asSlice(t, status["packages"])[0])
	if pkg["dirty"] != true || asMap(t, status["root"])["dirty"] != false {
		t.Errorf("editing a package must make only that worktree dirty: %v", status)
	}

	if err := os.RemoveAll(ws.pkgWTs["core-pkg"]); err != nil {
		t.Fatal(err)
	}
	status = toolJSON(t, cwd, env, "workspace_status", map[string]any{"name": ws.name})
	if pkg := asMap(t, asSlice(t, status["packages"])[0]); pkg["exists"] != false {
		t.Errorf("a removed worktree must report exists=false: %v", pkg)
	}

	if got := toolErrorCategory(t, cwd, env, "workspace_status", map[string]any{"name": "ghost"}); got != "precondition" {
		t.Errorf("unknown workspace category = %q", got)
	}
}

func TestE2EMCPNewToolsReturnStructuredContent(t *testing.T) {
	ws := planFixture(t)
	env := testEnv(ws.home)
	raw := mcpRPC(t, ws.rootRepo, env, "tools/call", map[string]any{"name": "discover_repos", "arguments": map[string]any{"scope": ws.scope}})
	var out struct {
		StructuredContent map[string]any `json:"structuredContent"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.StructuredContent == nil || len(asSlice(t, out.StructuredContent["repos"])) != 2 {
		t.Errorf("discover_repos must return structuredContent, got %s", raw)
	}
}
