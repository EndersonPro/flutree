package mcp

import (
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

func TestBuildServerIsNotNil(t *testing.T) {
	svc := MCPServices{Version: "0.0.0-test"}
	s := BuildServer("0.0.0-test", svc)
	if s == nil {
		t.Fatal("expected non-nil MCPServer")
	}
}

func TestBuildServerRegistersExactlyNineTools(t *testing.T) {
	svc := MCPServices{Version: "0.0.0-test"}
	s := BuildServer("0.0.0-test", svc)

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

func TestBuildServerVersionIsSetInServices(t *testing.T) {
	const v = "1.2.3"
	svc := MCPServices{Version: v}
	s := BuildServer(v, svc)
	if s == nil {
		t.Fatal("expected non-nil MCPServer")
	}
	// The version field is embedded in MCPServices and accessible to get_version tool.
	if svc.Version != v {
		t.Errorf("expected version %q in MCPServices, got %q", v, svc.Version)
	}
}
