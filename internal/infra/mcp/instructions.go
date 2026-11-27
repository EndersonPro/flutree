package mcp

// serverInstructions is sent to every client in the initialize result. It is
// the short version of guideMarkdown: enough for an agent to use the tools
// correctly without reading anything else.
const serverInstructions = `flutree manages multi-repository Flutter workspaces built from git worktrees.

MODEL
- A workspace is one ROOT worktree plus zero or more PACKAGE worktrees, all created from local Flutter repositories (a folder with .git and a pubspec.yaml that has a name).
- Layout: ~/Documents/worktrees/<name>/root/<root-repo>/ and ~/Documents/worktrees/<name>/packages/<package-repo>/. <name> is lowercased and slugified.
- Every worktree gets the same new branch, feature/<name> by default (override with branch). Each one starts from its own base branch.
- Your original repositories are never modified apart from the new branch and the git worktree entries.
- pubspec_overrides.yaml is generated in the root worktree when at least one package is selected. It points every package to its package worktree, and it also carries the dependency_overrides of the root pubspec.yaml and the plain path dependencies of the repos (relative paths rebased), so the whole graph resolves from the worktrees. pubspec.yaml is never edited.
- That file is listed in the root repository's .git/info/exclude (never committed; .gitignore is untouched).
- A VSCode <name>.code-workspace file with the root and package folders is written in <name>/ unless workspace.generate is false.
- Files matching .env and .env.* are copied from each source repo into its worktree (extra patterns: copy_root_files).
- After creation flutree runs the Flutter SDK manager the repo pins: fvm (.fvmrc) or puro (.puro.json). Config key sdk.manager = auto (default, fvm first), fvm, puro or none.
- A registry (~/Documents/worktrees/.worktrees_registry.json) tracks every workspace; list_worktrees and flutree://workspaces read it.
- Config keys: scope.root (default folder to discover repositories), workspace.generate (true/false), sdk.manager. Read them with get_config or flutree://config.

RECOMMENDED FLOW
1. Understand: read flutree://guide if you need details; call list_worktrees to see existing workspaces and get_config for scope.root.
2. Discover: discover_repos(scope) lists Flutter repos in a folder. inspect_repo(path) shows one repo (branch, SDK pin, path dependencies). list_branches(repo_path) lists local and origin/* branches.
3. ASK THE USER, never guess: the workspace name, which repo is the root, which repos are packages, and the base branch of the root and of EVERY package. Offer the branches from list_branches.
4. Plan: plan_create with those answers. It changes nothing and returns worktree paths, branches, the pubspec_overrides.yaml content and the workspace file.
5. Show the plan to the user and get an explicit yes.
6. Create: create_workspace with the same arguments plus confirm=true. Without confirm=true nothing is created and the plan is returned again.
7. Verify and prepare: workspace_status(name) for paths, branches, dirty state and overrides; pubget(name) to run pub get (packages first, then root).
8. Later: add_repo adds repositories to an existing workspace; complete_worktree(name) removes the worktrees (the local branches stay).

RULES
- Paths in results are absolute. Repo selectors accept a repo folder name, a package name or a path (absolute, ~ or relative to the server's cwd); a path may be outside scope.
- complete_worktree is destructive: it removes every worktree of the workspace and refuses dirty ones unless force=true. Check workspace_status first and ask the user before using force.
- create_worktree is the older one-shot create (no confirmation step); prefer plan_create + create_workspace.
- Errors are tool results with isError=true and a JSON body {category, message, hint, code}. Follow the hint; categories are input, precondition, git, persistence and unexpected.
`

// guideMarkdown is served as flutree://guide. It repeats the instructions with
// the detail and examples that would be too long for the initialize result.
const guideMarkdown = `# flutree guide for agents

flutree creates **workspaces**: a set of git worktrees from several local Flutter
repositories that share one feature branch and resolve each other through a
generated ` + "`pubspec_overrides.yaml`" + `.

## Concepts

| Term | Meaning |
| --- | --- |
| Root repo | The app you work on. Its worktree lives in ` + "`root/<repo>`" + `. |
| Package repos | Other Flutter repos the root depends on. Worktrees in ` + "`packages/<repo>`" + `. |
| Workspace | Root + packages, registered under one name. |
| Base branch | The branch/commit-ish a worktree branch starts from. Each repo has its own. |

Destination: ` + "`~/Documents/worktrees/<name>/`" + ` (the name is lowercased and slugified).

` + "```" + `
~/Documents/worktrees/feature-login/
  root/app/                 <- root worktree, branch feature/feature-login
    pubspec_overrides.yaml  <- generated, git-excluded
  packages/core/            <- package worktree, same branch name
  packages/ui/
  feature-login.code-workspace
` + "```" + `

The branch defaults to ` + "`feature/<name>`" + ` and is the same in every worktree. The
original repositories only gain that branch and the worktree entries.

## pubspec_overrides.yaml

Created in the root worktree when at least one package is selected (a root-only
workspace has none). A ` + "`pubspec_overrides.yaml`" + ` **replaces** the
` + "`dependency_overrides`" + ` of ` + "`pubspec.yaml`" + `, so flutree carries these into it, lowest
precedence first:

1. plain ` + "`path:`" + ` dependencies between the selected repos, remapped to worktrees;
2. the ` + "`dependency_overrides`" + ` already in the root ` + "`pubspec.yaml`" + ` (version, ` + "`path`" + `,
   ` + "`git`" + `, ` + "`hosted`" + `); relative paths that leave the repo are rebased to still resolve;
3. entries already in the worktree's ` + "`pubspec_overrides.yaml`" + `;
4. the selected package worktrees (they win on conflict).

` + "`pubspec.yaml`" + ` is never modified. The override file is listed in the root
repository's ` + "`.git/info/exclude`" + ` so it is never committed; ` + "`.gitignore`" + ` is untouched.

Paths are relative to the root worktree. Example (` + "`core`" + ` and ` + "`ui`" + ` selected):

` + "```yaml" + `
dependency_overrides:
  core:
    path: ../../packages/core
  ui:
    path: ../../packages/ui
` + "```" + `

## Other generated state

- ` + "`<name>.code-workspace`" + `: VSCode workspace with the root and package folders. Controlled by ` + "`workspace.generate`" + ` (default true).
- ` + "`.env`" + ` and ` + "`.env.*`" + ` of each source repo are copied into its worktree (` + "`copy_root_files`" + ` adds patterns).
- SDK setup: if a repo pins fvm (` + "`.fvmrc`" + `) or puro (` + "`.puro.json`" + `), flutree runs
  ` + "`fvm use <version>`" + ` or ` + "`puro use <env>`" + ` in its worktree. ` + "`sdk.manager`" + ` is ` + "`auto`" + ` (fvm
  first), ` + "`fvm`" + `, ` + "`puro`" + ` or ` + "`none`" + `. Failures are reported in ` + "`sdk_setup`" + ` and never undo the workspace.
- Registry: ` + "`~/Documents/worktrees/.worktrees_registry.json`" + `. Package worktrees are
  stored as ` + "`<name>__pkg__<repo>`" + ` records but are folded into their workspace by the tools.

## Configuration

| Key | Meaning |
| --- | --- |
| ` + "`scope.root`" + ` | Default folder searched for Flutter repos (empty = unset). |
| ` + "`workspace.generate`" + ` | Write the ` + "`.code-workspace`" + ` file (default ` + "`true`" + `). |
| ` + "`sdk.manager`" + ` | ` + "`auto`" + ` (default), ` + "`fvm`" + `, ` + "`puro`" + `, ` + "`none`" + `. |

## Recommended flow

1. ` + "`get_config`" + ` / ` + "`list_worktrees`" + ` to see defaults and existing workspaces.
2. ` + "`discover_repos`" + ` (scope) then ` + "`inspect_repo`" + ` and ` + "`list_branches`" + ` for candidates.
3. **Ask the user**: name, root repo, packages, base branch of the root and of each package.
4. ` + "`plan_create`" + ` (no side effects). Show the plan.
5. After an explicit yes: ` + "`create_workspace`" + ` with ` + "`confirm: true`" + `.
6. ` + "`workspace_status`" + ` and ` + "`pubget`" + `.
7. ` + "`add_repo`" + ` to attach more repositories later.
8. ` + "`complete_worktree`" + ` when the work is merged: removes the worktrees, keeps the local branches.

Example:

` + "```json" + `
// discover_repos
{"scope": "/Users/me/code"}
// plan_create (after asking the user)
{"name": "feature-login", "root_repo": "app", "packages": ["core", "ui"],
 "base_branch": "develop", "package_base": {"core": "main", "ui": "develop"},
 "scope": "/Users/me/code"}
// create_workspace: same arguments plus
{"confirm": true}
` + "```" + `

## Selectors

` + "`root_repo`" + ` and ` + "`packages`" + ` entries are a repo folder name, a package name, or a path
(absolute, ` + "`~/x`" + ` or relative to the server's working directory). A path to a
Flutter repo outside ` + "`scope`" + ` is accepted, so repositories may live in unrelated
folders. ` + "`package_base`" + ` keys are the repo folder names (or repo paths).

## Errors

Failures are tool results with ` + "`isError: true`" + ` and a JSON body
` + "`{category, message, hint, code}`" + `. Categories: ` + "`input`" + ` (fix the arguments),
` + "`precondition`" + ` (state is not ready, for example the workspace already exists),
` + "`git`" + `, ` + "`persistence`" + `, ` + "`unexpected`" + `. Always read the hint.

## Safety

- ` + "`complete_worktree`" + ` removes every worktree of a workspace. It refuses worktrees with
  uncommitted changes unless ` + "`force: true`" + `; check ` + "`workspace_status`" + ` and ask first.
- ` + "`create_workspace`" + ` and ` + "`plan_create`" + ` never touch the original repositories except
  for adding the feature branch when applied.
`
