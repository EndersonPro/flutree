package mcp

import (
	"context"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/EndersonPro/flutree/internal/domain"
)

type countingCompleteRunner struct{ calls int }

func (c *countingCompleteRunner) Run(domain.CompleteInput) (domain.CompleteResult, error) {
	c.calls++
	return domain.CompleteResult{}, nil
}

// Handlers called directly have no client session, so they take the
// no-elicitation path.
func TestCompleteWorktree_WithoutConfirmReturnsNeedsUserInput(t *testing.T) {
	fake := &countingCompleteRunner{}
	handler := makeCompleteWorktreeHandler(MCPServices{Complete: fake})

	for _, args := range []map[string]any{{"name": "wt"}, {"name": "wt", "force": true}} {
		out, res := callJSON(t, handler, args)
		if res.IsError || out["needs_user_input"] != true || out["completed"] != false {
			t.Fatalf("args %v: result = %v", args, out)
		}
		questions, _ := out["questions"].([]any)
		if len(questions) != 1 {
			t.Fatalf("expected one question, got %v", out["questions"])
		}
		if instruction, _ := out["instruction"].(string); !strings.Contains(instruction, "confirm=true") {
			t.Errorf("instruction = %q", instruction)
		}
	}
	if fake.calls != 0 {
		t.Fatalf("nothing may be removed without confirmation, Run called %d times", fake.calls)
	}
}

func TestPubGet_ForceNeedsConfirmButPlainRunDoesNot(t *testing.T) {
	handler := makePubGetHandler(MCPServices{PubGet: &fakePubGetRunner{}})

	out, res := callJSON(t, handler, map[string]any{"name": "wt", "force": true})
	if res.IsError || out["needs_user_input"] != true || out["executed"] != false {
		t.Fatalf("pubget force without confirm = %v", out)
	}
	if res, err := handler(context.Background(), makeRequest(map[string]any{"name": "wt"})); err != nil || res.IsError {
		t.Fatalf("a plain pubget must run: %v %v", res, err)
	}
	if res, err := handler(context.Background(), makeRequest(map[string]any{"name": "wt", "force": true, "confirm": true})); err != nil || res.IsError {
		t.Fatalf("pubget force with confirm must run: %v %v", res, err)
	}
}

func TestDeclinedAndNeedsInputResultsAreNormalResults(t *testing.T) {
	declined := declinedResult(map[string]any{"created": false}, askCancelled)
	if declined.IsError || !strings.Contains(extractTextContent(t, declined.Content), "cancelled_by_user") {
		t.Errorf("declined result = %+v", declined)
	}
	needs := needsInputResult(map[string]any{"created": false}, "ask the user", []userQuestion{{Field: "confirm", Prompt: "ok?", Type: "boolean"}})
	text := extractTextContent(t, needs.Content)
	if needs.IsError || !strings.Contains(text, `"needs_user_input":true`) || !strings.Contains(text, `"field":"confirm"`) {
		t.Errorf("needs_user_input result = %s", text)
	}
}

func TestPromptsMatchTheRealTools(t *testing.T) {
	ctx := context.Background()
	get := func(name string, args map[string]string) (string, error) {
		req := mcplib.GetPromptRequest{}
		req.Params.Name = name
		req.Params.Arguments = args
		var (
			res *mcplib.GetPromptResult
			err error
		)
		switch name {
		case "create_workspace_flow":
			res, err = createWorkspaceFlowPrompt(ctx, req)
		case "add_package_flow":
			res, err = addPackageFlowPrompt(ctx, req)
		default:
			res, err = finishWorkspaceFlowPrompt(ctx, req)
		}
		if err != nil {
			return "", err
		}
		return res.Messages[0].Content.(mcplib.TextContent).Text, nil
	}

	registered := map[string]bool{}
	for _, name := range expectedToolNames {
		registered[name] = true
	}
	for _, tc := range []struct {
		prompt string
		args   map[string]string
	}{
		{"create_workspace_flow", nil},
		{"add_package_flow", map[string]string{"workspace": "w"}},
		{"finish_workspace_flow", map[string]string{"workspace": "w"}},
	} {
		text, err := get(tc.prompt, tc.args)
		if err != nil || strings.TrimSpace(text) == "" {
			t.Fatalf("%s: %q %v", tc.prompt, text, err)
		}
		for _, word := range strings.FieldsFunc(text, func(r rune) bool { return !(r == '_' || r >= 'a' && r <= 'z') }) {
			// every snake_case tool-looking word that is one of our tool names exists
			if strings.Contains(word, "_") && strings.HasSuffix(word, "_worktree") && !registered[word] {
				t.Errorf("%s mentions unknown tool %q", tc.prompt, word)
			}
		}
	}
	for _, name := range []string{"add_package_flow", "finish_workspace_flow"} {
		if _, err := get(name, nil); err == nil {
			t.Errorf("%s must require the workspace argument", name)
		}
	}
}
