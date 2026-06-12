package mcp

import (
	"encoding/json"
	"errors"
	"fmt"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/EndersonPro/flutree/internal/domain"
)

// mcpCallToolResult is an alias so the rest of the package and tests share the same type.
type mcpCallToolResult = mcplib.CallToolResult

// newToolResultText wraps mcp.NewToolResultText for internal use.
func newToolResultText(text string) *mcpCallToolResult {
	return mcplib.NewToolResultText(text)
}

// toolErrorPayload is the JSON body placed inside the tool-level error content.
type toolErrorPayload struct {
	Category string `json:"category"`
	Message  string `json:"message"`
	Hint     string `json:"hint,omitempty"`
	Code     int    `json:"code"`
}

// toToolError converts any error into an MCP tool-level error result (IsError=true).
// Domain errors are serialised with their Category, Message, Hint, and Code.
// Plain (non-domain) errors are mapped to category "unexpected".
func toToolError(err error) *mcpCallToolResult {
	payload := toolErrorPayload{
		Category: string(domain.CategoryUnexpected),
		Message:  err.Error(),
	}

	var appErr *domain.AppError
	if ok := errors.As(err, &appErr); ok {
		payload.Category = string(appErr.Category)
		payload.Message = appErr.Message
		payload.Hint = appErr.Hint
		payload.Code = appErr.Code
	}

	raw, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		// Fallback: plain text if JSON serialisation somehow fails.
		r := mcplib.NewToolResultText(fmt.Sprintf(`{"category":"unexpected","message":%q}`, err.Error()))
		r.IsError = true
		return r
	}

	result := mcplib.NewToolResultText(string(raw))
	result.IsError = true
	return result
}

// withPanicRecovery executes fn and converts any panic into a tool-level error
// with category "unexpected", so the MCP server stays alive on unexpected panics.
func withPanicRecovery(fn func() (*mcpCallToolResult, error)) (result *mcpCallToolResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			panicMsg := fmt.Sprintf("%v", r)
			panicErr := domain.NewError(
				domain.CategoryUnexpected,
				1,
				"unexpected panic: "+panicMsg,
				"",
				nil,
			)
			result = toToolError(panicErr)
			err = nil
		}
	}()
	return fn()
}
