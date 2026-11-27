<p align="center"><img src="assets/flutree-icon.png" alt="flutree logo" width="300" /></p>

`flutree` is a Go CLI that manages Git worktrees for multi-repo Flutter workflows: one workspace folder with a root app and its local packages, wired together with `pubspec_overrides.yaml`.

It has native AI agent support: `flutree mcp install` plugs it into Claude Code, Cursor, VS Code, Codex, OpenCode, Gemini CLI, Claude Desktop and Windsurf through the Model Context Protocol (MCP).

[![tests](https://github.com/EndersonPro/flutree/actions/workflows/tests.yml/badge.svg)](https://github.com/EndersonPro/flutree/actions/workflows/tests.yml)
[![Release](https://img.shields.io/github/v/release/EndersonPro/flutree?color=green&style=flat-square)](https://github.com/EndersonPro/flutree/releases/latest)
[![License](https://img.shields.io/github/license/EndersonPro/flutree?color=blue&style=flat-square)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/EndersonPro/flutree?style=flat-square)](go.mod)
[![Last Commit](https://img.shields.io/github/last-commit/EndersonPro/flutree?style=flat-square)](https://github.com/EndersonPro/flutree/commits/main)

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Set up flutree for your AI agent (MCP)](#set-up-flutree-for-your-ai-agent-mcp)
- [Quickstart (CLI)](#quickstart-cli)
- [Commands Reference](#commands-reference)
- [Workspace layout](#workspace-layout)
- [Non-interactive behavior and JSON output](#non-interactive-behavior-and-json-output)
- [Exit Codes](#exit-codes)
- [Registry](#registry)
- [Testing](#testing)
- [Project structure](#project-structure)
- [Contributing](#contributing)
- [License](#license)
- [Acknowledgments](#acknowledgments)

## Features

- **AI-native (MCP)**: a built-in MCP server (`flutree mcp serve`) with 15 tools, resources and prompts. The agent discovers your repos, plans a workspace, asks you for the missing decisions and creates it only after you confirm.
- **Interactive create**: a wizard that works from any directory, with a folder navigator, filter-as-you-type lists and a dry plan before anything is written.
- **Overrides and dependency graph**: a `pubspec_overrides.yaml` in the root worktree and in every package worktree that depends on other selected packages. They keep the overrides already in your `pubspec.yaml` and resolve the whole dependency graph from the selected worktrees.
- **Multi-repo workspaces**: root and package worktrees grouped in one folder, with an optional VSCode `.code-workspace`.
- **SDK setup**: runs `fvm` or `puro` in every new worktree so each one uses the right Flutter SDK.
- **`--json` everywhere**: machine-readable output and JSON error envelopes for scripts and CI.
- **Shell completion**: `flutree completion <bash|zsh|fish|powershell>`.

## Installation

### Homebrew (macOS arm64)

```bash
brew tap EndersonPro/flutree
brew install EndersonPro/flutree/flutree
```

Upgrade:

```bash
brew update
brew upgrade flutree
# or via the flutree helper command
flutree update
```

### Windows (Scoop)

```bash
scoop bucket add flutree https://github.com/EndersonPro/scoop-flutree
scoop install flutree
```

Upgrade:

```bash
scoop update flutree
```

### Release archives (Linux, macOS, Windows)

Each [release](https://github.com/EndersonPro/flutree/releases/latest) ships these archives plus a `checksums.txt` (SHA-256):

| Platform | Archive |
|----------|---------|
| Linux amd64 | `flutree-<version>-linux-amd64.tar.gz` |
| Linux arm64 | `flutree-<version>-linux-arm64.tar.gz` |
| macOS arm64 | `flutree-<version>-macos-arm64.tar.gz` |
| Windows amd64 | `flutree-<version>-windows-amd64.zip` |
| Windows arm64 | `flutree-<version>-windows-arm64.zip` |

Linux example (replace the version and architecture):

```bash
VERSION=2.0.0
ARCHIVE=flutree-${VERSION}-linux-amd64.tar.gz
curl -LO https://github.com/EndersonPro/flutree/releases/download/v${VERSION}/${ARCHIVE}
curl -LO https://github.com/EndersonPro/flutree/releases/download/v${VERSION}/checksums.txt

# verify the download (prints "<archive>: OK")
sha256sum --ignore-missing -c checksums.txt

# extract and put the binary on your PATH
tar -xzf ${ARCHIVE}
sudo install -m 0755 flutree /usr/local/bin/flutree
flutree --version
```

On macOS use `shasum -a 256 -c checksums.txt` instead of `sha256sum`. On Windows, extract the `.zip` and move `flutree.exe` to a folder that is on your `PATH`.

### Build from source

Requires Go `>=1.25` (see `go.mod`) and Git.

```bash
go build -o ./flutree ./cmd/flutree
./flutree --help
```

Note: `go install github.com/EndersonPro/flutree/cmd/flutree@<version>` does not work for v2 releases, because the module path has no `/v2` suffix. Use one of the methods above.

Requirements at runtime: Git available in `PATH`. `fvm` or `puro` are optional (see [SDK setup](#sdk-setup)).

## Set up flutree for your AI agent (MCP)

flutree includes an MCP server, so an AI coding agent can create and manage your Flutter workspaces for you. Setup takes one command.

### Step 1: install the binary at a stable path

The agent's config stores the absolute path of the `flutree` binary. Install it with Homebrew, Scoop or a release archive (see [Installation](#installation)) so the path stays valid. Do not install from a temporary location or with `go run`: the path in the agent's config would break as soon as the binary disappears, and `flutree mcp doctor` warns about it (`[WARN] binary: binary lives in a temporary directory`).

### Step 2: register flutree in your agent

```bash
flutree mcp install                      # auto-detect installed clients
flutree mcp install --client claude-code # one or more clients: --client cursor,codex
flutree mcp install --scope project      # write the repo-shared config of the current directory
flutree mcp install --dry-run            # preview target file and entry, write nothing
```

The installer is non-destructive: existing keys and other servers are preserved, and an existing flutree entry is kept unless you pass `--force`.

### Step 3: check the installation

```bash
flutree mcp doctor
```

| Line | Meaning |
|------|---------|
| `[OK]` | The check passed: binary resolved, entry present with an existing command, or handshake succeeded. |
| `[-]` | Informational: the client is not detected, or it is detected but has no flutree entry yet. |
| `[WARN]` | A real warning, for example the binary lives in a temporary directory and may disappear. |
| `[ERR]` | A detected client's config cannot be read, the entry's command path is broken, or the live `mcp serve` handshake failed. `doctor` exits `1`. |

### Step 4: restart or reload the agent

Restart the agent (or reload its MCP servers) so it picks up the new entry. See the per-client notes below.

### Step 5: try it

Ask your agent, in plain language:

- "Create a flutree workspace feature-login with root app and packages core and ui"
- "Show the status of workspace feature-login"
- "Finish workspace feature-login"

For the first prompt the agent discovers your repositories (`discover_repos`, `inspect_repo`, `list_branches`), asks you for what it does not know (base branches, packages), calls `plan_create` and shows you the plan, and only after you confirm calls `create_workspace`. For the second it calls `workspace_status`; for the third it checks the status, asks for your approval and calls `complete_worktree`.

### Claude Code

```bash
flutree mcp install --client claude-code
```

- Writes `~/.claude.json` (user) or `.mcp.json` in the current directory (`--scope project`), key `mcpServers.flutree`.
- Restart Claude Code, then run `/mcp` to see `flutree` listed as connected.
- Caveat: Claude Code may ask you to approve a project-scoped server from `.mcp.json` the first time.

### Cursor

```bash
flutree mcp install --client cursor
```

- Writes `~/.cursor/mcp.json` (user) or `.cursor/mcp.json` (`--scope project`), key `mcpServers.flutree`.
- Restart Cursor and check Settings, MCP, where `flutree` should appear with its tools.
- Caveat: Cursor is detected by its config directory (`~/.cursor`) or a `cursor` binary on `PATH`.

### VS Code (Copilot)

```bash
flutree mcp install --client vscode
```

- Writes `<VS Code user dir>/User/mcp.json` (user) or `.vscode/mcp.json` (`--scope project`), key `servers.flutree`.
- In VS Code run "MCP: List Servers" from the Command Palette to see and start `flutree`; use it from Copilot Chat in agent mode.
- Caveat: `mcp.json` may contain JSONC comments or trailing commas. The installer refuses to rewrite such a file unless you pass `--force`, which strips the comments. The per-OS user directory is not documented by VS Code, so verify the path with `--dry-run`.

### Codex

```bash
flutree mcp install --client codex
```

- Writes `~/.codex/config.toml` (user) or `.codex/config.toml` (`--scope project`), table `[mcp_servers.flutree]`.
- Restart Codex; the server starts with the session.
- Caveat: TOML comments are not preserved on the round-trip, so a config that has comments needs `--force`.

### OpenCode

```bash
flutree mcp install --client opencode
```

- Writes `~/.config/opencode/opencode.json` (user) or `opencode.json` (`--scope project`).
- Restart OpenCode and check that `flutree` is listed among the MCP servers.
- Caveat: new files use the `mcp.servers.flutree` layout; if your file already uses the legacy `mcp.flutree` layout, that layout is kept and uninstall and doctor read both.

### Gemini CLI

```bash
flutree mcp install --client gemini
```

- Writes `~/.gemini/settings.json` (user) or `.gemini/settings.json` (`--scope project`), key `mcpServers.flutree`.
- Start `gemini` again and run `/mcp` to list the server and its tools.
- Caveat: none known beyond the usual restart.

### Claude Desktop

```bash
flutree mcp install --client claude-desktop
```

- Writes `claude_desktop_config.json` in the Claude app folder: macOS `~/Library/Application Support/Claude/`, Windows `%APPDATA%\Claude\`, Linux `~/.config/Claude/`. Key `mcpServers.flutree`.
- Quit and reopen Claude Desktop completely; `flutree` appears in the tools menu of a chat.
- Caveat: user scope only. `--scope project` reports `unsupported_scope`.

### Windsurf

```bash
flutree mcp install --client windsurf
```

- Writes `~/.codeium/windsurf/mcp_config.json` (user scope only), key `mcpServers.flutree`.
- Reload Windsurf and open the Cascade MCP panel to check that `flutree` is listed.
- Caveat: the path follows Windsurf's long-standing convention but could not be verified against its current official docs. Check it with `--dry-run`.

Detection never makes network calls: a client counts as present when its binary is on `PATH` or its config directory or file already exists.

### What the agent gets

**Tools (15).** All paths in results are absolute.

| Tool | Use it to |
|------|-----------|
| `discover_repos` | Find the Flutter repositories under a folder (start here). |
| `inspect_repo` | Describe one repository: branches, SDK pin, path dependencies. |
| `list_branches` | List local and `origin/*` branches of a repository, to propose base branches. |
| `plan_create` | Dry-run a workspace: validates everything and returns the plan, changes nothing. |
| `create_workspace` | Create the workspace after the plan; needs `confirm=true` or the user's confirmation. |
| `workspace_status` | Read one workspace: branches, dirty state, ahead/behind, overrides. |
| `list_worktrees` | List registered workspaces (`show_all`, `global_scope`). |
| `create_worktree` | Legacy one-shot create with no confirmation step; prefer `plan_create` + `create_workspace`. |
| `add_repo` | Attach more repositories to an existing workspace. |
| `complete_worktree` | Remove a workspace's worktrees (branches stay). Always asks the user first. |
| `pubget` | Run `pub get` across the packages of a workspace. |
| `clean_worktree` | Clean the managed worktree that contains the server's working directory. |
| `get_config` | Read `scope.root`, `workspace.generate` or `sdk.manager`. |
| `set_config` | Write one of those config keys. |
| `get_version` | Return the flutree binary version. |

**Resources.** `flutree://guide` (usage guide), `flutree://workspaces` (all workspaces), `flutree://workspace/{name}` (status of one workspace) and `flutree://config` (effective configuration).

**Prompts.** `create_workspace_flow`, `add_package_flow` and `finish_workspace_flow` walk the agent through the recommended steps.

**Asking the user and confirmation.** The agent never decides base branches, packages or destructive actions for you. When the client supports MCP elicitation, flutree asks you directly with a form (missing base branch, packages, a yes/no before `create_workspace`, and always before `complete_worktree`; `pubget` only with `force=true`). If you decline or cancel, nothing is executed and the agent must not retry on its own.

**Fallback without elicitation.** Nothing is executed. The tool returns `needs_user_input: true` (for example `status: "confirmation_required"`) with a `questions` list; the agent asks you in the chat and calls the tool again with your answers or `confirm=true`.

Full details (arguments, results, errors) are in [docs/mcp.md](docs/mcp.md).

### Uninstalling

```bash
flutree mcp uninstall                      # remove only the flutree entry from every detected client
flutree mcp uninstall --client cursor      # one client
flutree mcp uninstall --dry-run            # show what would be removed
```

Other keys and servers are preserved. Each client reports `removed`, `not_found` (no flutree entry), `not_installed` (client not detected) or `unsupported_scope`.

### Manual configuration (unsupported clients)

For clients `mcp install` does not handle, add this stdio entry manually:

```json
{
  "mcpServers": {
    "flutree": {
      "type": "stdio",
      "command": "/path/to/flutree",
      "args": ["mcp", "serve"]
    }
  }
}
```

Replace `/path/to/flutree` with the output of `which flutree`. The server speaks JSON-RPC over stdin/stdout; do not pass `--json` to `mcp serve` (it would corrupt the transport framing).

### Troubleshooting

- **The agent does not list flutree:** run `flutree mcp doctor`; if the client shows `[-] ... no flutree entry`, run `flutree mcp install --client <name>`, then restart the agent.
- **`[WARN] binary: ... temporary directory`:** reinstall flutree at a stable path and run `flutree mcp install --force`.
- **`[ERR]` on a command path:** the binary moved or was removed; fix the path with `flutree mcp install --force`.
- **`[ERR] handshake`:** run `flutree mcp serve` by hand; it should wait silently on stdin. Check that the entry does not pass `--json`.
- **Install refuses to touch a config with comments:** pass `--force` (comments are stripped).
- **`unsupported_scope`:** that client has no project config; use the default user scope.
- **Project config not picked up:** `--scope project` writes into the current directory, so run it from the repository root.

## Quickstart (CLI)

Interactive create works from any directory:

```bash
flutree create feature-login
```

If the scope (`--scope` > `scope.root` > current directory) has no Flutter repositories, a folder navigator opens. Keys: `Up`/`Down` (or `j`/`k`) move, `Enter` opens a folder (`..` goes up), `Tab` selects the current folder as scope, `Esc` goes up one folder, `ctrl+o` changes folder from the root step or adds packages from any folder on the packages step. Folders that are Flutter repos are marked `[flutter]`. Lists filter as you type, and `create` shows a dry plan and asks for confirmation before writing anything.

Non-interactive (scripts and CI):

```bash
flutree create feature-login --scope ~/code --root-repo app --package core --package ui --yes --non-interactive
```

Then:

```bash
flutree list                       # workspaces of the current repo (global outside a repo)
flutree pubget feature-login       # pub get across packages, root last
flutree add-repo feature-login --repo analytics --non-interactive
flutree complete feature-login --yes
```

Set a default discovery folder once with `flutree config set scope.root ~/code`.

Shell completion:

```bash
flutree completion zsh > "${fpath[1]}/_flutree"
```

## Commands Reference

Global flags on every command:

- `--help` / `-h` (also `flutree help <command>`)
- `--json`: machine-readable output and JSON error envelopes. Works before or after the command (`flutree --json list`).
- `--version` (root command only): print the version.

Flags may appear before or after positional arguments. Long flags need two dashes.

### create

Creates a managed workspace (worktrees, branches, overrides, registry entry).

```
flutree create <name> [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--branch` | string | `feature/<name>` | Target branch name |
| `--base-branch` | string | `main` | Base branch for worktree creation |
| `--scope` | string | `.` | Directory scope for Flutter repo discovery |
| `--root-repo` | string |  | Root repository: name, package name or path to a Flutter repo, may be outside `--scope` (required with `--non-interactive`) |
| `--package` | stringArray |  | Package repository: name, package name or path to a Flutter repo (repeatable) |
| `--package-base` | stringArray |  | Override package base branch as `<selector>=<branch>` (repeatable). The default `develop` must exist locally or on origin, otherwise create fails with exit `2` before changing anything |
| `--no-package` | bool | `false` | Root-only workspace; conflicts with `--package` and `--package-base` |
| `--workspace` | bool | `true` | Generate a VSCode workspace file (default from config `workspace.generate`) |
| `--no-workspace` | bool | `false` | Disable VSCode workspace generation |
| `--copy-root-file` | stringArray |  | Extra root file or pattern to copy (repeatable). `.env` and `.env.*` are copied by default |
| `--reuse-existing-branch` | bool | `false` | Allow non-interactive reuse of an existing branch |
| `--non-interactive` | bool | `false` | Disable prompts |
| `--yes` | bool | `false` | Auto-acknowledge the dry plan in non-interactive mode |

```bash
flutree create feature-login
flutree create feature-login --scope ~/code --root-repo app --package core --non-interactive --yes
```

If `create` fails, the branches it created are deleted again; pre-existing branches are never touched. If the scope you picked in the navigator differs from `scope.root`, the review step offers to save it as the default (`s` toggles, on by default).

### add-repo

Attaches repositories to an existing workspace and regenerates `pubspec_overrides.yaml`. In an interactive terminal without `--repo` it opens a multiselect wizard with a final review.

```
flutree add-repo <workspace> [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--repo` | stringArray |  | Repository to attach: name, package name or path to a Flutter repo, may be outside `--scope` (repeatable). Skips the wizard |
| `--scope` | string | `.` | Directory scope for Flutter repo discovery |
| `--sync-policy` | string | `auto` | Sync behavior before create: `auto`, `always` or `never` |
| `--package-base` | stringArray |  | Override package base branch as `<selector>=<branch>` (repeatable) |
| `--package-branch-source` | stringArray |  | Override package target branch as `<selector>=<branch>` (repeatable) |
| `--copy-root-file` | stringArray |  | Extra root file or pattern to copy (repeatable) |
| `--reuse-existing-branch` | bool | `false` | Allow non-interactive reuse of an existing branch |
| `--non-interactive` | bool | `false` | Disable the wizard and prompts; requires deterministic selectors |

```bash
flutree add-repo feature-login
flutree add-repo feature-login --repo core --sync-policy never --non-interactive
```

### config

Manages persisted configuration (stored in `~/Documents/worktrees/.flutree_config.json`).

```
flutree config get <key>
flutree config set <key> <value>
```

| Key | Description |
|-----|-------------|
| `scope.root` | Default discovery root for `create` and `add-repo` when `--scope` is omitted. Must be an existing directory |
| `workspace.generate` | `true` or `false` (default `true`): whether `create` writes a VSCode `.code-workspace` when neither `--workspace` nor `--no-workspace` is passed |
| `sdk.manager` | `auto`, `fvm`, `puro` or `none` (default `auto`): Flutter SDK manager run in every new worktree |

```bash
flutree config set scope.root ~/code
flutree config get scope.root
flutree config set sdk.manager fvm
```

#### SDK setup

After `create` and `add-repo`, flutree prepares the Flutter SDK in the root and every package worktree:

- **fvm**: reads `.fvmrc` (`"flutter"`) or the legacy `.fvm/fvm_config.json`, then runs `fvm use <version> --force --skip-pub-get`.
- **puro**: reads `.puro.json` (`"env"`), from the source repository too since puro gitignores it, then runs `puro use <env>`.
- `auto` (default) uses fvm when the repo pins an fvm version, otherwise puro. `fvm` or `puro` force one manager; `none` disables the step.
- Repos without a pin are skipped. If the manager fails, the worktree is kept, a `[warning]` is printed on stderr and `sdk_setup` in `--json` output reports `failed` with the reason.

### list

Lists managed workspaces. Inside a repository the scope is that repo; outside one it falls back to the global registry.

```
flutree list [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool | `false` | Include unmanaged Git worktrees |
| `--global` | bool | `false` | List across all registered repositories, from any directory |

```bash
flutree list
flutree list --global --all
```

### complete

Removes a workspace's worktrees and cleans its registry entry. Local branches are kept. If a recorded path is already missing, it cleans the stale entry and still succeeds.

```
flutree complete <name> [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--yes` | bool | `false` | Skip interactive confirmation |
| `--force` | bool | `false` | Force worktree removal (also allows dirty worktrees) |
| `--non-interactive` | bool | `false` | Disable prompts; requires `--yes` |

```bash
flutree complete feature-login
flutree complete feature-login --non-interactive --yes --force
```

### pubget

Runs `pub get` in all package repos of a workspace in parallel, then in the root last.

```
flutree pubget <name> [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool | `false` | Clean cache and remove `pubspec.lock` before `pub get` |

```bash
flutree pubget feature-login
flutree pubget feature-login --force
```

### clean

Cleans the current managed worktree (the one containing your current directory).

```
flutree clean [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool | `false` | Also remove `pubspec.lock` |

```bash
flutree clean
flutree clean --force
```

### update

Checks for or applies a Homebrew update. Homebrew is the only supported update channel; for Scoop use `scoop update flutree`, and for release archives download the new one.

```
flutree update [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--check` | bool | `false` | Check whether a brew update is available, without changing anything |
| `--apply` | bool | `false` | Apply the brew update now (the default behavior of `flutree update`) |

```bash
flutree update --check
flutree update
```

### version

Prints the CLI version only (stable, parseable output). `flutree --version` is equivalent.

```
flutree version
```

This command has no flags besides the global ones.

```bash
flutree version
flutree --json version
```

### completion

Generates a shell completion script for `bash`, `zsh`, `fish` or `powershell`.

```
flutree completion <bash|zsh|fish|powershell>
```

This command has no flags besides the global ones. Run `flutree completion <shell> --help` for per-shell setup details.

```bash
flutree completion bash > /etc/bash_completion.d/flutree
flutree completion zsh > "${fpath[1]}/_flutree"
flutree completion fish > ~/.config/fish/completions/flutree.fish
flutree completion powershell | Out-String | Invoke-Expression
```

### mcp

Runs, installs and diagnoses the MCP server for AI agents. See [Set up flutree for your AI agent (MCP)](#set-up-flutree-for-your-ai-agent-mcp) for the step-by-step guide.

```
flutree mcp <serve|install|uninstall|doctor>
```

Supported clients for `--client`: `claude-code`, `opencode`, `codex`, `cursor`, `vscode`, `gemini`, `claude-desktop`, `windsurf`.

#### mcp serve

Starts the MCP stdio server. AI clients launch it themselves; you rarely run it by hand. stdout is the JSON-RPC transport, so `--json` must not be used.

```
flutree mcp serve
```

This command has no flags besides the global ones.

```bash
flutree mcp serve
```

#### mcp install

Writes the flutree entry into every detected client (or the ones you choose). Existing keys and other servers are preserved; an existing flutree entry is kept unless `--force`.

```
flutree mcp install [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--client` | string |  | Comma-separated clients to target. Omit to target all detected clients |
| `--scope` | string | `user` | `user` or `project` (the current directory). Clients without a project config report `unsupported_scope` |
| `--dry-run` | bool | `false` | Show the target file and entry per client without writing (works with `--json`) |
| `--force` | bool | `false` | Overwrite existing entries; also strips comments from JSONC/TOML configs |

```bash
flutree mcp install --dry-run
flutree mcp install --client claude-code,cursor --scope project
```

#### mcp uninstall

Removes only the flutree entry from each detected client.

```
flutree mcp uninstall [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--client` | string |  | Comma-separated clients to target |
| `--scope` | string | `user` | `user` or `project` |
| `--dry-run` | bool | `false` | Show what would be removed without writing |

```bash
flutree mcp uninstall --dry-run
flutree mcp uninstall --client opencode
```

#### mcp doctor

Reports the resolved flutree binary, per client whether it was detected, its config path, whether the flutree entry exists and its command is an executable, and runs a live `mcp serve` initialize handshake (5s timeout). Exits `1` on `[ERR]`.

```
flutree mcp doctor [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--scope` | string | `user` | Config scope to inspect: `user` or `project` |

```bash
flutree mcp doctor
flutree --json mcp doctor --scope project
```

## Workspace layout

Default destination root is `~/Documents/worktrees`. Each workspace is one folder:

```text
~/Documents/worktrees/<worktree-name-slug>/
  root/<root-repo-folder>/          # the root app worktree (holds pubspec_overrides.yaml)
  packages/<package-repo-folder>/   # one worktree per selected package (holds its own pubspec_overrides.yaml when it needs one)
  <name>.code-workspace             # optional, folders only
```

`create` is a two-phase flow: it first renders a full dry plan (repos, branches, paths, commands and output files), then asks for one final confirmation before any `git worktree add` or file and registry write. If the target branch already exists, it asks whether to reuse it. In interactive mode it also asks whether to sync local branches from `origin` first.

`pubspec_overrides.yaml` rules:

- `flutree create` writes one `pubspec_overrides.yaml` in the root worktree when at least one package is selected; root-only workspaces (`--no-package`) get no file.
- It keeps every `dependency_overrides` entry already declared in the root `pubspec.yaml` (version, `path`, `git`, `hosted`), because a `pubspec_overrides.yaml` replaces those instead of merging with them. Relative paths that point outside the repository are rebased so they still resolve from the worktree; selected package worktrees win on conflict.
- The whole dependency graph is resolved from the worktrees, so transitive path dependencies between selected packages point at their worktrees too.
- The file is ignored through `.git/info/exclude` (never committed); the tracked `.gitignore` is not modified. `pubspec.yaml` is never modified.
- Each selected package worktree gets its own `pubspec_overrides.yaml` too, because `flutree pubget` resolves inside every package and pub only reads the overrides of the package it resolves. It points the selected packages that package depends on (directly or through other selected packages, whatever the dependency spec: `path`, `git`, hosted or version) at their worktrees, and carries the package's own `dependency_overrides` with the same rules as the root. Paths are relative to the package worktree.
- A package that needs nothing gets no file. A package that uses Dart pub workspaces (`resolution: workspace`) is skipped, and the `--json` result lists it under `package_overrides` with a note. `add-repo` regenerates the files of every package worktree; entries you edited by hand survive.

The VSCode workspace output is minimal and includes `folders` entries only (no `settings`, `tasks` or `launch`). Use `--no-workspace` or `flutree config set workspace.generate false` to skip it.

## Non-interactive behavior and JSON output

- `create` requires a final confirmation in interactive mode.
- `create --yes` only auto-approves together with `--non-interactive`.
- `create --non-interactive` without `--yes` fails fast by design, and it also requires `--root-repo`.
- `create --non-interactive` requires `--reuse-existing-branch` when the target branch already exists.
- `complete` requires confirmation unless `--yes` is passed; `complete --non-interactive` without `--yes` fails fast.
- `add-repo --non-interactive` never prompts and requires explicit `--repo` selectors.

With `--json`, commands print machine-readable results on stdout and errors as an envelope on stderr:

```json
{"error": {"category": "input", "code": 2, "message": "...", "hint": "..."}}
```

`code` equals the process exit code. Create and add-repo results include `sdk_setup` entries.

## Exit Codes

- `0`: success
- `1`: operational, precondition, process, git or update failure
- `2`: input error or user-cancelled flow (including unknown commands)

## Registry

Global registry file: `~/Documents/worktrees/.worktrees_registry.json`.

Each record stores the workspace name, branch, paths, status and, since v2, an optional `base_branch` per worktree (the base it was created from). `workspace_status` uses it to compute ahead/behind; records created before it was stored simply omit those numbers. Writes are atomic and schema-validated.

## Testing

The test suite is end-to-end first: it builds the real `flutree` binary and drives it like a user would.

```bash
go test ./...          # everything
./scripts/e2e.sh       # build the binary and run the e2e suite (integration/)
```

Enable the versioned pre-push hook once per clone; it runs `scripts/e2e.sh` before every `git push` and aborts the push when it fails:

```bash
git config core.hooksPath .githooks
```

Skip it only in an emergency with `git push --no-verify`. Manual flow:

```bash
go build -o ./flutree ./cmd/flutree
./flutree list
./flutree create demo --scope . --root-repo <repo> --yes --non-interactive
./flutree complete demo --yes --force
```

## Project structure

```text
cmd/flutree/
internal/
  app/
  domain/
  infra/
  runtime/
  ui/
integration/
docs/
  usage.md
  architecture.md
  mcp.md
```

More details:

- [docs/usage.md](docs/usage.md)
- [docs/architecture.md](docs/architecture.md)
- [docs/mcp.md](docs/mcp.md)
- [CHANGELOG.md](CHANGELOG.md) for release history and breaking changes

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request. For major changes, please open an issue first to discuss what you would like to change. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) for interactive UI
- Built on [Cobra](https://github.com/spf13/cobra)
- Inspired by the need to manage complex Flutter monorepo workflows
