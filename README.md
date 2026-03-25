# flutree

`flutree` is a Go CLI to manage Git worktrees for multi-package Flutter workflows.

Current commands:
- `create`: creates a managed worktree and stores metadata in a global registry.
- `list`: lists managed worktrees (scoped to current repo when available, otherwise global registry scope).
- `complete`: remove-only completion flow (removes worktree and keeps local branch).

## Requirements

- Go `>=1.22` (for local build from source)
- Git available in `PATH`

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
```

### Build from source

```bash
go build -o flutree ./cmd/flutree
./flutree --help
```

### Probar en local (paso a paso)

```bash
# 1) Compilar
go build -o ./flutree ./cmd/flutree

# 2) Smoke test
./flutree --help

# 3) Ejecutar tests
go test ./...
```

## Quickstart

Run from inside a Git repository:

```bash
flutree create feature-login --branch feature/login --root-repo repo --scope . --yes --non-interactive
flutree list
flutree complete feature-login --yes --force
```

If you omit `--branch`, branch defaults to `feature/<normalized-name>`.

Use `--yes` on `create` with `--non-interactive` to approve the dry plan in automation and CI scripts.

Default destination root is `~/Documents/worktrees`, generating:

`~/Documents/worktrees/<worktree-name-slug>/`

`create` now runs a strict two-phase flow before mutation:
- first, it renders a full dry plan preview (repos, branches, paths, commands, and output files);
- second, it asks for one final confirmation token gate before `git worktree add` and file/registry writes.

For automation/non-interactive runs, `create --non-interactive` requires explicit `--yes` and `--root-repo`.
For deterministic package targeting, pass `--package` and optional `--package-base` overrides.

Example with explicit package selectors and workspace output:

```bash
flutree create feature-login --scope . --root-repo root-app --package core-pkg --package-base core-pkg=develop --yes --non-interactive
```

Disable workspace generation when needed:

```bash
flutree create feature-login --scope . --root-repo root-app --package core-pkg --yes --non-interactive --no-workspace
```

Package override generation rules:
- `flutree create` writes one `pubspec_override.yaml` in the selected root worktree.
- `pubspec.yaml` is not modified.

VSCode workspace output is MVP-only and includes `folders` entries only.
`settings`, `tasks`, and `launch` are intentionally not generated.
Use `--no-workspace` to skip `.code-workspace` output entirely.

## How to test

```bash
go test ./...
```

Ejemplo de prueba manual de flujo:

```bash
go build -o ./flutree ./cmd/flutree
./flutree list
./flutree create demo --scope . --root-repo <repo> --yes --non-interactive
./flutree complete demo --yes --force
```

## Non-interactive behavior

- `create` requires final confirmation in interactive mode.
- `create --yes` is only auto-approval in `--non-interactive` mode.
- `create --non-interactive` without `--yes` fails fast by design.
- `create --non-interactive` also requires explicit `--root-repo` selector.
- `complete` requires confirmation unless `--yes` is passed.
- `complete --non-interactive` without `--yes` fails fast by design.

## Registry

Global registry file:

`~/Documents/worktrees/.worktrees_registry.json`

Writes are atomic and schema-validated.

## Notes

Homebrew artifacts are produced from the Go CLI binary at `./cmd/flutree`.

## Project structure

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
