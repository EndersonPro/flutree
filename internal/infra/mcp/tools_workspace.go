package mcp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/EndersonPro/flutree/internal/domain"
)

// This file holds the agent-oriented tools: discovery (discover_repos,
// inspect_repo, list_branches), planning (plan_create), gated creation
// (create_workspace) and inspection (workspace_status). Their results are
// JSON objects delivered both as structuredContent and as text.

// ── input errors ─────────────────────────────────────────────────────────────

func inputError(message, hint string) *mcpCallToolResult {
	return toToolError(domain.NewError(domain.CategoryInput, 2, message, hint, nil))
}

func missingArg(name, hint string) *mcpCallToolResult {
	return inputError("Missing required argument: "+name+".", hint)
}

// marshalStructured returns v as structuredContent plus its JSON text, for
// clients that only read text content.
func marshalStructured(v any) (*mcpCallToolResult, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return toToolError(domain.NewError(domain.CategoryUnexpected, 1,
			"failed to marshal result: "+err.Error(), "", err)), nil
	}
	return mcplib.NewToolResultStructured(v, string(raw)), nil
}

// ── discover_repos ───────────────────────────────────────────────────────────

func buildDiscoverReposTool() mcplib.Tool {
	return mcplib.NewTool("discover_repos",
		mcplib.WithDescription("List the Flutter repositories found under a folder (any git repo with a pubspec.yaml that has a name). Use it at the start of a create flow to learn which repositories can be the root or a package. Read-only. Returns {repos:[{name, package_name, repo_root, current_branch}]}. Fails with category precondition when the folder does not exist or holds no Flutter repository."),
		mcplib.WithString("scope", mcplib.Required(), mcplib.Description("Absolute path of the folder to search recursively (for example the user's code folder, or scope.root from get_config).")),
		mcplib.WithReadOnlyHintAnnotation(true), mcplib.WithDestructiveHintAnnotation(false),
	)
}

func makeDiscoverReposHandler(svc MCPServices) toolHandler {
	return func(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return withPanicRecovery(func() (*mcpCallToolResult, error) {
			scope := strings.TrimSpace(req.GetString("scope", ""))
			if scope == "" {
				return missingArg("scope", "Pass the folder to search; get_config scope.root gives the user's default."), nil
			}
			repos, err := svc.Inspect.DiscoverRepos(scope)
			if err != nil {
				return toToolError(err), nil
			}
			if repos == nil {
				repos = []domain.RepoSummary{}
			}
			return marshalStructured(map[string]any{"repos": repos})
		})
	}
}

// ── inspect_repo ─────────────────────────────────────────────────────────────

func buildInspectRepoTool() mcplib.Tool {
	return mcplib.NewTool("inspect_repo",
		mcplib.WithDescription("Describe one repository before choosing it as root or package: whether it is a Flutter repo, its package name, repo root, current branch, local branches, the Flutter SDK pin (fvm_version from .fvmrc, puro_env from .puro.json) and its pubspec path dependencies (dependency name -> path, as written in dependencies and dev_dependencies). Use path_dependencies to find which other repos a root needs as packages. Read-only. A folder that is not a Flutter repo is a valid answer (is_flutter=false with a reason); a path that does not exist is an input error."),
		mcplib.WithString("path", mcplib.Required(), mcplib.Description("Path of the repository folder (absolute, ~ is not expanded here).")),
		mcplib.WithReadOnlyHintAnnotation(true), mcplib.WithDestructiveHintAnnotation(false),
	)
}

func makeInspectRepoHandler(svc MCPServices) toolHandler {
	return func(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return withPanicRecovery(func() (*mcpCallToolResult, error) {
			path := strings.TrimSpace(req.GetString("path", ""))
			if path == "" {
				return missingArg("path", "Pass the path of the repository to inspect."), nil
			}
			result, err := svc.Inspect.InspectRepo(path)
			if err != nil {
				return toToolError(err), nil
			}
			return marshalStructured(result)
		})
	}
}

// ── list_branches ────────────────────────────────────────────────────────────

func buildListBranchesTool() mcplib.Tool {
	return mcplib.NewTool("list_branches",
		mcplib.WithDescription("List the branches a repository can start a worktree from: local branches and origin/* remote-tracking branches, plus the current branch. Use it to offer the user real choices for base_branch and package_base. Reads local refs only (no fetch), so remote branches may be stale. Read-only. Returns {repo_path, current, local[], remote[]}."),
		mcplib.WithString("repo_path", mcplib.Required(), mcplib.Description("Path of the git repository (a repo_root from discover_repos).")),
		mcplib.WithReadOnlyHintAnnotation(true), mcplib.WithDestructiveHintAnnotation(false),
	)
}

func makeListBranchesHandler(svc MCPServices) toolHandler {
	return func(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return withPanicRecovery(func() (*mcpCallToolResult, error) {
			path := strings.TrimSpace(req.GetString("repo_path", ""))
			if path == "" {
				return missingArg("repo_path", "Pass the repository path, for example a repo_root from discover_repos."), nil
			}
			result, err := svc.Inspect.ListBranches(path)
			if err != nil {
				return toToolError(err), nil
			}
			return marshalStructured(result)
		})
	}
}

// ── plan_create / create_workspace ───────────────────────────────────────────

// createToolOptions are the arguments shared by plan_create and
// create_workspace; the second one adds confirm and the apply options.
func createToolOptions() []mcplib.ToolOption {
	return []mcplib.ToolOption{
		mcplib.WithString("name", mcplib.Required(), mcplib.Description("Name of the new workspace. Lowercased and slugified for the folder name; the default branch is feature/<name>. It must not already exist (see list_worktrees).")),
		mcplib.WithString("root_repo", mcplib.Required(), mcplib.Description("The root repository: a folder name or package name found by discover_repos, or a path (absolute, ~ or relative to the server's working directory), which may lie outside scope.")),
		mcplib.WithString("base_branch", mcplib.Required(), mcplib.Description("Branch (or tag/commit) the ROOT worktree starts from, for example 'main' or 'develop'. Ask the user; do not guess. Use list_branches to offer choices.")),
		mcplib.WithArray("packages", mcplib.Description("Package repositories to create worktrees for; same selector forms as root_repo. Empty or omitted creates a root-only workspace. Every entry needs a package_base entry."), mcplib.WithStringItems()),
		mcplib.WithObject("package_base", mcplib.Description("Base branch per package, {\"<package folder name or repo path>\": \"<branch>\"}. Required for every entry of packages. Ask the user; the branch must exist locally or as origin/<branch>.")),
		mcplib.WithString("scope", mcplib.Description("Folder searched for repositories when root_repo or packages are names. Defaults to the configured scope.root. Not needed when every selector is a path.")),
		mcplib.WithString("branch", mcplib.Description("Name of the new branch created in every worktree. Defaults to feature/<name>.")),
		mcplib.WithBoolean("generate_workspace", mcplib.Description("Write a VSCode <name>.code-workspace file. Defaults to the workspace.generate config (true unless changed).")),
		mcplib.WithArray("copy_root_files", mcplib.Description("Extra file patterns to copy from each source repo into its worktree. .env and .env.* are always copied."), mcplib.WithStringItems()),
	}
}

func buildPlanCreateTool() mcplib.Tool {
	opts := append([]mcplib.ToolOption{
		mcplib.WithDescription("Dry-run a workspace creation. Use it AFTER asking the user for the name, root repo, packages and base branches, and BEFORE create_workspace. It validates everything (selectors, that every base branch exists, that the target folder is free) and returns the exact plan without creating any directory, branch or registry entry: root and package worktree paths, branches and base branches, the full pubspec_overrides.yaml content, and the workspace file. Show the plan to the user and wait for an explicit yes. Returns {name, container_path, root, packages[], override_path, override_content, workspace_file, workspace_folders, copy_root_files}."),
		mcplib.WithReadOnlyHintAnnotation(true), mcplib.WithDestructiveHintAnnotation(false),
	}, createToolOptions()...)
	return mcplib.NewTool("plan_create", opts...)
}

func buildCreateWorkspaceTool() mcplib.Tool {
	opts := append([]mcplib.ToolOption{
		mcplib.WithDescription("Create a workspace: git worktrees for the root and every package on a new branch, the generated pubspec_overrides.yaml (git-excluded), the registry entry, the optional .code-workspace file and fvm/puro SDK setup. Takes the same arguments as plan_create plus confirm. It creates NOTHING unless confirm=true: without it the result is {created:false, status:\"confirmation_required\", plan} so you can show the plan. Set confirm=true only after the user explicitly approved that plan. Preferred over the legacy create_worktree. On success returns {created:true, status:\"created\", result:{record, selected_packages, workspace_path, sdk_setup[], next_step}, plan}; then call workspace_status and pubget. If creation fails midway flutree rolls back what it made."),
		mcplib.WithDestructiveHintAnnotation(false),
	}, createToolOptions()...)
	opts = append(opts,
		mcplib.WithBoolean("confirm", mcplib.Description("Must be true to create. Set it only after the user approved the plan returned by plan_create. Default false.")),
		mcplib.WithBoolean("sync_with_remote", mcplib.Description("Fetch each base branch from origin and branch from origin/<base> instead of the local ref (needs network). Default false.")),
		mcplib.WithBoolean("reuse_existing_branch", mcplib.Description("Reuse the branch when it already exists instead of failing. Default false.")),
	)
	return mcplib.NewTool("create_workspace", opts...)
}

// planView is the JSON shape of a dry plan.
type planView struct {
	Name             string        `json:"name"`
	ContainerPath    string        `json:"container_path"`
	Root             plannedView   `json:"root"`
	Packages         []plannedView `json:"packages"`
	OverridePath     string        `json:"override_path,omitempty"`
	OverrideContent  string        `json:"override_content,omitempty"`
	WorkspaceFile    string        `json:"workspace_file,omitempty"`
	WorkspaceFolders []string      `json:"workspace_folders,omitempty"`
	CopyRootFiles    []string      `json:"copy_root_files"`
	Notes            []string      `json:"notes,omitempty"`
}

type plannedView struct {
	Role        string `json:"role"`
	RepoName    string `json:"repo_name"`
	PackageName string `json:"package_name"`
	RepoRoot    string `json:"repo_root"`
	Path        string `json:"path"`
	Branch      string `json:"branch"`
	BaseBranch  string `json:"base_branch"`
}

func toPlannedView(p domain.PlannedWorktree) plannedView {
	return plannedView{
		Role:        p.Role,
		RepoName:    p.Repo.Name,
		PackageName: p.Repo.PackageName,
		RepoRoot:    p.Repo.RepoRoot,
		Path:        p.Path,
		Branch:      p.Branch,
		BaseBranch:  p.BaseBranch,
	}
}

func toPlanView(plan domain.CreateDryPlan) planView {
	view := planView{
		Name:             plan.NormalizedName,
		ContainerPath:    plan.ContainerPath,
		Root:             toPlannedView(plan.Root),
		Packages:         make([]plannedView, 0, len(plan.Packages)),
		OverridePath:     plan.OverridePath,
		OverrideContent:  plan.OverrideContent,
		WorkspaceFile:    plan.WorkspacePath,
		WorkspaceFolders: plan.WorkspaceFolders,
		CopyRootFiles:    plan.RootFiles,
	}
	for _, pkg := range plan.Packages {
		view.Packages = append(view.Packages, toPlannedView(pkg))
	}
	if len(plan.Packages) == 0 {
		view.Notes = append(view.Notes, "Root-only workspace: no pubspec_overrides.yaml is generated.")
	}
	return view
}

// buildCreateInput turns plan_create / create_workspace arguments into a
// CreateInput, applying the config defaults for scope and workspace
// generation. Missing user decisions are reported as input errors that tell the
// agent to ask the user, never defaulted.
func buildCreateInput(svc MCPServices, req mcplib.CallToolRequest) (domain.CreateInput, *mcpCallToolResult) {
	name := strings.TrimSpace(req.GetString("name", ""))
	if name == "" {
		return domain.CreateInput{}, missingArg("name", "Ask the user for the workspace name.")
	}
	rootRepo := strings.TrimSpace(req.GetString("root_repo", ""))
	if rootRepo == "" {
		return domain.CreateInput{}, missingArg("root_repo", "Ask the user which repository is the root; discover_repos lists candidates.")
	}
	baseBranch := strings.TrimSpace(req.GetString("base_branch", ""))
	if baseBranch == "" {
		return domain.CreateInput{}, missingArg("base_branch", "Ask the user which branch the root worktree starts from; list_branches lists candidates.")
	}

	packages := req.GetStringSlice("packages", nil)
	packageBase := getStringMap(req, "package_base")
	for _, pkg := range packages {
		if !hasPackageBase(packageBase, pkg) {
			return domain.CreateInput{}, inputError(
				"Missing base branch for package '"+pkg+"'.",
				"Ask the user for the base branch of '"+pkg+"' and pass it in package_base as {\""+filepath.Base(pkg)+"\": \"<branch>\"}.")
		}
	}

	scope := strings.TrimSpace(req.GetString("scope", ""))
	if scope == "" {
		if configured, err := svc.Config.Get("scope.root"); err == nil {
			scope = strings.TrimSpace(configured)
		}
	}
	if scope == "" && !allPathSelectors(append([]string{rootRepo}, packages...)) {
		return domain.CreateInput{}, inputError(
			"Missing required argument: scope.",
			"root_repo and packages are names, so a folder to search is needed. Pass scope, or use paths for every repository, or set scope.root with set_config.")
	}

	generate := true
	if _, given := req.GetArguments()["generate_workspace"]; given {
		generate = req.GetBool("generate_workspace", true)
	} else if configured, err := svc.Config.Get("workspace.generate"); err == nil {
		generate = configured != "false"
	}

	return domain.CreateInput{
		Name:              name,
		Branch:            req.GetString("branch", ""),
		BaseBranch:        baseBranch,
		ExecutionScope:    scope,
		RootSelector:      rootRepo,
		NoPackage:         len(packages) == 0,
		PackageSelectors:  packages,
		PackageBaseBranch: packageBase,
		RootFiles:         req.GetStringSlice("copy_root_files", nil),
		GenerateWorkspace: generate,
		Yes:               true,
		NonInteractive:    true,
	}, nil
}

// hasPackageBase reports whether packageBase has an entry for selector, by the
// selector itself or by the folder name it ends with.
func hasPackageBase(packageBase map[string]string, selector string) bool {
	sel := strings.TrimSpace(selector)
	if _, ok := packageBase[sel]; ok {
		return true
	}
	_, ok := packageBase[filepath.Base(filepath.Clean(sel))]
	return ok
}

// allPathSelectors reports whether every selector is a path (so no discovery
// scope is needed): it starts with ~ or . or contains a path separator.
func allPathSelectors(selectors []string) bool {
	for _, sel := range selectors {
		sel = strings.TrimSpace(sel)
		if !(strings.HasPrefix(sel, "~") || strings.HasPrefix(sel, ".") || strings.ContainsRune(sel, filepath.Separator)) {
			return false
		}
	}
	return true
}

func makePlanCreateHandler(svc MCPServices) toolHandler {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return withPanicRecovery(func() (*mcpCallToolResult, error) {
			req, declined := elicitCreateInputs(ctx, svc, req, map[string]any{"planned": false})
			if declined != nil {
				return declined, nil
			}
			input, bad := buildCreateInput(svc, req)
			if bad != nil {
				return bad, nil
			}
			plan, err := svc.Create.BuildDryPlan(input)
			if err != nil {
				return toToolError(err), nil
			}
			return marshalStructured(toPlanView(plan))
		})
	}
}

// describePlanForUser renders what create_workspace will create, for the
// confirmation question.
func describePlanForUser(v planView) string {
	var b strings.Builder
	b.WriteString("Create workspace '" + v.Name + "' in " + v.ContainerPath + "?\n")
	b.WriteString("- root: " + v.Root.Path + " (new branch " + v.Root.Branch + " from " + v.Root.BaseBranch + ")\n")
	for _, pkg := range v.Packages {
		b.WriteString("- package: " + pkg.Path + " (new branch " + pkg.Branch + " from " + pkg.BaseBranch + ")\n")
	}
	if v.OverridePath != "" {
		b.WriteString("- generated: " + v.OverridePath + "\n")
	}
	if v.WorkspaceFile != "" {
		b.WriteString("- generated: " + v.WorkspaceFile + "\n")
	}
	b.WriteString("Nothing existing is deleted; the original repositories only gain the new branches and worktree entries.")
	return b.String()
}

func makeCreateWorkspaceHandler(svc MCPServices) toolHandler {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return withPanicRecovery(func() (*mcpCallToolResult, error) {
			req, declined := elicitCreateInputs(ctx, svc, req, map[string]any{"created": false})
			if declined != nil {
				return declined, nil
			}
			input, bad := buildCreateInput(svc, req)
			if bad != nil {
				return bad, nil
			}
			plan, err := svc.Create.BuildDryPlan(input)
			if err != nil {
				return toToolError(err), nil
			}
			view := toPlanView(plan)
			instruction := "Nothing was created. Show this plan to the user and ask them to approve it. Only after they explicitly say yes, call create_workspace again with the same arguments and confirm=true."
			if res, ok := requiresConfirmation(ctx, req, confirmSpec{
				Message:     describePlanForUser(view),
				Pending:     map[string]any{"created": false, "status": "confirmation_required", "plan": view},
				Instruction: instruction,
			}); !ok {
				return res, nil
			}
			result, err := svc.Create.Apply(plan, domain.CreateApplyOptions{
				NonInteractive:      true,
				ReuseExistingBranch: req.GetBool("reuse_existing_branch", false),
				SyncWithRemote:      req.GetBool("sync_with_remote", false),
			})
			if err != nil {
				return toToolError(err), nil
			}
			return marshalStructured(map[string]any{
				"created": true,
				"status":  "created",
				"result":  result,
				"plan":    view,
			})
		})
	}
}

// ── workspace_status ─────────────────────────────────────────────────────────

func buildWorkspaceStatusTool() mcplib.Tool {
	return mcplib.NewTool("workspace_status",
		mcplib.WithDescription("Inspect one existing workspace: for the root and every package its path, branch, whether the folder still exists and whether it has uncommitted changes (dirty), plus the workspace file and whether pubspec_overrides.yaml exists with its parsed entries. Use it after creating or changing a workspace, and BEFORE complete_worktree to see what would be lost. For each worktree it also reports base_branch plus ahead (commits in the worktree HEAD not in the base) and behind (commits in the base not in HEAD); when the registry has no base for a worktree (created by an older flutree) or the base no longer resolves, ahead/behind are omitted and ahead_behind_note says why. Read-only. Fails with category precondition when the name is not registered."),
		mcplib.WithString("name", mcplib.Required(), mcplib.Description("Workspace name, as shown by list_worktrees.")),
		mcplib.WithReadOnlyHintAnnotation(true), mcplib.WithDestructiveHintAnnotation(false),
	)
}

func makeWorkspaceStatusHandler(svc MCPServices) toolHandler {
	return func(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return withPanicRecovery(func() (*mcpCallToolResult, error) {
			name := strings.TrimSpace(req.GetString("name", ""))
			if name == "" {
				return missingArg("name", "Pass the workspace name from list_worktrees."), nil
			}
			status, err := svc.Inspect.WorkspaceStatus(name)
			if err != nil {
				return toToolError(err), nil
			}
			return marshalStructured(status)
		})
	}
}
