package mcp

import (
	"context"
	"encoding/json"
	"testing"
)

// expectedToolNames is the canonical list of the 9 tools flutree exposes via MCP.
var expectedToolNames = []string{
	"list_worktrees",
	"create_worktree",
	"add_repo",
	"complete_worktree",
	"pubget",
	"clean_worktree",
	"get_config",
	"set_config",
	"get_version",
}

func fullServices(version string) MCPServices {
	return MCPServices{
		Version:  version,
		List:     &fakeListRunner{},
		Create:   &fakeCreateRunner{},
		AddRepo:  &fakeAddRepoRunner{},
		Complete: &fakeCompleteRunner{},
		PubGet:   &fakePubGetRunner{},
		Clean:    &fakeCleanRunner{},
		Config:   &fakeConfigRunner{},
	}
}

func TestBuildServerIsNotNil(t *testing.T) {
	s, err := BuildServer("0.0.0-test", fullServices("0.0.0-test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil MCPServer")
	}
}

func TestBuildServerRegistersExactlyNineTools(t *testing.T) {
	s, err := BuildServer("0.0.0-test", fullServices("0.0.0-test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tools := s.ListTools()
	if len(tools) != len(expectedToolNames) {
		t.Errorf("expected %d tools, got %d", len(expectedToolNames), len(tools))
	}

	for _, name := range expectedToolNames {
		if _, ok := tools[name]; !ok {
			t.Errorf("tool %q not registered", name)
		}
	}
}

func TestBuildServerGetVersionHandlerReturnsVersion(t *testing.T) {
	const v = "1.2.3"
	svc := fullServices(v)
	s, err := BuildServer(v, svc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tools := s.ListTools()
	versionTool, ok := tools["get_version"]
	if !ok {
		t.Fatal("get_version tool not registered")
	}

	result, err := versionTool.Handler(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error")
	}

	text := extractTextContent(t, result.Content)
	var m map[string]string
	if jsonErr := json.Unmarshal([]byte(text), &m); jsonErr != nil {
		t.Fatalf("result not valid JSON: %v — got: %q", jsonErr, text)
	}
	if m["version"] != v {
		t.Errorf("expected version=%q, got %q", v, m["version"])
	}
}

func TestBuildServerNilServiceReturnsError(t *testing.T) {
	cases := []struct {
		name string
		svc  MCPServices
	}{
		{"nil List", MCPServices{Version: "0.0.0-test", Create: &fakeCreateRunner{}, AddRepo: &fakeAddRepoRunner{}, Complete: &fakeCompleteRunner{}, PubGet: &fakePubGetRunner{}, Clean: &fakeCleanRunner{}, Config: &fakeConfigRunner{}}},
		{"nil Create", MCPServices{Version: "0.0.0-test", List: &fakeListRunner{}, AddRepo: &fakeAddRepoRunner{}, Complete: &fakeCompleteRunner{}, PubGet: &fakePubGetRunner{}, Clean: &fakeCleanRunner{}, Config: &fakeConfigRunner{}}},
		{"nil AddRepo", MCPServices{Version: "0.0.0-test", List: &fakeListRunner{}, Create: &fakeCreateRunner{}, Complete: &fakeCompleteRunner{}, PubGet: &fakePubGetRunner{}, Clean: &fakeCleanRunner{}, Config: &fakeConfigRunner{}}},
		{"nil Complete", MCPServices{Version: "0.0.0-test", List: &fakeListRunner{}, Create: &fakeCreateRunner{}, AddRepo: &fakeAddRepoRunner{}, PubGet: &fakePubGetRunner{}, Clean: &fakeCleanRunner{}, Config: &fakeConfigRunner{}}},
		{"nil PubGet", MCPServices{Version: "0.0.0-test", List: &fakeListRunner{}, Create: &fakeCreateRunner{}, AddRepo: &fakeAddRepoRunner{}, Complete: &fakeCompleteRunner{}, Clean: &fakeCleanRunner{}, Config: &fakeConfigRunner{}}},
		{"nil Clean", MCPServices{Version: "0.0.0-test", List: &fakeListRunner{}, Create: &fakeCreateRunner{}, AddRepo: &fakeAddRepoRunner{}, Complete: &fakeCompleteRunner{}, PubGet: &fakePubGetRunner{}, Config: &fakeConfigRunner{}}},
		{"nil Config", MCPServices{Version: "0.0.0-test", List: &fakeListRunner{}, Create: &fakeCreateRunner{}, AddRepo: &fakeAddRepoRunner{}, Complete: &fakeCompleteRunner{}, PubGet: &fakePubGetRunner{}, Clean: &fakeCleanRunner{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildServer("0.0.0-test", tc.svc)
			if err == nil {
				t.Errorf("expected error for %s, got nil", tc.name)
			}
		})
	}
}
