# Usage Guide

## Install and run

Homebrew install (macOS arm64):

```bash
brew tap EndersonPro/flutree
brew install EndersonPro/flutree/flutree
flutree --help
```

Homebrew upgrade:

```bash
brew update
brew upgrade flutree
```

Build from source:

```bash
go build -o flutree ./cmd/flutree
./flutree --help
```

Run tests:

```bash
go test ./...
```

## Architecture support matrix

- Supported Brew binary target: `darwin-arm64`.
- Unsupported architecture (for example Intel macOS `x86_64`): build from source with Go.

## Command Summary

`flutree create NAME [OPTIONS]`
- Creates a new worktree and branch.
- Persists metadata in the global registry.
- Runs preflight checkpoints before any mutation is applied.
- `--no-package` enables explicit root-only creation and skips package metadata flow.
- `--no-package` conflicts with `--package` and `--package-base`.
- If a target branch already exists, asks for explicit reuse confirmation (or requires `--reuse-existing-branch` in non-interactive mode).
- Syncs the configured base branch before any new branch worktree creation.

`flutree add-repo WORKSPACE [OPTIONS]`
- Attaches additional repositories to an existing managed workspace.
- In interactive mode, missing branch/base values can be entered per attached repo.
- In non-interactive mode, the command never prompts and uses deterministic defaults (`source=<workspace branch>`, `base=main`) unless explicit overrides are provided.
- `--sync-policy auto|always|never` controls pre-create remote sync behavior for attached repos.

`flutree config set scope.root PATH` / `flutree config get scope.root`
- Persists and reads the default discovery scope root.
- Scope resolution precedence for `create` and `add-repo`: explicit `--scope` > persisted `scope.root` > `.`.
- `scope.root` must be an existing reachable directory.

`flutree list [--all] [--global]`
- Lists managed entries for the current repository when running inside a repo.
- If running outside a repo, it falls back to the global registry view.
- `--global` forces global registry scope from any current directory.
- `--all` also includes unmanaged Git worktrees discovered from `git worktree list --porcelain` for discovered managed repos.

`flutree complete NAME [OPTIONS]`
- Remove-only MVP completion flow.
- Removes the worktree and keeps the local branch.
- If a tracked worktree path is already missing, it cleans the stale registry entry and reports success.

`flutree pubget NAME [OPTIONS]`
- Runs `pub get` in all package repos for a managed workspace in parallel.
- Runs root repo `pub get` only after package phase completes.

`flutree --version` / `flutree version`
- Prints the CLI version only (stable, parseable output).

`flutree update [--check|--apply]`
- Brew-only automatic update flow for Homebrew installations.
- `--check` reports availability without mutating.
- Default behavior (`flutree update`) applies update flow; `--apply` is an explicit equivalent.

## create

Options:
- `--branch, -b TEXT`: target branch name for the root worktree. If omitted, defaults to `feature/<normalized-name>`.
- `--base-branch TEXT`: source branch for root worktree creation (default: `main`).
- `--scope PATH`: execution directory scope used to discover Flutter repositories (default: current directory).
  - if omitted, `create` uses persisted `scope.root` when configured.
- `--root-repo TEXT`: explicit root repository selector for non-interactive usage.
- `--no-package`: explicit root-only mode; skip package selection and package metadata prompts.
- `--package, -p TEXT`: explicit package repository selector (repeatable).
- `--package-base TEXT`: per-package base branch override in `<selector>=<branch>` format (repeatable, default `develop`).
- `--workspace/--no-workspace`: enable or disable VSCode `.code-workspace` generation (enabled by default).
- `--yes`: acknowledge dry plan automatically only when `--non-interactive` is enabled.
- `--non-interactive`: disable prompts; requires explicit `--yes` and `--root-repo`.
- `--reuse-existing-branch`: allow reusing existing local branches without interactive prompt (required in non-interactive mode when target branch already exists).

Two-phase flow:
- phase 1: dry-plan preview prints selected root/packages, branches, commands, and file outputs.
- phase 2: single final confirmation token gate before any `git worktree add` and file/registry mutation.

Examples:

```bash
flutree create auth-fix --branch feature/auth-fix --scope .
flutree create auth-fix --scope ~/code --root-repo app-root --no-package --yes --non-interactive
flutree create auth-fix --scope ~/code --root-repo app-root --package package-core --package package-ui
flutree create auth-fix --scope ~/code --root-repo app-root --package package-core --package-base package-core=develop --yes --non-interactive
```

Generated destination path format:

`~/Documents/worktrees/<worktree-name-slug>/`

Generated worktrees are grouped into:
- root: `~/Documents/worktrees/<worktree-name-slug>/root/<root-repo-folder>/`
- packages: `~/Documents/worktrees/<worktree-name-slug>/packages/<package-repo-folder>/`

Package override output:
- `flutree create` writes exactly one `pubspec_overrides.yaml` in the selected root worktree.
- dependency paths target selected package worktree paths.
- `pubspec.yaml` is never modified by this workflow.

VSCode workspace output (MVP):
- When `--workspace` is enabled, the generated workspace contains only:

```json
{
  "folders": [
    { "path": "root/root-app" },
    { "path": "packages/core-pkg" }
  ]
}
```

- `settings`, `tasks`, and `launch` are intentionally omitted in this phase.

## list

Options:
- `--all`: include unmanaged worktrees in the output table.
- `--global`: force global registry scope from any current directory.

Scope behavior:
- `flutree list`: repo-scoped when running inside a repository, global fallback outside repositories.
- `flutree list --global`: always global scope, independent of CWD.
- `flutree list --global --all`: global scope plus unmanaged rows across all selected repositories.

Output fields:
- `Name`: managed name, or `-` for unmanaged rows.
- `Branch`: tracked branch for managed, detected branch for unmanaged.
- `Path`: filesystem path.
- `Status`: `active`, `missing`, `unmanaged`, or `completed`.

Notes:
- In the current remove-only MVP flow, completed records are removed from registry, so `completed` is uncommon unless injected externally.

## complete

Options:
- `--yes`: skip interactive confirmation.
- `--force`: force worktree removal (also allows dirty worktree completion).
- `--non-interactive`: disable prompts; requires explicit confirmation via `--yes`.

Examples:

```bash
flutree complete auth-fix --yes
flutree complete auth-fix --non-interactive --yes
```

## pubget

Options:
- `--force`: run cleanup before `pub get` (`flutter clean` for Flutter repos, remove `.dart_tool` for Dart repos) and remove `pubspec.lock`.

Examples:

```bash
flutree pubget auth-fix
flutree pubget auth-fix --force
```

## Exit and Error Behavior

Exit code contract:
- `0` success
- `1` operational/precondition/process/git/update failure
- `2` input/cancel errors (invalid args, declined confirmation, missing explicit non-interactive confirmation)

By default, unexpected errors are hidden behind a concise message.
Use `--debug` to surface internal exception details.

## Go version compatibility

`flutree` source builds require Go `>=1.22`.

## Failure Remediation

Not in a repository (create flow):
- Error: `[precondition] Current directory is not inside a Git repository.`
- Fix: run create from a valid repo root or child folder.
- Note: `flutree list` now works outside repositories using global registry scope.
- Note: `flutree complete` also works outside repositories using record `repo_root`.

Branch already in use:
- Error category: `precondition`
- Fix: choose another branch or complete/remove the conflicting worktree.

Dirty worktree on complete:
- Error category: `precondition`
- Fix: commit or stash changes, or use `--force` deliberately.

Registry/persistence issues:
- Error category: `persistence`
- Fix: inspect `~/Documents/worktrees/.worktrees_registry.json` and correct invalid shape/duplicates.
## add-repo

Options:
- `--scope PATH`: execution directory scope used to discover Flutter repositories (default: current directory).
  - if omitted, `add-repo` uses persisted `scope.root` when configured.
- `--repo TEXT`: repository selector to attach (repeatable). When omitted in interactive TTY mode, `add-repo` opens the multiselect wizard.
- `--package-branch-source TEXT`: per-repository target branch override in `<selector>=<branch>` format (repeatable).
- `--package-base TEXT`: per-repository base branch override in `<selector>=<branch>` format (repeatable).
- `--sync-policy TEXT`: sync behavior before creation: `auto` (interactive confirm, non-interactive false), `always`, `never`.
- `--reuse-existing-branch`: allow non-interactive branch reuse when target branch already exists.
- `--copy-root-file TEXT`: extra root-level file/pattern copied into each attached worktree (repeatable).
- `--non-interactive`: disable interactive wizard/prompts and enforce deterministic execution.

Interactive flow (TTY + no `--repo` + no `--non-interactive`):
- Step 1: repository multiselect (`↑/↓` or `j/k`, `space` to toggle).
- Step 2: per-selected-repo source/base branch inputs.
- Step 3: final review + explicit apply/cancel gate before any mutation.

Examples:

```bash
flutree add-repo feature-login --scope ~/code --repo core-pkg
flutree add-repo feature-login --scope ~/code --repo core-pkg --package-branch-source core-pkg=feature/core --package-base core-pkg=main --sync-policy always --non-interactive --reuse-existing-branch
flutree add-repo feature-login --scope ~/code --repo core-pkg --sync-policy never --non-interactive
```

## config

Supported key:
- `scope.root`: default discovery root for `create` and `add-repo`.

Examples:

```bash
flutree config set scope.root ~/code
flutree config get scope.root
```

Persistence file:
- `~/Documents/worktrees/.flutree_config.json`
- JSON schema v1:

```json
{ "version": 1, "scope": { "root": "/absolute/path" } }
```
