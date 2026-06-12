package mcp

import (
	"encoding/json"
	"errors"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/EndersonPro/flutree/internal/domain"
)

func TestToToolError_DomainError(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCat  string
		wantMsg  string
		wantHint string
		wantCode int
	}{
		{
			name:     "input error",
			err:      domain.NewError(domain.CategoryInput, 2, "bad input", "fix it", nil),
			wantCat:  "input",
			wantMsg:  "bad input",
			wantHint: "fix it",
			wantCode: 2,
		},
		{
			name:     "precondition error no hint",
			err:      domain.NewError(domain.CategoryPrecondition, 3, "precondition failed", "", nil),
			wantCat:  "precondition",
			wantMsg:  "precondition failed",
			wantHint: "",
			wantCode: 3,
		},
		{
			name:     "git error",
			err:      domain.NewError(domain.CategoryGit, 5, "git error", "run git fetch", nil),
			wantCat:  "git",
			wantMsg:  "git error",
			wantHint: "run git fetch",
			wantCode: 5,
		},
		{
			name:     "unexpected error wrapped",
			err:      domain.NewError(domain.CategoryUnexpected, 1, "unexpected", "", errors.New("root cause")),
			wantCat:  "unexpected",
			wantMsg:  "unexpected",
			wantHint: "",
			wantCode: 1,
		},
		{
			name:     "plain error mapped to unexpected",
			err:      errors.New("plain error"),
			wantCat:  "unexpected",
			wantMsg:  "plain error",
			wantHint: "",
			wantCode: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := toToolError(tc.err)

			if result == nil {
				t.Fatal("expected non-nil result")
			}
			if !result.IsError {
				t.Fatalf("expected IsError=true, got false")
			}
			if len(result.Content) == 0 {
				t.Fatal("expected non-empty content")
			}

			text := extractTextContent(t, result.Content)

			var payload struct {
				Category string `json:"category"`
				Message  string `json:"message"`
				Hint     string `json:"hint,omitempty"`
				Code     int    `json:"code"`
			}
			if err := json.Unmarshal([]byte(text), &payload); err != nil {
				t.Fatalf("content is not valid JSON: %v — got: %q", err, text)
			}
			if payload.Category != tc.wantCat {
				t.Errorf("category: want %q got %q", tc.wantCat, payload.Category)
			}
			if payload.Message != tc.wantMsg {
				t.Errorf("message: want %q got %q", tc.wantMsg, payload.Message)
			}
			if payload.Hint != tc.wantHint {
				t.Errorf("hint: want %q got %q", tc.wantHint, payload.Hint)
			}
			if payload.Code != tc.wantCode {
				t.Errorf("code: want %d got %d", tc.wantCode, payload.Code)
			}
		})
	}
}

func TestToToolError_JoinedErrorChain(t *testing.T) {
	// errors.Join produces an error with Unwrap() []error — the old manual walk
	// missed this case; errors.As handles it correctly.
	appErr := domain.NewError(domain.CategoryGit, 7, "joined git error", "try again", nil)
	joined := errors.Join(errors.New("wrapper"), appErr)

	result := toToolError(joined)

	if result == nil || !result.IsError {
		t.Fatal("expected IsError=true")
	}
	text := extractTextContent(t, result.Content)
	var payload struct {
		Category string `json:"category"`
		Code     int    `json:"code"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("not valid JSON: %v — got: %q", err, text)
	}
	if payload.Category != "git" {
		t.Errorf("expected category=git, got %q", payload.Category)
	}
	if payload.Code != 7 {
		t.Errorf("expected code=7, got %d", payload.Code)
	}
}

func TestWithPanicRecovery_NoPanic(t *testing.T) {
	called := false
	result, err := withPanicRecovery(func() (*mcpCallToolResult, error) {
		called = true
		return newToolResultText("ok"), nil
	})
	if !called {
		t.Fatal("expected function to be called")
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestWithPanicRecovery_Panic(t *testing.T) {
	result, err := withPanicRecovery(func() (*mcpCallToolResult, error) {
		panic("something blew up")
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result after panic recovery")
	}
	if !result.IsError {
		t.Fatalf("expected IsError=true after panic")
	}

	text := extractTextContent(t, result.Content)

	var payload struct {
		Category string `json:"category"`
	}
	if err2 := json.Unmarshal([]byte(text), &payload); err2 != nil {
		t.Fatalf("content not valid JSON: %v — got: %q", err2, text)
	}
	if payload.Category != "unexpected" {
		t.Errorf("expected category=unexpected, got %q", payload.Category)
	}
}

func TestWithPanicRecovery_NilPanic(t *testing.T) {
	result, err := withPanicRecovery(func() (*mcpCallToolResult, error) {
		var p *string
		_ = *p // nil dereference
		return newToolResultText("never"), nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected IsError=true after nil-dereference panic")
	}
}

// extractTextContent pulls the text string from the first content element.
func extractTextContent(t *testing.T, content []mcplib.Content) string {
	t.Helper()
	if len(content) == 0 {
		t.Fatal("content slice is empty")
	}
	tc, ok := mcplib.AsTextContent(content[0])
	if !ok {
		t.Fatalf("content[0] is not TextContent: %T", content[0])
	}
	return tc.Text
}
