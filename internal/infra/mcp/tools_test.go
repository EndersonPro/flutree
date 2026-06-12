package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/EndersonPro/flutree/internal/domain"
)

// ── fake service implementations ────────────────────────────────────────────

type fakeListRunner struct {
	rows []domain.ListRow
	err  error
	// captured args
	capturedNonInteractive bool
}

func (f *fakeListRunner) Run(input domain.ListInput) ([]domain.ListRow, error) {
	return f.rows, f.err
}

type fakeCreateRunner struct {
	planErr                error
	applyErr               error
	result                 domain.CreateResult
	nonInteractiveCaptured bool
}

func (f *fakeCreateRunner) BuildDryPlan(input domain.CreateInput) (domain.CreateDryPlan, error) {
	f.nonInteractiveCaptured = input.NonInteractive
	if f.planErr != nil {
		return domain.CreateDryPlan{}, f.planErr
	}
	return domain.CreateDryPlan{NormalizedName: input.Name}, nil
}

func (f *fakeCreateRunner) Apply(plan domain.CreateDryPlan, opts domain.CreateApplyOptions) (domain.CreateResult, error) {
	if f.applyErr != nil {
		return domain.CreateResult{}, f.applyErr
	}
	r := f.result
	r.Record.Name = plan.NormalizedName
	return r, nil
}

type fakeAddRepoRunner struct {
	result                 domain.AddRepoResult
	err                    error
	nonInteractiveCaptured bool
}

func (f *fakeAddRepoRunner) Run(input domain.AddRepoInput) (domain.AddRepoResult, error) {
	f.nonInteractiveCaptured = input.NonInteractive
	return f.result, f.err
}

type fakeCompleteRunner struct {
	result                 domain.CompleteResult
	err                    error
	nonInteractiveCaptured bool
	yesCaptured            bool
}

func (f *fakeCompleteRunner) Run(input domain.CompleteInput) (domain.CompleteResult, error) {
	f.nonInteractiveCaptured = input.NonInteractive
	f.yesCaptured = input.Yes
	return f.result, f.err
}

type fakePubGetRunner struct {
	result domain.PubGetResult
	err    error
}

func (f *fakePubGetRunner) Run(input domain.PubGetInput) (domain.PubGetResult, error) {
	r := f.result
	r.WorkspaceName = input.Name
	return r, f.err
}

type fakeCleanRunner struct {
	result domain.CleanResult
	err    error
}

func (f *fakeCleanRunner) Run(input domain.CleanInput) (domain.CleanResult, error) {
	return f.result, f.err
}

type fakeConfigRunner struct {
	getVal string
	getErr error
	setVal string
	setErr error
}

func (f *fakeConfigRunner) GetScopeRoot(key string) (string, error) {
	return f.getVal, f.getErr
}

func (f *fakeConfigRunner) SetScopeRoot(key, value string) (string, error) {
	return f.setVal, f.setErr
}

// ── helper: build a CallToolRequest with string arguments ───────────────────

func makeRequest(args map[string]any) mcplib.CallToolRequest {
	raw, _ := json.Marshal(args)
	var params mcplib.CallToolParams
	_ = json.Unmarshal(raw, &params.Arguments)
	return mcplib.CallToolRequest{Params: params}
}

// ── test: list_worktrees ─────────────────────────────────────────────────────

func TestListWorktrees_HappyPath(t *testing.T) {
	svc := MCPServices{
		List: &fakeListRunner{
			rows: []domain.ListRow{
				{Name: "my-workspace", Branch: "main", Status: "active"},
			},
		},
	}
	handler := makeListWorktreesHandler(svc)
	result, err := handler(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", extractTextContent(t, result.Content))
	}
	text := extractTextContent(t, result.Content)
	var rows []domain.ListRow
	if jsonErr := json.Unmarshal([]byte(text), &rows); jsonErr != nil {
		t.Fatalf("result is not JSON array: %v — got: %q", jsonErr, text)
	}
	if len(rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(rows))
	}
}

func TestListWorktrees_EmptyWorkspace(t *testing.T) {
	svc := MCPServices{
		List: &fakeListRunner{rows: []domain.ListRow{}},
	}
	handler := makeListWorktreesHandler(svc)
	result, err := handler(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("empty list should succeed, got error: %s", extractTextContent(t, result.Content))
	}
	text := extractTextContent(t, result.Content)
	if !strings.HasPrefix(strings.TrimSpace(text), "[") {
		t.Errorf("expected JSON array, got: %q", text)
	}
	var rows []domain.ListRow
	if jsonErr := json.Unmarshal([]byte(text), &rows); jsonErr != nil {
		t.Fatalf("result not valid JSON array: %v", jsonErr)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

// ── test: create_worktree ────────────────────────────────────────────────────

func TestCreateWorktree_HappyPath(t *testing.T) {
	fake := &fakeCreateRunner{}
	svc := MCPServices{Create: fake}
	handler := makeCreateWorktreeHandler(svc)
	result, err := handler(context.Background(), makeRequest(map[string]any{
		"name":      "my-wt",
		"root_repo": "/repos/myapp",
		"scope":     "/repos",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", extractTextContent(t, result.Content))
	}
	if !fake.nonInteractiveCaptured {
		t.Error("NonInteractive must be forced to true")
	}
}

func TestCreateWorktree_MissingRootRepo(t *testing.T) {
	svc := MCPServices{Create: &fakeCreateRunner{}}
	handler := makeCreateWorktreeHandler(svc)
	result, err := handler(context.Background(), makeRequest(map[string]any{
		"name":  "my-wt",
		"scope": "/repos",
		// root_repo intentionally absent
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected isError=true for missing root_repo")
	}
	text := extractTextContent(t, result.Content)
	var payload struct {
		Category string `json:"category"`
	}
	if jsonErr := json.Unmarshal([]byte(text), &payload); jsonErr != nil {
		t.Fatalf("error payload not valid JSON: %v", jsonErr)
	}
	if payload.Category != "input" {
		t.Errorf("expected category=input, got %q", payload.Category)
	}
}

// ── test: add_repo ───────────────────────────────────────────────────────────

func TestAddRepo_HappyPath(t *testing.T) {
	fake := &fakeAddRepoRunner{
		result: domain.AddRepoResult{WorkspaceName: "ws", AddedRepos: []string{"repo-a"}},
	}
	svc := MCPServices{AddRepo: fake}
	handler := makeAddRepoHandler(svc)
	result, err := handler(context.Background(), makeRequest(map[string]any{
		"workspace_name": "ws",
		"repos":          []string{"repo-a"},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", extractTextContent(t, result.Content))
	}
	if !fake.nonInteractiveCaptured {
		t.Error("NonInteractive must be forced to true")
	}
}

func TestAddRepo_UnknownWorkspace(t *testing.T) {
	fake := &fakeAddRepoRunner{
		err: domain.NewError(domain.CategoryPrecondition, 3, "workspace not found", "run flutree list", nil),
	}
	svc := MCPServices{AddRepo: fake}
	handler := makeAddRepoHandler(svc)
	result, err := handler(context.Background(), makeRequest(map[string]any{
		"workspace_name": "nonexistent",
		"repos":          []string{"x"},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected isError=true for unknown workspace")
	}
	text := extractTextContent(t, result.Content)
	var payload struct {
		Category string `json:"category"`
	}
	_ = json.Unmarshal([]byte(text), &payload)
	if payload.Category != "precondition" {
		t.Errorf("expected category=precondition, got %q", payload.Category)
	}
}

// ── test: complete_worktree ──────────────────────────────────────────────────

func TestCompleteWorktree_HappyPath(t *testing.T) {
	fake := &fakeCompleteRunner{
		result: domain.CompleteResult{Record: domain.RegistryRecord{Name: "wt1"}},
	}
	svc := MCPServices{Complete: fake}
	handler := makeCompleteWorktreeHandler(svc)
	result, err := handler(context.Background(), makeRequest(map[string]any{"name": "wt1"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", extractTextContent(t, result.Content))
	}
	if !fake.nonInteractiveCaptured {
		t.Error("NonInteractive must be forced to true")
	}
	if !fake.yesCaptured {
		t.Error("Yes must be forced to true")
	}
}

func TestCompleteWorktree_NonExistentName(t *testing.T) {
	fake := &fakeCompleteRunner{
		err: domain.NewError(domain.CategoryPrecondition, 3, "worktree not found", "", nil),
	}
	svc := MCPServices{Complete: fake}
	handler := makeCompleteWorktreeHandler(svc)
	result, err := handler(context.Background(), makeRequest(map[string]any{"name": "ghost"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected isError=true for non-existent worktree")
	}
	text := extractTextContent(t, result.Content)
	var payload struct {
		Category string `json:"category"`
	}
	_ = json.Unmarshal([]byte(text), &payload)
	if payload.Category != "precondition" {
		t.Errorf("expected category=precondition, got %q", payload.Category)
	}
}

// ── test: pubget ─────────────────────────────────────────────────────────────

func TestPubGet_HappyPath(t *testing.T) {
	svc := MCPServices{
		PubGet: &fakePubGetRunner{
			result: domain.PubGetResult{},
		},
	}
	handler := makePubGetHandler(svc)
	result, err := handler(context.Background(), makeRequest(map[string]any{"name": "my-wt"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", extractTextContent(t, result.Content))
	}
	text := extractTextContent(t, result.Content)
	var pr domain.PubGetResult
	if jsonErr := json.Unmarshal([]byte(text), &pr); jsonErr != nil {
		t.Fatalf("result not valid JSON: %v", jsonErr)
	}
	if pr.WorkspaceName != "my-wt" {
		t.Errorf("expected workspace_name=my-wt, got %q", pr.WorkspaceName)
	}
}

// ── test: clean_worktree ─────────────────────────────────────────────────────

func TestCleanWorktree_HappyPath(t *testing.T) {
	svc := MCPServices{
		Clean: &fakeCleanRunner{
			result: domain.CleanResult{Record: domain.RegistryRecord{Name: "stale"}},
		},
	}
	handler := makeCleanWorktreeHandler(svc)
	result, err := handler(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", extractTextContent(t, result.Content))
	}
	text := extractTextContent(t, result.Content)
	var cr domain.CleanResult
	if jsonErr := json.Unmarshal([]byte(text), &cr); jsonErr != nil {
		t.Fatalf("result not valid JSON: %v", jsonErr)
	}
}

// ── test: get_config ─────────────────────────────────────────────────────────

func TestGetConfig_KnownKey(t *testing.T) {
	svc := MCPServices{
		Config: &fakeConfigRunner{getVal: "/workspaces"},
	}
	handler := makeGetConfigHandler(svc)
	result, err := handler(context.Background(), makeRequest(map[string]any{"key": "scope.root"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", extractTextContent(t, result.Content))
	}
	text := extractTextContent(t, result.Content)
	var m map[string]string
	if jsonErr := json.Unmarshal([]byte(text), &m); jsonErr != nil {
		t.Fatalf("result not valid JSON: %v", jsonErr)
	}
	if m["scope.root"] != "/workspaces" {
		t.Errorf("expected scope.root=/workspaces, got %q", m["scope.root"])
	}
}

func TestGetConfig_UnknownKey(t *testing.T) {
	svc := MCPServices{
		Config: &fakeConfigRunner{
			getErr: domain.NewError(domain.CategoryInput, 2, "unsupported key", "use scope.root", nil),
		},
	}
	handler := makeGetConfigHandler(svc)
	result, err := handler(context.Background(), makeRequest(map[string]any{"key": "scope.unknown"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected isError=true for unknown key")
	}
	text := extractTextContent(t, result.Content)
	var payload struct {
		Category string `json:"category"`
	}
	_ = json.Unmarshal([]byte(text), &payload)
	if payload.Category != "input" {
		t.Errorf("expected category=input, got %q", payload.Category)
	}
}

// ── test: set_config ─────────────────────────────────────────────────────────

func TestSetConfig_Success(t *testing.T) {
	svc := MCPServices{
		Config: &fakeConfigRunner{setVal: "/new-path"},
	}
	handler := makeSetConfigHandler(svc)
	result, err := handler(context.Background(), makeRequest(map[string]any{
		"key":   "scope.root",
		"value": "/new-path",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", extractTextContent(t, result.Content))
	}
	text := extractTextContent(t, result.Content)
	var m map[string]string
	if jsonErr := json.Unmarshal([]byte(text), &m); jsonErr != nil {
		t.Fatalf("result not valid JSON: %v", jsonErr)
	}
	if m["scope.root"] != "/new-path" {
		t.Errorf("expected scope.root=/new-path, got %q", m["scope.root"])
	}
}

// ── test: get_version ────────────────────────────────────────────────────────

func TestGetVersion_ReturnsVersion(t *testing.T) {
	const v = "1.2.3"
	svc := MCPServices{Version: v}
	handler := makeGetVersionHandler(svc)
	result, err := handler(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", extractTextContent(t, result.Content))
	}
	text := extractTextContent(t, result.Content)
	var m map[string]string
	if jsonErr := json.Unmarshal([]byte(text), &m); jsonErr != nil {
		t.Fatalf("result not valid JSON: %v", jsonErr)
	}
	if m["version"] != v {
		t.Errorf("expected version=%q, got %q", v, m["version"])
	}
}

func TestGetVersion_DoesNotCallAnyService(t *testing.T) {
	// All service fields are nil — if get_version calls any, it will panic
	// and withPanicRecovery would turn it into an error result.
	svc := MCPServices{Version: "0.9.9"}
	handler := makeGetVersionHandler(svc)
	result, err := handler(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("get_version should not error with nil services: %s", extractTextContent(t, result.Content))
	}
}
