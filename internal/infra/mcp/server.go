package mcp

import (
	"context"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/EndersonPro/flutree/internal/domain"
)

// Narrow service interfaces so tool handlers stay testable without wiring real infra.

type listRunner interface {
	Run(input domain.ListInput) ([]domain.ListRow, error)
}

type createRunner interface {
	BuildDryPlan(input domain.CreateInput) (domain.CreateDryPlan, error)
	Apply(plan domain.CreateDryPlan, opts domain.CreateApplyOptions) (domain.CreateResult, error)
}

type addRepoRunner interface {
	Run(input domain.AddRepoInput) (domain.AddRepoResult, error)
}

type completeRunner interface {
	Run(input domain.CompleteInput) (domain.CompleteResult, error)
}

type pubGetRunner interface {
	Run(input domain.PubGetInput) (domain.PubGetResult, error)
}

type cleanRunner interface {
	Run(input domain.CleanInput) (domain.CleanResult, error)
}

type configRunner interface {
	GetScopeRoot(key string) (string, error)
	SetScopeRoot(key, value string) (string, error)
}

// MCPServices holds all application-layer service interfaces that MCP tool
// handlers invoke. Any field may be nil in test contexts where only a subset
// of tools is exercised; the handlers guard against nil via withPanicRecovery.
type MCPServices struct {
	Version  string
	List     listRunner
	Create   createRunner
	AddRepo  addRepoRunner
	Complete completeRunner
	PubGet   pubGetRunner
	Clean    cleanRunner
	Config   configRunner
}

// BuildServer constructs and returns a fully registered MCP server.
// All 9 tools are registered here; actual handler implementations live in tools.go.
func BuildServer(version string, services MCPServices) *mcpserver.MCPServer {
	s := mcpserver.NewMCPServer("flutree", version)

	s.AddTool(buildListWorktreesTool(), makeListWorktreesHandler(services))
	s.AddTool(buildCreateWorktreeTool(), makeCreateWorktreeHandler(services))
	s.AddTool(buildAddRepoTool(), makeAddRepoHandler(services))
	s.AddTool(buildCompleteWorktreeTool(), makeCompleteWorktreeHandler(services))
	s.AddTool(buildPubGetTool(), makePubGetHandler(services))
	s.AddTool(buildCleanWorktreeTool(), makeCleanWorktreeHandler(services))
	s.AddTool(buildGetConfigTool(), makeGetConfigHandler(services))
	s.AddTool(buildSetConfigTool(), makeSetConfigHandler(services))
	s.AddTool(buildGetVersionTool(), makeGetVersionHandler(services))

	return s
}

// ServeStdio blocks on stdin and serves MCP requests until the client closes the connection.
func ServeStdio(s *mcpserver.MCPServer) error {
	return mcpserver.ServeStdio(s)
}

// toolHandler is the function signature expected by mcp-go AddTool.
type toolHandler = func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error)

// buildListWorktreesTool defines the tool schema for list_worktrees.
func buildListWorktreesTool() mcplib.Tool {
	return mcplib.NewTool("list_worktrees",
		mcplib.WithDescription("List registered flutree worktrees."),
		mcplib.WithBoolean("show_all", mcplib.Description("Include unmanaged worktrees.")),
		mcplib.WithBoolean("global_scope", mcplib.Description("List across all repos, not just the current one.")),
	)
}

// buildCreateWorktreeTool defines the tool schema for create_worktree.
func buildCreateWorktreeTool() mcplib.Tool {
	return mcplib.NewTool("create_worktree",
		mcplib.WithDescription("Create a new flutree worktree."),
		mcplib.WithString("name", mcplib.Required(), mcplib.Description("Worktree name.")),
		mcplib.WithString("root_repo", mcplib.Required(), mcplib.Description("Absolute path to the root repository.")),
		mcplib.WithString("scope", mcplib.Required(), mcplib.Description("Execution scope path.")),
		mcplib.WithString("branch", mcplib.Description("Branch name to create.")),
		mcplib.WithString("base_branch", mcplib.Description("Base branch to branch from.")),
		mcplib.WithBoolean("no_package", mcplib.Description("Skip package discovery.")),
		mcplib.WithArray("packages", mcplib.Description("Package selectors.")),
		mcplib.WithObject("package_base_branches", mcplib.Description("Per-package base branch overrides.")),
		mcplib.WithBoolean("yes", mcplib.Description("Auto-confirm prompts (default true).")),
		mcplib.WithBoolean("reuse_existing_branch", mcplib.Description("Reuse a branch if it already exists.")),
		mcplib.WithBoolean("generate_workspace", mcplib.Description("Generate a Flutter workspace file.")),
		mcplib.WithArray("copy_root_files", mcplib.Description("Root files to copy into the worktree.")),
	)
}

// buildAddRepoTool defines the tool schema for add_repo.
func buildAddRepoTool() mcplib.Tool {
	return mcplib.NewTool("add_repo",
		mcplib.WithDescription("Add repositories to an existing flutree workspace."),
		mcplib.WithString("workspace_name", mcplib.Required(), mcplib.Description("Target workspace name.")),
		mcplib.WithArray("repos", mcplib.Required(), mcplib.Description("Repository selectors to add.")),
		mcplib.WithString("scope", mcplib.Description("Execution scope path.")),
		mcplib.WithObject("package_branch_source", mcplib.Description("Per-package branch source overrides.")),
		mcplib.WithObject("package_base", mcplib.Description("Per-package base branch overrides.")),
		mcplib.WithString("sync_policy", mcplib.Description("Sync policy: auto|always|never (default auto).")),
		mcplib.WithBoolean("reuse_existing_branch", mcplib.Description("Reuse existing branches.")),
		mcplib.WithArray("copy_root_files", mcplib.Description("Root files to copy.")),
	)
}

// buildCompleteWorktreeTool defines the tool schema for complete_worktree.
func buildCompleteWorktreeTool() mcplib.Tool {
	return mcplib.NewTool("complete_worktree",
		mcplib.WithDescription("Mark a flutree worktree as completed and remove it."),
		mcplib.WithString("name", mcplib.Required(), mcplib.Description("Worktree name.")),
		mcplib.WithBoolean("force", mcplib.Description("Force removal even if the worktree is dirty.")),
	)
}

// buildPubGetTool defines the tool schema for pubget.
func buildPubGetTool() mcplib.Tool {
	return mcplib.NewTool("pubget",
		mcplib.WithDescription("Run pub get on a flutree worktree."),
		mcplib.WithString("name", mcplib.Required(), mcplib.Description("Worktree name.")),
		mcplib.WithBoolean("force", mcplib.Description("Force pub get even if up to date.")),
	)
}

// buildCleanWorktreeTool defines the tool schema for clean_worktree.
func buildCleanWorktreeTool() mcplib.Tool {
	return mcplib.NewTool("clean_worktree",
		mcplib.WithDescription("Clean stale flutree worktrees."),
		mcplib.WithBoolean("force", mcplib.Description("Force clean without confirmation.")),
	)
}

// buildGetConfigTool defines the tool schema for get_config.
func buildGetConfigTool() mcplib.Tool {
	return mcplib.NewTool("get_config",
		mcplib.WithDescription("Get a flutree configuration value."),
		mcplib.WithString("key", mcplib.Required(), mcplib.Description("Config key (e.g. scope.root).")),
	)
}

// buildSetConfigTool defines the tool schema for set_config.
func buildSetConfigTool() mcplib.Tool {
	return mcplib.NewTool("set_config",
		mcplib.WithDescription("Set a flutree configuration value."),
		mcplib.WithString("key", mcplib.Required(), mcplib.Description("Config key (e.g. scope.root).")),
		mcplib.WithString("value", mcplib.Required(), mcplib.Description("New value to set.")),
	)
}

// buildGetVersionTool defines the tool schema for get_version.
func buildGetVersionTool() mcplib.Tool {
	return mcplib.NewTool("get_version",
		mcplib.WithDescription("Return the flutree binary version without calling any service."),
	)
}
