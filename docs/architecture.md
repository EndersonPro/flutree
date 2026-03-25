# Architecture Guide

`flutree` follows a layered Go architecture to keep workflow rules testable and infrastructure isolated.

## Layers

## 1) Command Layer (`cmd/flutree`)
- CLI handlers map arguments/options to typed input models.
- Commands call services and then render output.
- They do not execute Git subprocesses or parse files directly.

## 2) Domain Layer (`internal/domain`)
- Typed contracts for inputs, registry documents, read models.
- Stable user-facing error categories and exit code semantics.

## 3) Adapter Layer (`internal/infra`)
- `git/`: subprocess interaction and porcelain parsing.
- `registry/`: global registry repository + integrity checks.
- `prompt/`: confirmation boundary with non-interactive fail-fast behavior.

## 4) UI Layer (`internal/ui`)
- Console rendering primitives for success and list views.

## Runtime Boundary

`internal/runtime` provides consistent exception mapping.
- Domain/adapters raise typed app errors.
- The boundary maps errors into consistent stderr output and exit codes.

## Data/Control Flow

### create
CLI -> `CreateInput` -> `CreateService` -> `GitGateway` + `RegistryRepository` -> `render_create_success`

`CreateService` flow details:
- Discover Flutter repositories from the execution scope, independent of current working directory.
- Resolve exactly one root repository plus zero-or-more package repositories.
- Resolve per-package base branch values (default `develop`).
- Build a complete dry plan first (paths, branches, commands, files) with zero side effects.
- Execute only after explicit final confirmation token gate.
- Create worktrees in deterministic order: root -> packages.
- Write one root-level `pubspec_override.yaml` referencing selected package worktree paths.
- Write one VSCode `.code-workspace` file in `~/Documents/worktrees/<folder-name>/`.
- Apply rollback by removing created worktrees and reverting registry entries on failures.

### complete
CLI -> `CompleteInput` -> `CompleteService` -> `GitGateway` + `PromptAdapter` + `RegistryRepository` -> `render_complete_success`

### list
CLI -> `ListService` -> `RegistryRepository` + `GitGateway` reconciliation -> `render_list`

## How to Add a New Command

1. Add a new service in `internal/app/`.
2. Extend typed models/errors if needed in `internal/domain/`.
3. Add/extend adapters only for IO boundaries (Git, filesystem, prompts).
4. Add a command handler in `cmd/flutree/main.go`.
5. Register command in CLI dispatch.
6. Add tests for service/adapter behavior and integration tests for command contract.

Guardrail: keep infrastructure out of command handlers and workflow logic out of adapters.
