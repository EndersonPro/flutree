package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/EndersonPro/flutree/internal/domain"
)

type fakeInspectRunner struct {
	repos     []domain.RepoSummary
	inspected domain.RepoInspection
	branches  domain.BranchList
	status    domain.WorkspaceStatus
	err       error
}

func (f *fakeInspectRunner) DiscoverRepos(string) ([]domain.RepoSummary, error) {
	return f.repos, f.err
}
func (f *fakeInspectRunner) InspectRepo(string) (domain.RepoInspection, error) {
	return f.inspected, f.err
}
func (f *fakeInspectRunner) ListBranches(string) (domain.BranchList, error) {
	return f.branches, f.err
}
func (f *fakeInspectRunner) WorkspaceStatus(name string) (domain.WorkspaceStatus, error) {
	s := f.status
	s.Name = name
	return s, f.err
}

func callJSON(t *testing.T, h toolHandler, args map[string]any) (map[string]any, *mcplib.CallToolResult) {
	t.Helper()
	res, err := h(context.Background(), makeRequest(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out map[string]any
	if !res.IsError {
		if jsonErr := json.Unmarshal([]byte(extractTextContent(t, res.Content)), &out); jsonErr != nil {
			t.Fatalf("result is not a JSON object: %v", jsonErr)
		}
		if res.StructuredContent == nil {
			t.Fatal("expected structuredContent")
		}
	}
	return out, res
}

func TestDiscoverRepos_ReturnsStructuredRepos(t *testing.T) {
	svc := MCPServices{Inspect: &fakeInspectRunner{repos: []domain.RepoSummary{{Name: "app", PackageName: "app", RepoRoot: "/r/app", CurrentBranch: "main"}}}}
	out, _ := callJSON(t, makeDiscoverReposHandler(svc), map[string]any{"scope": "/r"})
	if repos, _ := out["repos"].([]any); len(repos) != 1 {
		t.Fatalf("repos = %v", out)
	}
}

func TestDiscoverRepos_MissingScopeAndServiceError(t *testing.T) {
	svc := MCPServices{Inspect: &fakeInspectRunner{err: domain.NewError(domain.CategoryPrecondition, 3, "no repos", "look elsewhere", nil)}}
	_, res := callJSON(t, makeDiscoverReposHandler(svc), nil)
	if !res.IsError || !strings.Contains(extractTextContent(t, res.Content), `"category":"input"`) {
		t.Fatalf("missing scope must be an input error: %+v", res)
	}
	_, res = callJSON(t, makeDiscoverReposHandler(svc), map[string]any{"scope": "/r"})
	text := extractTextContent(t, res.Content)
	if !res.IsError || !strings.Contains(text, "precondition") || !strings.Contains(text, "look elsewhere") {
		t.Fatalf("service error must keep category and hint: %s", text)
	}
}

func TestInspectAndBranchesToolsRequireTheirPath(t *testing.T) {
	svc := MCPServices{Inspect: &fakeInspectRunner{}}
	for name, h := range map[string]toolHandler{"inspect_repo": makeInspectRepoHandler(svc), "list_branches": makeListBranchesHandler(svc)} {
		_, res := callJSON(t, h, nil)
		if !res.IsError {
			t.Errorf("%s without a path must fail", name)
		}
	}
	out, _ := callJSON(t, makeInspectRepoHandler(svc), map[string]any{"path": "/r"})
	if _, ok := out["path_dependencies"]; !ok {
		t.Errorf("inspect_repo result lacks path_dependencies: %v", out)
	}
}

func TestPlanCreate_NeverApplies(t *testing.T) {
	fake := &fakeCreateRunner{}
	svc := MCPServices{Create: fake, Config: &fakeConfigRunner{}}
	out, _ := callJSON(t, makePlanCreateHandler(svc), map[string]any{
		"name": "w", "root_repo": "/r/app", "base_branch": "main",
		"packages": []string{"/r/core"}, "package_base": map[string]string{"core": "main"},
	})
	if out["name"] != "w" {
		t.Fatalf("plan = %v", out)
	}
	if fake.applied {
		t.Fatal("plan_create must not call Apply")
	}
}

func TestPlanCreate_AsksForMissingDecisions(t *testing.T) {
	svc := MCPServices{Create: &fakeCreateRunner{}, Config: &fakeConfigRunner{}}
	h := makePlanCreateHandler(svc)
	cases := map[string]map[string]any{
		"name":         {"root_repo": "/r/app", "base_branch": "main"},
		"root_repo":    {"name": "w", "base_branch": "main"},
		"base_branch":  {"name": "w", "root_repo": "/r/app"},
		"package base": {"name": "w", "root_repo": "/r/app", "base_branch": "main", "packages": []string{"/r/core"}},
		"scope":        {"name": "w", "root_repo": "app", "base_branch": "main"},
	}
	for label, args := range cases {
		_, res := callJSON(t, h, args)
		text := extractTextContent(t, res.Content)
		if !res.IsError || !strings.Contains(text, `"category":"input"`) || !strings.Contains(text, `"hint"`) {
			t.Errorf("%s: expected input error with hint, got %s", label, text)
		}
	}
}

func TestPlanCreate_UsesConfiguredScopeAndWorkspaceDefault(t *testing.T) {
	fake := &fakeCreateRunner{}
	svc := MCPServices{Create: fake, Config: &fakeConfigRunner{getVal: "/configured"}}
	callJSON(t, makePlanCreateHandler(svc), map[string]any{"name": "w", "root_repo": "app", "base_branch": "main"})
	if fake.capturedScope != "/configured" {
		t.Errorf("scope = %q, want configured scope.root", fake.capturedScope)
	}
}

func TestCreateWorkspace_RequiresConfirm(t *testing.T) {
	fake := &fakeCreateRunner{}
	svc := MCPServices{Create: fake, Config: &fakeConfigRunner{}}
	h := makeCreateWorkspaceHandler(svc)
	args := map[string]any{"name": "w", "root_repo": "/r/app", "base_branch": "main"}

	out, _ := callJSON(t, h, args)
	if out["created"] != false || out["status"] != "confirmation_required" || fake.applied {
		t.Fatalf("without confirm: %v applied=%v", out, fake.applied)
	}

	args["confirm"] = true
	out, _ = callJSON(t, h, args)
	if out["created"] != true || !fake.applied {
		t.Fatalf("with confirm: %v applied=%v", out, fake.applied)
	}
}

func TestCreateWorkspace_ApplyErrorIsToolError(t *testing.T) {
	svc := MCPServices{Create: &fakeCreateRunner{applyErr: errors.New("boom")}, Config: &fakeConfigRunner{}}
	_, res := callJSON(t, makeCreateWorkspaceHandler(svc), map[string]any{"name": "w", "root_repo": "/r/app", "base_branch": "main", "confirm": true})
	if !res.IsError || !strings.Contains(extractTextContent(t, res.Content), "boom") {
		t.Fatalf("apply error must surface as a tool error: %+v", res)
	}
}

func TestWorkspaceStatus_PassesNameAndReportsErrors(t *testing.T) {
	svc := MCPServices{Inspect: &fakeInspectRunner{}}
	out, _ := callJSON(t, makeWorkspaceStatusHandler(svc), map[string]any{"name": "w"})
	if out["name"] != "w" {
		t.Fatalf("status = %v", out)
	}
	svc = MCPServices{Inspect: &fakeInspectRunner{err: errors.New("nope")}}
	_, res := callJSON(t, makeWorkspaceStatusHandler(svc), map[string]any{"name": "w"})
	if !res.IsError {
		t.Fatal("service error must be a tool error")
	}
}

func TestBuildServerRegistersInstructionsAndResources(t *testing.T) {
	s, err := BuildServer("0.0.0-test", fullServices())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(serverInstructions, "plan_create") || !strings.Contains(guideMarkdown, "plan_create") {
		t.Error("instructions and guide must describe the recommended flow")
	}
	_ = s
}
