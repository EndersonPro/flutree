<p align="center"><img src="assets/flutree-icon.png" alt="flutree logo" width="300" /></p>

`flutree` is a Go CLI to manage Git worktrees for multi-package Flutter workflows.

[![tests](https://github.com/EndersonPro/flutree/actions/workflows/tests.yml/badge.svg)](https://github.com/EndersonPro/flutree/actions/workflows/tests.yml)
[![Release](https://img.shields.io/github/v/release/EndersonPro/flutree?color=green&style=flat-square)](https://github.com/EndersonPro/flutree/releases/latest)
[![License](https://img.shields.io/github/license/EndersonPro/flutree?color=blue&style=flat-square)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/EndersonPro/flutree?style=flat-square)](go.mod)
[![Last Commit](https://img.shields.io/github/last-commit/EndersonPro/flutree?style=flat-square)](https://github.com/EndersonPro/flutree/commits/main)


## Table of Contents

- [🚀 Features](#-features)
- [📦 Installation](#-installation)
- [🔧 Requirements](#-requirements)
- [🏗️ Commands Reference](#%EF%B8%8F-commands-reference)
  - [create](#create)
  - [add-repo](#add-repo)
  - [list](#list)
  - [complete](#complete)
  - [pubget](#pubget)
- [🛠️ Quickstart](#%EF%B8%8F-quickstart)
- [🆕 Recent CLI adjustments](#-recent-cli-adjustments)
- [⚙️ Advanced Usage](#%EF%B8%8F-advanced-usage)
- [🔄 Version and update](#-version-and-update)
- [🧪 Testing](#-testing)
- [🤖 Non-interactive behavior](#-non-interactive-behavior)
- [🗂️ Registry](#%EF%B8%8F-registry)
- [🏗️ Project structure](#%EF%B8%8F-project-structure)
- [🤝 Contributing](#-contributing)
- [📄 License](#-license)
- [🙏 Acknowledgments](#-acknowledgments)

## 🚀 Features

- **Multi-package Management**: Handle complex Flutter monorepos with multiple packages efficiently
- **Git Worktree Integration**: Seamless integration with Git's worktree functionality
- **Parallel Operations**: Execute operations across multiple worktrees in parallel
- **VSCode Workspace Generation**: Automatically generate workspace files for IDE integration
- **Package Override Support**: Manage `pubspec_overrides.yaml` files for development workflows

## 📦 Installation

### Homebrew (macOS arm64)

```bash
brew tap EndersonPro/flutree
brew install EndersonPro/flutree/flutree
```

Upgrade:

```bash
brew update
brew upgrade flutree
# or via flutree helper command
flutree update
```

### Build from source

```bash
go build -o flutree ./cmd/flutree
./flutree --help
```

## 🔧 Requirements

- Go `>=1.22` (for local build from source)
- Git available in `PATH`

## 🛠️ Quickstart

Run from inside a Git repository:

```bash
flutree create feature-login --branch feature/login --root-repo repo --scope . --yes --non-interactive
flutree add-repo feature-login --repo core-pkg --scope . --non-interactive
flutree list
flutree --version
flutree update --check
flutree pubget feature-login
flutree pubget feature-login --force
flutree complete feature-login --yes --force
```

If you omit `--branch`, branch defaults to `feature/<normalized-name>`.

Package worktrees reuse the exact same branch token as root (`--branch` value, or the default when omitted).

Use `--yes` on `create` with `--non-interactive` to approve the dry plan in automation and CI scripts.

Default destination root is `~/Documents/worktrees`, generating:

`~/Documents/worktrees/<worktree-name-slug>/`

## 🆕 Recent CLI adjustments

- Every subcommand supports command-scoped help via `--help` and `-h`.
- `create --no-package` is now an explicit root-only mode:
  - package selection is skipped in interactive mode,
  - `--no-package` conflicts with `--package` and `--package-base` (fail-fast input error).
- `add-repo` is the command for attaching repositories after a workspace already exists.
- Before syncing branches from `origin` during `create`, the CLI now asks for confirmation:
  - **Yes** → sync from `origin` and continue with worktree creation.
  - **No** → skip remote sync entirely and continue from local refs.
- `list --global` always uses global registry scope, regardless of current directory.
- `list --global --all` includes unmanaged worktrees across all globally selected repositories.

## ⚙️ Advanced Usage

### Two-phase Flow

`create` now runs a strict two-phase flow before mutation:
- first, it renders a full dry plan preview (repos, branches, paths, commands, and output files);
- second, it asks for one final confirmation token gate before `git worktree add` and file/registry writes.

For automation/non-interactive runs, `create --non-interactive` requires explicit `--yes` and `--root-repo`.
If the target branch already exists, non-interactive runs also require `--reuse-existing-branch`.
Use `--no-package` when you need a root-only workspace and no package metadata flow.
`--no-package` cannot be combined with `--package` or `--package-base`.
In interactive mode, after selecting **Apply changes**, `create` asks whether local branches should be synced from `origin` before worktree creation.
If the answer is **Yes**, `create` syncs before worktree creation and fails fast if sync cannot be completed.
If the answer is **No**, `create` skips remote sync and continues from local refs.
For deterministic package targeting during `create`, use `--package` and optional `--package-base`.
If you forget to include a repository at create time, attach it later with `add-repo`.

Example with root env propagation:

```bash
flutree create feature-login --scope . --root-repo root-app --copy-root-file ".env.local" --yes --non-interactive
```

### Workspace Control

Disable workspace generation when needed:

```bash
flutree create feature-login --scope . --root-repo root-app --yes --non-interactive --no-workspace
```

Package override generation rules:
- `flutree create` writes one `pubspec_overrides.yaml` in the selected root worktree.
- `pubspec.yaml` is not modified.

VSCode workspace output is MVP-only and includes `folders` entries only.
`settings`, `tasks`, and `launch` are intentionally not generated.
Use `--no-workspace` to skip `.code-workspace` output entirely.

## 🏗️ Commands Reference

All subcommands support:

- `--help`
- `-h`

### create

Creates a managed worktree and stores metadata in a global registry.

Usage:
```
flutree create <name> [options]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--branch` | string | `feature/<name>` | Target branch name |
| `--base-branch` | string | `main` | Base branch for worktree creation |
| `--scope` | string | `.` | Directory scope used to discover Flutter repositories |
| `--root-repo` | string |  | Root repository selector (required in non-interactive mode) |
| `--workspace` | boolean | `true` | Generate VSCode workspace file |
| `--no-workspace` | boolean | `false` | Disable VSCode workspace generation |
| `--yes` | boolean | `false` | Acknowledge dry plan automatically in non-interactive mode |
| `--non-interactive` | boolean | `false` | Disable prompts |
| `--reuse-existing-branch` | boolean | `false` | Reuse existing local branch in non-interactive mode |
| `--no-package` | boolean | `false` | Root-only mode (skip package selection and package metadata) |
| `--package` | string |  | Package repository selector (repeatable) |
| `--package-base` | string |  | Override package base branch as `<selector>=<branch>` (repeatable) |
| `--copy-root-file` | string |  | Extra root-level file/pattern to copy into each worktree (repeatable). By default `.env` and `.env.*` are copied when present |

### add-repo

Attaches additional repositories to an existing managed workspace and regenerates `pubspec_overrides.yaml`.

Usage:
```
flutree add-repo <workspace> [options]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--scope` | string | `.` | Directory scope used to discover Flutter repositories |
| `--repo` | string |  | Repository selector to attach (repeatable). Required in non-interactive mode |
| `--package-base` | string |  | Override package base branch as `<selector>=<branch>` (repeatable) |
| `--copy-root-file` | string |  | Extra root-level file/pattern to copy into attached worktrees (repeatable). Default includes `.env` and `.env.*` |
| `--non-interactive` | boolean | `false` | Disable prompts |

### list

Lists managed worktrees. By default, scope is current repo when available (fallback to global outside a repo).
Use `--global` to force global registry scope from any location.

Usage:
```
flutree list [options]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | boolean | `false` | Include unmanaged Git worktrees |
| `--global` | boolean | `false` | Force global registry scope regardless of current directory |

### complete

Remove-only completion flow (removes worktree and keeps local branch).
If a recorded path is already missing, completion performs stale registry cleanup and still succeeds.

Usage:
```
flutree complete <name> [options]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--yes` | boolean | `false` | Skip interactive confirmation |
| `--force` | boolean | `false` | Force worktree removal |
| `--non-interactive` | boolean | `false` | Disable prompts |

### pubget

Runs `pub get` for all managed package repos in parallel, then runs root last. Includes interactive loading feedback on TTY.

Usage:
```
flutree pubget <name> [--force]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | boolean | `false` | Clean cache and remove `pubspec.lock` before `pub get` |

## 🔄 Version and update

Stable version output:

```bash
flutree --version
flutree version
```

Brew-only update flow:

```bash
flutree update --check
flutree update
flutree update --apply
```

`flutree update` is equivalent to `flutree update --apply`.

Current update automation scope is Homebrew only (non-brew channels are out of scope in this release).

Exit code contract for version/update paths:
- `0`: success
- `1`: brew/precondition/process failure on `update`
- `2`: input/cancel validation failures

## 🧪 Testing

```bash
go test ./...
```

Example of manual test flow:

```bash
go build -o ./flutree ./cmd/flutree
./flutree list
./flutree create demo --scope . --root-repo <repo> --yes --non-interactive
./flutree complete demo --yes --force
```

## 🤖 Non-interactive behavior

- `create` requires final confirmation in interactive mode.
- `create --yes` is only auto-approval in `--non-interactive` mode.
- `create --non-interactive` without `--yes` fails fast by design.
- `create --non-interactive` also requires explicit `--root-repo` selector.
- `create --non-interactive` requires `--reuse-existing-branch` when the target branch already exists.
- `complete` requires confirmation unless `--yes` is passed.
- `complete --non-interactive` without `--yes` fails fast by design.

## Exit Codes

- `0`: success
- `1`: operational/precondition/process/git/update failure
- `2`: input or user-cancelled flow

## 🗂️ Registry

Global registry file:

`~/Documents/worktrees/.worktrees_registry.json`

Writes are atomic and schema-validated.

## 🏗️ Project structure

```text
cmd/flutree/
internal/
  app/
  domain/
  infra/
  runtime/
  ui/
docs/
  usage.md
  architecture.md
```

More details:
- `docs/usage.md`
- `docs/architecture.md`

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request. For major changes, please open an issue first to discuss what you would like to change.

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) for interactive UI
- Inspired by the need to manage complex Flutter monorepo workflows
