package mcp

import (
	"context"
	"encoding/json"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/EndersonPro/flutree/internal/domain"
)

// Ensure concrete app services satisfy the runner interfaces defined in server.go.
// These compile-time checks belong here rather than in main.go to keep the
// infra/mcp package self-contained.
//
// They are placed in tools.go because tools.go imports both the interfaces (via
// the mcp package itself) and the app services (via the domain types they share).

// makeListWorktreesHandler returns the handler for the list_worktrees tool.
func makeListWorktreesHandler(svc MCPServices) toolHandler {
	return func(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return withPanicRecovery(func() (*mcpCallToolResult, error) {
			input := domain.ListInput{
				ShowAll:     req.GetBool("show_all", false),
				GlobalScope: req.GetBool("global_scope", false),
			}
			rows, err := svc.List.Run(input)
			if err != nil {
				return toToolError(err), nil
			}
			if rows == nil {
				rows = []domain.ListRow{}
			}
			return marshalResult(rows)
		})
	}
}

// makeCreateWorktreeHandler returns the handler for the create_worktree tool.
func makeCreateWorktreeHandler(svc MCPServices) toolHandler {
	return func(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return withPanicRecovery(func() (*mcpCallToolResult, error) {
			name := req.GetString("name", "")
			rootRepo := req.GetString("root_repo", "")
			scope := req.GetString("scope", "")

			if name == "" {
				return toToolError(domain.NewError(domain.CategoryInput, 2,
					"Missing required argument: name.",
					"Provide a non-empty worktree name.",
					nil,
				)), nil
			}
			if rootRepo == "" {
				return toToolError(domain.NewError(domain.CategoryInput, 2,
					"Missing required argument: root_repo.",
					"Provide the absolute path to the root repository.",
					nil,
				)), nil
			}
			if scope == "" {
				return toToolError(domain.NewError(domain.CategoryInput, 2,
					"Missing required argument: scope.",
					"Provide the execution scope path.",
					nil,
				)), nil
			}

			reuseExistingBranch := req.GetBool("reuse_existing_branch", false)
			input := domain.CreateInput{
				Name:              name,
				RootSelector:      rootRepo,
				ExecutionScope:    scope,
				Branch:            req.GetString("branch", ""),
				BaseBranch:        req.GetString("base_branch", ""),
				NoPackage:         req.GetBool("no_package", false),
				PackageSelectors:  req.GetStringSlice("packages", nil),
				PackageBaseBranch: getStringMap(req, "package_base_branches"),
				GenerateWorkspace: req.GetBool("generate_workspace", false),
				RootFiles:         req.GetStringSlice("copy_root_files", nil),
				Yes:               req.GetBool("yes", true),
				NonInteractive:    true,
			}

			plan, err := svc.Create.BuildDryPlan(input)
			if err != nil {
				return toToolError(err), nil
			}
			opts := domain.CreateApplyOptions{
				NonInteractive:      true,
				ReuseExistingBranch: reuseExistingBranch,
			}
			result, err := svc.Create.Apply(plan, opts)
			if err != nil {
				return toToolError(err), nil
			}
			return marshalResult(result)
		})
	}
}

// makeAddRepoHandler returns the handler for the add_repo tool.
func makeAddRepoHandler(svc MCPServices) toolHandler {
	return func(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return withPanicRecovery(func() (*mcpCallToolResult, error) {
			input := domain.AddRepoInput{
				WorkspaceName:       req.GetString("workspace_name", ""),
				RepoSelectors:       req.GetStringSlice("repos", nil),
				ExecutionScope:      req.GetString("scope", ""),
				PackageBranchSource: getStringMap(req, "package_branch_source"),
				PackageBaseBranch:   getStringMap(req, "package_base"),
				SyncPolicy:          domain.AddRepoSyncPolicy(req.GetString("sync_policy", string(domain.AddRepoSyncAuto))),
				ReuseExistingBranch: req.GetBool("reuse_existing_branch", false),
				RootFiles:           req.GetStringSlice("copy_root_files", nil),
				NonInteractive:      true,
			}
			result, err := svc.AddRepo.Run(input)
			if err != nil {
				return toToolError(err), nil
			}
			return marshalResult(result)
		})
	}
}

// makeCompleteWorktreeHandler returns the handler for the complete_worktree tool.
func makeCompleteWorktreeHandler(svc MCPServices) toolHandler {
	return func(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return withPanicRecovery(func() (*mcpCallToolResult, error) {
			input := domain.CompleteInput{
				Name:           req.GetString("name", ""),
				Force:          req.GetBool("force", false),
				NonInteractive: true,
				Yes:            true,
			}
			result, err := svc.Complete.Run(input)
			if err != nil {
				return toToolError(err), nil
			}
			return marshalResult(result)
		})
	}
}

// makePubGetHandler returns the handler for the pubget tool.
func makePubGetHandler(svc MCPServices) toolHandler {
	return func(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return withPanicRecovery(func() (*mcpCallToolResult, error) {
			input := domain.PubGetInput{
				Name:  req.GetString("name", ""),
				Force: req.GetBool("force", false),
			}
			result, err := svc.PubGet.Run(input)
			if err != nil {
				return toToolError(err), nil
			}
			return marshalResult(result)
		})
	}
}

// makeCleanWorktreeHandler returns the handler for the clean_worktree tool.
func makeCleanWorktreeHandler(svc MCPServices) toolHandler {
	return func(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return withPanicRecovery(func() (*mcpCallToolResult, error) {
			input := domain.CleanInput{
				Force: req.GetBool("force", false),
			}
			result, err := svc.Clean.Run(input)
			if err != nil {
				return toToolError(err), nil
			}
			return marshalResult(result)
		})
	}
}

// makeGetConfigHandler returns the handler for the get_config tool.
func makeGetConfigHandler(svc MCPServices) toolHandler {
	return func(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return withPanicRecovery(func() (*mcpCallToolResult, error) {
			key := req.GetString("key", "")
			value, err := svc.Config.GetScopeRoot(key)
			if err != nil {
				return toToolError(err), nil
			}
			return marshalResult(map[string]string{key: value})
		})
	}
}

// makeSetConfigHandler returns the handler for the set_config tool.
func makeSetConfigHandler(svc MCPServices) toolHandler {
	return func(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return withPanicRecovery(func() (*mcpCallToolResult, error) {
			key := req.GetString("key", "")
			value := req.GetString("value", "")
			newValue, err := svc.Config.SetScopeRoot(key, value)
			if err != nil {
				return toToolError(err), nil
			}
			return marshalResult(map[string]string{key: newValue})
		})
	}
}

// makeGetVersionHandler returns the handler for the get_version tool.
func makeGetVersionHandler(svc MCPServices) toolHandler {
	return func(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return withPanicRecovery(func() (*mcpCallToolResult, error) {
			return marshalResult(map[string]string{"version": svc.Version})
		})
	}
}

// getStringMap reads an object argument from the request as map[string]string.
// Keys whose values are not strings are silently skipped.
// Returns nil when the key is absent or not an object.
func getStringMap(req mcplib.CallToolRequest, key string) map[string]string {
	args := req.GetArguments()
	raw, ok := args[key]
	if !ok || raw == nil {
		return nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// marshalResult JSON-encodes v and returns it as a tool text result.
func marshalResult(v any) (*mcpCallToolResult, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return toToolError(domain.NewError(domain.CategoryUnexpected, 1,
			"failed to marshal result: "+err.Error(), "", err)), nil
	}
	return newToolResultText(string(raw)), nil
}
