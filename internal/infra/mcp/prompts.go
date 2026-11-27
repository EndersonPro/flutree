package mcp

import (
	"context"
	"fmt"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// This file registers the MCP prompts: ready-made conversations that walk an
// agent through the three main flows with the real tools, resources and
// questions. Each prompt returns one user message with step-by-step guidance.

func registerPrompts(s *mcpserver.MCPServer) {
	s.AddPrompt(mcplib.NewPrompt("create_workspace_flow",
		mcplib.WithPromptDescription("Guide for creating a new flutree workspace (root + package worktrees) step by step: discover, ask the user, plan, confirm, create, verify."),
		mcplib.WithArgument("name", mcplib.ArgumentDescription("Workspace name, when the user already gave one.")),
		mcplib.WithArgument("root_path", mcplib.ArgumentDescription("Path of the root repository, when the user already gave one.")),
	), createWorkspaceFlowPrompt)

	s.AddPrompt(mcplib.NewPrompt("add_package_flow",
		mcplib.WithPromptDescription("Guide for adding more package repositories to an existing workspace with add_repo."),
		mcplib.WithArgument("workspace", mcplib.RequiredArgument(), mcplib.ArgumentDescription("Name of the existing workspace, as shown by list_worktrees.")),
	), addPackageFlowPrompt)

	s.AddPrompt(mcplib.NewPrompt("finish_workspace_flow",
		mcplib.WithPromptDescription("Guide for finishing a workspace safely: check dirty worktrees, get the user's approval, then complete_worktree."),
		mcplib.WithArgument("workspace", mcplib.RequiredArgument(), mcplib.ArgumentDescription("Name of the workspace to finish, as shown by list_worktrees.")),
	), finishWorkspaceFlowPrompt)
}

func promptResult(description, text string) *mcplib.GetPromptResult {
	return mcplib.NewGetPromptResult(description, []mcplib.PromptMessage{
		mcplib.NewPromptMessage(mcplib.RoleUser, mcplib.NewTextContent(text)),
	})
}

func requirePromptArg(req mcplib.GetPromptRequest, name string) (string, error) {
	v := strings.TrimSpace(req.Params.Arguments[name])
	if v == "" {
		return "", fmt.Errorf("prompt %q requires the argument %q", req.Params.Name, name)
	}
	return v, nil
}

func createWorkspaceFlowPrompt(_ context.Context, req mcplib.GetPromptRequest) (*mcplib.GetPromptResult, error) {
	name := strings.TrimSpace(req.Params.Arguments["name"])
	root := strings.TrimSpace(req.Params.Arguments["root_path"])

	var known []string
	if name != "" {
		known = append(known, fmt.Sprintf("- The user already chose the workspace name: %q (do not ask for it again).", name))
	}
	if root != "" {
		known = append(known, fmt.Sprintf("- The user already chose the root repository: %q (do not ask for it again).", root))
	}
	knownText := ""
	if len(known) > 0 {
		knownText = "\nAlready decided:\n" + strings.Join(known, "\n") + "\n"
	}
	nameArg, rootArg := "<name>", "<root repo>"
	if name != "" {
		nameArg = name
	}
	if root != "" {
		rootArg = root
	}

	text := fmt.Sprintf(`You are creating a new flutree workspace: one root worktree plus zero or more package worktrees on a shared feature branch. Follow these steps in order and never guess a decision that belongs to the user.
%s
1. Understand. Read the resource flutree://guide if you have not yet. Call list_worktrees(global_scope=true) to see which workspaces exist, so the name is free. Call get_config(key="scope.root") for the default folder to search for repositories.
2. Discover. Call discover_repos(scope=<scope.root or the folder the user names>) to list the Flutter repositories. Call inspect_repo(path=<repo_root>) on the root candidate: its path_dependencies show which other repositories the root needs as packages.
3. Ask the user. One question at a time, offering real choices:
   - the workspace name (if not decided);
   - which repository is the root (if not decided);
   - which repositories are packages (offer path_dependencies matches first; none is valid);
   - the base branch of the root and of EVERY package. Call list_branches(repo_path=<repo_root>) for each repo and offer those branches.
   If a tool or the client asks the user directly, that is the same question: do not repeat it.
4. Plan. Call plan_create(name=%q, root_repo=%q, base_branch=<root base>, packages=[...], package_base={"<package folder>": "<base branch>"}, scope=<folder>). It changes nothing. It returns worktree paths, branches, the full pubspec_overrides.yaml and the workspace file.
5. Show the plan to the user in plain language and wait for an explicit yes. If they want changes, go back to step 3 and plan again.
6. Create. Call create_workspace with the same arguments plus confirm=true. If the result has needs_user_input=true, nothing was created: ask the questions it lists, then call again. If the result says the user declined or cancelled, stop and ask what they want to do; never retry on your own.
7. Verify. Call workspace_status(name=%q) to check paths, branches and pubspec_overrides.yaml, then pubget(name=%q) to resolve dependencies. Report the workspace paths to the user.`,
		knownText, nameArg, rootArg, nameArg, nameArg)

	return promptResult("Create a flutree workspace step by step", text), nil
}

func addPackageFlowPrompt(_ context.Context, req mcplib.GetPromptRequest) (*mcplib.GetPromptResult, error) {
	ws, err := requirePromptArg(req, "workspace")
	if err != nil {
		return nil, err
	}
	text := fmt.Sprintf(`You are adding package repositories to the existing flutree workspace %[1]q. Follow these steps in order and never guess a decision that belongs to the user.

1. Inspect the workspace. Call workspace_status(name=%[1]q) (or read flutree://workspace/%[1]s): note its branch, its current packages and any dirty worktree. If it is unknown, call list_worktrees(global_scope=true) and ask the user which workspace they mean.
2. Discover. Call get_config(key="scope.root"), then discover_repos(scope=<that folder or the one the user names>). Do not offer repositories that are already in the workspace.
3. Ask the user which repositories to add. Call inspect_repo on candidates; the root's path_dependencies hint at what is missing.
4. Ask the user the base branch of EACH repository to add. Call list_branches(repo_path=<repo_root>) for each and offer those branches.
5. Summarize what will happen (new worktrees on the workspace branch, pubspec_overrides.yaml regenerated) and wait for an explicit yes: add_repo has no dry run.
6. Add. Call add_repo(workspace_name=%[1]q, repos=[<selectors>], scope=<folder>, package_base={"<repo folder>": "<base branch>"}).
7. Verify. Call workspace_status(name=%[1]q) to confirm the new package worktrees and the regenerated overrides, then pubget(name=%[1]q).`, ws)
	return promptResult("Add packages to flutree workspace "+ws, text), nil
}

func finishWorkspaceFlowPrompt(_ context.Context, req mcplib.GetPromptRequest) (*mcplib.GetPromptResult, error) {
	ws, err := requirePromptArg(req, "workspace")
	if err != nil {
		return nil, err
	}
	text := fmt.Sprintf(`You are finishing the flutree workspace %[1]q. This DELETES its worktree folders from disk (the local branches are kept), so be careful and never skip the user's approval.

1. Inspect. Call workspace_status(name=%[1]q). List for the user every worktree path and branch, and flag each one with dirty=true (uncommitted changes).
2. Dirty worktrees. If any is dirty, tell the user exactly what is at risk and ask whether to commit or stash first. Never use force=true unless the user explicitly says to discard those changes.
3. Branches. Remind the user that the branches (%[2]s by default) stay in the original repositories; unpushed work lives there.
4. Ask for explicit approval to remove the workspace. The client may ask the user directly when you call complete_worktree; that answer counts as the approval and you must not ask twice.
5. Complete. Call complete_worktree(name=%[1]q, confirm=true) after the user approved (add force=true only if they accepted losing uncommitted changes). If the result has needs_user_input=true or says the user declined or cancelled, nothing was removed: stop and ask what the user wants.
6. Verify. Call list_worktrees(global_scope=true): the workspace should now show status completed. Tell the user which branches remain.`, ws, "feature/<name>")
	return promptResult("Finish flutree workspace "+ws, text), nil
}
