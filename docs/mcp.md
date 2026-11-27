# MCP server

`flutree mcp serve` runs a [Model Context Protocol](https://modelcontextprotocol.io) server over
stdio so AI agents can discover repositories, plan a workspace, create it and manage it. Install it
into an agent client with `flutree mcp install` (see the README).

stdout is the JSON-RPC transport: `--json` is rejected and nothing else is written there.

## Capabilities

| Capability | Content |
| --- | --- |
| `instructions` | A ~45 line summary of the model and the recommended flow, sent in the `initialize` result. |
| Resources | `flutree://guide`, `flutree://workspaces`, `flutree://config` and the template `flutree://workspace/{name}`. |
| Prompts | `create_workspace_flow(name?, root_path?)`, `add_package_flow(workspace)`, `finish_workspace_flow(workspace)`. |
| Elicitation | Asks the user directly when the client declares the `elicitation` capability; see [Asking the user](#asking-the-user). |
| Tools | 15 tools: 9 original ones plus `discover_repos`, `inspect_repo`, `list_branches`, `plan_create`, `create_workspace`, `workspace_status`. |

Tool results are JSON in a text content block. The new tools also return the same object as
`structuredContent`. Errors are tool results with `isError: true` and a JSON body
`{"category", "message", "hint", "code"}`; categories are `input`, `precondition`, `git`,
`persistence` and `unexpected`.

## Recommended flow

1. `get_config` / `list_worktrees` (or the resources) to see defaults and existing workspaces.
2. `discover_repos(scope)`, then `inspect_repo(path)` and `list_branches(repo_path)`.
3. Ask the user: workspace name, root repo, packages, base branch of the root and of every package.
4. `plan_create(...)` returns the plan and changes nothing. Show it to the user.
5. After an explicit yes, `create_workspace(..., confirm=true)`.
6. `workspace_status(name)` and `pubget(name)`.
7. `add_repo` to attach more repositories; `complete_worktree` when the work is done.

## Asking the user

Two kinds of decisions belong to the user, never to the agent: inputs the agent does not have
(base branch, packages) and approval before state-changing actions.

| Where | What is asked |
| --- | --- |
| `plan_create`, `create_workspace` | A missing `base_branch` (a string field; the repo's local and `origin/*` branches become an `enum`, the current branch the default). `packages` when the argument is omitted (the repos discovered near the root, except the root). A base branch for each package without one in `package_base`. |
| `create_workspace` without `confirm=true` | A yes/no confirmation that lists the root and package worktrees, branches and generated files. |
| `complete_worktree` | Always (even with `confirm=true`): a yes/no confirmation that lists every worktree folder to delete and the branches that are kept, marking dirty worktrees. |
| `pubget` with `force=true` | A yes/no confirmation. A plain `pubget` is not gated. |

The confirmation gate is one function, `requiresConfirmation`, shared by all of them.

**Client with elicitation.** The server sends `elicitation/create` with a form schema and waits up
to 2 minutes. `accept` with data continues. `decline`, `cancel`, a timeout or `accept` with
`confirm=false` return a normal result (not an error) with `user_declined: true`,
`status: "declined_by_user"` or `"cancelled_by_user"` and `message: "... Nothing was executed ..."`;
the agent must not retry on its own. For `create_workspace`/`plan_create` the form questions are
only asked for what is missing, and `confirm=true` from the agent skips the create confirmation
when every input is present.

**Client without elicitation.** Nothing is executed. Missing inputs stay input errors with a hint
(unchanged). A missing confirmation returns a normal result:

```json
{
  "created": false,
  "status": "confirmation_required",
  "needs_user_input": true,
  "questions": [{"field": "confirm", "type": "boolean", "prompt": "Create workspace 'x' ...?"}],
  "instruction": "Nothing was created. Show this plan to the user ... call create_workspace again with the same arguments and confirm=true.",
  "plan": {}
}
```

`questions[]` entries have `field`, `prompt`, `type` and, when known, `options`. The agent asks the
user and calls the tool again with the answers or `confirm=true`. `complete_worktree` accepts
`confirm=true` for this path (it returns `completed: false`; `pubget` returns `executed: false`).

## Prompts

| Prompt | Arguments | Steps |
| --- | --- | --- |
| `create_workspace_flow` | `name?`, `root_path?` | `list_worktrees`, `get_config`, `discover_repos`, `inspect_repo`, ask the user (name, root, packages, base branches via `list_branches`), `plan_create`, show the plan, `create_workspace(confirm=true)`, `workspace_status`, `pubget`. |
| `add_package_flow` | `workspace` (required) | `workspace_status`, `discover_repos`, ask which repos and base branches, `add_repo`, `workspace_status`, `pubget`. |
| `finish_workspace_flow` | `workspace` (required) | `workspace_status`, flag dirty worktrees, ask for approval, `complete_worktree`, `list_worktrees`. |

## Resources

| URI | MIME | Content |
| --- | --- | --- |
| `flutree://guide` | `text/markdown` | Layout, branches, `pubspec_overrides.yaml`, registry, SDK setup, config, flow and examples. |
| `flutree://workspaces` | `application/json` | Array of `{name, branch, path, repo_root, status, package_count}` (same rows as `list_worktrees` with `global_scope=true`). |
| `flutree://workspace/{name}` | `application/json` | The `workspace_status` object for `{name}`. An unknown name is a JSON-RPC error. |
| `flutree://config` | `application/json` | `{"scope.root", "workspace.generate", "sdk.manager"}` as strings (effective values; `scope.root` is empty when unset). |

## Tools

All paths in results are absolute, except the entries of `pubspec_overrides.yaml`, which are relative
to the root worktree.

### discover_repos

Read-only. Flutter repositories under a folder.

- Arguments: `scope` (string, required): folder searched recursively.
- Result: `{"repos": [{"name", "package_name", "repo_root", "current_branch"}]}`.
- Errors: `precondition` when the folder is missing or holds no Flutter repository; `input` without `scope`.

### inspect_repo

Read-only. Describes one repository.

- Arguments: `path` (string, required).
- Result: `{path, is_flutter, reason?, name, package_name, repo_root, current_branch, local_branches[], sdk_pin: {fvm_version?, puro_env?}, path_dependencies: {<dependency>: <path as written>}}`.
  `path_dependencies` covers `dependencies` and `dev_dependencies` entries with a `path:`; hosted and git dependencies are not listed.
- A folder that is not a Flutter repo returns `is_flutter: false` and a `reason`. A path that does not exist is an `input` error.

### list_branches

Read-only. Local refs only (no fetch), so remote branches can be stale.

- Arguments: `repo_path` (string, required).
- Result: `{repo_path, current, local[], remote[]}`; `remote` holds `origin/<branch>` names without `origin/HEAD`.
- Errors: `input` for a missing path, `precondition` when it is not a git repository.

### plan_create

Read-only dry run of a workspace creation. Validates selectors, that the target folder is free and
that every package base branch exists, then returns the plan.

- Arguments:
  - `name` (required), `root_repo` (required), `base_branch` (required);
  - `packages` (string array; omit for a root-only workspace), `package_base` (object, required for every entry of `packages`, keyed by folder name or repo path);
  - `scope` (defaults to the configured `scope.root`; not needed when every selector is a path);
  - `branch` (default `feature/<name>`), `generate_workspace` (default: the `workspace.generate` config), `copy_root_files` (extra patterns; `.env` and `.env.*` are always copied).
- Selectors (`root_repo`, `packages`) are a folder name, a package name or a path (absolute, `~`, or relative to the server's working directory), as in the CLI.
- Missing decisions are `input` errors whose hint tells the agent to ask the user: a missing `base_branch` or a package without a `package_base` entry is never defaulted.
- Result: `{name, container_path, root, packages[], override_path, override_content, workspace_file, workspace_folders, copy_root_files, notes?}` where `root` and each package are `{role, repo_name, package_name, repo_root, path, branch, base_branch}`.
- No directory, branch, worktree or registry entry is created. (Like every flutree command, reading the configuration may create the default config file.)

### create_workspace

Same arguments as `plan_create`, plus:

- `confirm` (bool, default false): creates only when true.
- `sync_with_remote` (bool): fetch each base branch from origin and branch from `origin/<base>`.
- `reuse_existing_branch` (bool).

Without `confirm=true` nothing is created and the result is
`{"created": false, "status": "confirmation_required", "message", "plan"}`. With it, the workspace is
created exactly as `flutree create` does (worktrees, `pubspec_overrides.yaml`, `.git/info/exclude`,
registry, optional `.code-workspace`, fvm/puro setup, rollback on failure) and the result is
`{"created": true, "status": "created", "result": {record, selected_packages, workspace_path, sdk_setup[], next_step}, "plan"}`.

### workspace_status

Read-only state of one workspace.

- Arguments: `name` (required).
- Result: `{name, branch, status, container_path, workspace_file?, root, packages[], pubspec_overrides}`.
  `root` and each package are `{name, role, path, repo_root, branch, exists, dirty}`;
  `pubspec_overrides` is `{path, exists, entries: {<package>: <spec>}}`.
- Ahead/behind versus the base branch is not reported: the registry does not store the base each worktree was created from.
- Errors: `precondition` when the workspace is not registered.

### Existing tools

Names and arguments are unchanged apart from the new optional `confirm` on `complete_worktree` and `pubget`; descriptions now say when to use them.

| Tool | Notes |
| --- | --- |
| `list_worktrees` | `show_all`, `global_scope`. Read-only. |
| `create_worktree` | Legacy one-shot create without a confirmation step; prefer `plan_create` + `create_workspace`. |
| `add_repo` | `workspace_name`, `repos`, `scope`, `package_branch_source`, `package_base`, `sync_policy`, `reuse_existing_branch`, `copy_root_files`. |
| `complete_worktree` | Destructive: removes the worktrees (branches stay). Refuses dirty worktrees unless `force`. Asks the user first (elicitation), or needs `confirm=true` after the agent asked. Check `workspace_status` first. |
| `pubget` | `name`, `force`, `confirm` (only needed with `force=true`). |
| `clean_worktree` | Acts on the managed worktree containing the server's working directory. |
| `get_config` / `set_config` | Keys `scope.root`, `workspace.generate`, `sdk.manager`. |
| `get_version` | Binary version. |
