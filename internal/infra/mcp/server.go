package mcp

import (
	"context"
	"fmt"

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

// inspectRunner serves the read-only discovery and status tools and resources.
type inspectRunner interface {
	DiscoverRepos(scope string) ([]domain.RepoSummary, error)
	InspectRepo(path string) (domain.RepoInspection, error)
	ListBranches(repoPath string) (domain.BranchList, error)
	WorkspaceStatus(name string) (domain.WorkspaceStatus, error)
}

type configRunner interface {
	Get(key string) (string, error)
	Set(key, value string) (string, error)
}

// MCPServices holds all application-layer service interfaces that MCP tool
// handlers invoke. All interface fields must be non-nil; BuildServer returns
// an error listing every nil field so callers get an explicit failure rather
// than an opaque panic at call time. withPanicRecovery remains as a last resort.
//
// Version is intentionally NOT a field here; BuildServer receives version as an
// explicit parameter and passes it directly to makeGetVersionHandler, keeping
// a single authoritative source and preventing silent divergence.
type MCPServices struct {
	List     listRunner
	Create   createRunner
	AddRepo  addRepoRunner
	Complete completeRunner
	PubGet   pubGetRunner
	Clean    cleanRunner
	Config   configRunner
	Inspect  inspectRunner
}

// BuildServer constructs and returns a fully registered MCP server, or an
// error if any required service field in services is nil.
// All tools, resources and the server instructions are registered here; actual handler implementations live in tools.go.
func BuildServer(version string, services MCPServices) (*mcpserver.MCPServer, error) {
	var nilFields []string
	if services.List == nil {
		nilFields = append(nilFields, "List")
	}
	if services.Create == nil {
		nilFields = append(nilFields, "Create")
	}
	if services.AddRepo == nil {
		nilFields = append(nilFields, "AddRepo")
	}
	if services.Complete == nil {
		nilFields = append(nilFields, "Complete")
	}
	if services.PubGet == nil {
		nilFields = append(nilFields, "PubGet")
	}
	if services.Clean == nil {
		nilFields = append(nilFields, "Clean")
	}
	if services.Config == nil {
		nilFields = append(nilFields, "Config")
	}
	if services.Inspect == nil {
		nilFields = append(nilFields, "Inspect")
	}
	if len(nilFields) > 0 {
		return nil, fmt.Errorf("BuildServer: nil service fields: %v", nilFields)
	}

	s := mcpserver.NewMCPServer("flutree", version,
		mcpserver.WithInstructions(serverInstructions),
		mcpserver.WithResourceCapabilities(false, false),
	)

	s.AddTool(buildListWorktreesTool(), makeListWorktreesHandler(services))
	s.AddTool(buildCreateWorktreeTool(), makeCreateWorktreeHandler(services))
	s.AddTool(buildAddRepoTool(), makeAddRepoHandler(services))
	s.AddTool(buildCompleteWorktreeTool(), makeCompleteWorktreeHandler(services))
	s.AddTool(buildPubGetTool(), makePubGetHandler(services))
	s.AddTool(buildCleanWorktreeTool(), makeCleanWorktreeHandler(services))
	s.AddTool(buildGetConfigTool(), makeGetConfigHandler(services))
	s.AddTool(buildSetConfigTool(), makeSetConfigHandler(services))
	s.AddTool(buildGetVersionTool(), makeGetVersionHandler(version, services))

	s.AddTool(buildDiscoverReposTool(), makeDiscoverReposHandler(services))
	s.AddTool(buildInspectRepoTool(), makeInspectRepoHandler(services))
	s.AddTool(buildListBranchesTool(), makeListBranchesHandler(services))
	s.AddTool(buildPlanCreateTool(), makePlanCreateHandler(services))
	s.AddTool(buildCreateWorkspaceTool(), makeCreateWorkspaceHandler(services))
	s.AddTool(buildWorkspaceStatusTool(), makeWorkspaceStatusHandler(services))

	registerResources(s, services)

	return s, nil
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
		mcplib.WithDescription("List the workspaces flutree manages (name, branch, root path, status active|missing|completed, package_count). Use it first to see what already exists, and to check that a name is free before planning a new workspace. Read-only. Same data as the flutree://workspaces resource."),
		mcplib.WithReadOnlyHintAnnotation(true), mcplib.WithDestructiveHintAnnotation(false),
		mcplib.WithBoolean("show_all", mcplib.Description("Also list git worktrees that flutree does not manage (default false).")),
		mcplib.WithBoolean("global_scope", mcplib.Description("List workspaces of every repository, not only the repository of the server's working directory. Agents usually want true (default false).")),
	)
}

// buildCreateWorktreeTool defines the tool schema for create_worktree.
// base_branch is required — agents must ask the user rather than guess.
// sync_with_remote fetches origin/<base_branch> before creating so the
// worktree starts from the latest remote state.
func buildCreateWorktreeTool() mcplib.Tool {
	return mcplib.NewTool("create_worktree",
		mcplib.WithDescription("Legacy one-shot creation of a workspace: plans and creates immediately, with no confirmation step. Prefer plan_create followed by create_workspace (confirm=true), which lets the user review the plan first. Kept for backward compatibility. base_branch is required: ask the user which branch to use instead of guessing; use sync_with_remote=true to fetch origin/<base_branch> first."),
		mcplib.WithString("name", mcplib.Required(), mcplib.Description("Name of the new workspace; lowercased and slugified for folders, and the default branch is feature/<name>.")),
		mcplib.WithString("root_repo", mcplib.Required(), mcplib.Description("Absolute path to the root repository.")),
		mcplib.WithString("scope", mcplib.Required(), mcplib.Description("Execution scope path.")),
		mcplib.WithString("branch", mcplib.Description("Branch name to create.")),
		mcplib.WithString("base_branch", mcplib.Required(), mcplib.Description("Git branch to base the new worktree on (e.g. 'main', 'develop'). Required — if the user has not specified one, ask them instead of guessing.")),
		mcplib.WithBoolean("sync_with_remote", mcplib.Description("Fetch the base branch from origin and branch from origin/<base_branch> instead of the local ref. Recommended to get the latest changes before creating.")),
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
		mcplib.WithDescription("Attach more Flutter repositories as package worktrees to an existing workspace and regenerate its pubspec_overrides.yaml. Use it when the user wants another package in a workspace that already exists (use create_workspace for a new one). Ask the user for the base branch of each added repo (package_base). Changes the filesystem and git branches."),
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
		mcplib.WithDescription("DESTRUCTIVE: finish a workspace by removing its root and package worktrees from disk and marking it completed in the registry. Local branches are kept. Use it only when the user says the work is done. It refuses worktrees with uncommitted changes unless force=true: call workspace_status first, show the user what is dirty and ask before forcing."),
		mcplib.WithString("name", mcplib.Required(), mcplib.Description("Workspace name, as shown by list_worktrees.")),
		mcplib.WithBoolean("force", mcplib.Description("Force removal even if the worktree is dirty.")),
	)
}

// buildPubGetTool defines the tool schema for pubget.
func buildPubGetTool() mcplib.Tool {
	return mcplib.NewTool("pubget",
		mcplib.WithDescription("Run pub get (flutter or dart, auto-detected) in every package worktree of the workspace in parallel, then in the root. Use it after create_workspace or add_repo so dependencies resolve through pubspec_overrides.yaml. Takes the workspace name."),
		mcplib.WithString("name", mcplib.Required(), mcplib.Description("Workspace name, as shown by list_worktrees.")),
		mcplib.WithBoolean("force", mcplib.Description("Force pub get even if up to date.")),
	)
}

// buildCleanWorktreeTool defines the tool schema for clean_worktree.
func buildCleanWorktreeTool() mcplib.Tool {
	return mcplib.NewTool("clean_worktree",
		mcplib.WithDescription("Run flutter/dart clean in the managed worktree that contains the server's working directory (the process must run inside a managed worktree). It does not take a workspace name; rarely useful for agents. force=true also removes pubspec.lock."),
		mcplib.WithBoolean("force", mcplib.Description("Force clean without confirmation.")),
	)
}

// buildGetConfigTool defines the tool schema for get_config.
func buildGetConfigTool() mcplib.Tool {
	return mcplib.NewTool("get_config",
		mcplib.WithDescription("Read one flutree configuration value: scope.root (default folder to discover repositories), workspace.generate (true/false) or sdk.manager (auto|fvm|puro|none). All three at once: flutree://config."),
		mcplib.WithReadOnlyHintAnnotation(true), mcplib.WithDestructiveHintAnnotation(false),
		mcplib.WithString("key", mcplib.Required(), mcplib.Description("Config key: scope.root, workspace.generate or sdk.manager.")),
	)
}

// buildSetConfigTool defines the tool schema for set_config.
func buildSetConfigTool() mcplib.Tool {
	return mcplib.NewTool("set_config",
		mcplib.WithDescription("Persist one flutree configuration value. Only change configuration when the user asks for it. scope.root must be an existing directory; workspace.generate takes true|false; sdk.manager takes auto|fvm|puro|none."),
		mcplib.WithString("key", mcplib.Required(), mcplib.Description("Config key: scope.root, workspace.generate or sdk.manager.")),
		mcplib.WithString("value", mcplib.Required(), mcplib.Description("New value to set.")),
	)
}

// buildGetVersionTool defines the tool schema for get_version.
func buildGetVersionTool() mcplib.Tool {
	return mcplib.NewTool("get_version",
		mcplib.WithDescription("Return the version of the flutree binary this server runs. Takes no arguments."),
		mcplib.WithReadOnlyHintAnnotation(true), mcplib.WithDestructiveHintAnnotation(false),
	)
}
