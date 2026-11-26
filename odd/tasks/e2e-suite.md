Objective: make end-to-end CLI tests the primary test layer and run them locally before every push; then fix the user's reported bugs.
Branch: `feat/e2e-suite` (stacked on `feat/cobra-migration`) · Delivery: `ask-on-risk` · Runner: `go test ./integration/...` via `scripts/e2e.sh`.

## Specs

- S1 — "necesito implementar primordialmente pruebas e2e a la cli, no usar más test unitarios a menos que sean necesarios". New coverage is written as e2e tests that build and run the real `flutree` binary; unit tests only when e2e cannot reach the behavior. Documented as a contributor convention.
- S2 — Cover the commands that lack e2e coverage today: `pubget`, `clean`, `complete` error paths, MCP tool calls (assumption: derived from the coverage map, not listed by the user).
- S3 — "esos serán los que se ejecuten antes de cada PR que lance en local". User chose "Hook pre-push": a versioned `.githooks/pre-push` runs the e2e suite on every `git push`, enabled once with `git config core.hooksPath .githooks`.
- S4 — "y corregir unos bugs" — bugs not yet specified; pending user input after S1–S3.

## Tasks

| ID | Specs | Route | Status | Commit |
| --- | --- | --- | --- | --- |
| T1 | S1, S2 | inline | done | 6d110e4, 7e32ef0 |
| T2 | S3 | inline | done | (hook commit) |
| T3 | S4 | inline | blocked (needs bug list) | — |

## Log

- L1 (2026-11-26, user): "Bien, ahora necesito implementar primordialmente pruebas e2e a la cli, no usar más test unitarios a menos que sean necesarios, y corregir unos bugs. Pero primero los test y esos serán los que se ejecuten antes de cada PR que lance en local"
- L2 (2026-11-26, user decision): "Hook pre-push (Recomendado)".
- L3: `integration/` already holds 72 binary-level tests. Gaps: no `pubget`/`clean` e2e, `complete` only happy path, MCP only initialize/tools/list. `runCLI` merges stdout and stderr, and `buildCLI` rebuilds the binary per test. Test-first exception: new tests pin existing behavior, so RED is not expected; a failing test is a bug candidate for S4.
- L4 T1: helpers (single build via TestMain, `runE2E` with split streams, `fakeToolchain`, `createWorkspace`); 21 new e2e tests for pubget, clean, complete, lifecycle, `--json` stdout purity, MCP tools/call. e2e suite ~30 s → ~5 s. Full `go test ./...` green.
- L5 Bugs found by the new suite:
  - B1 (fixed, 6d110e4): `pubget --json` printed "Running pub get across workspace..." before the JSON.
  - B2 (fixed, 6d110e4): `create --json` printed the dry-plan table before the JSON.
  - B3 (open): `complete` without `--force` refuses a freshly created workspace with packages, because flutree's own generated `.gitignore` / `pubspec_overrides.yaml` count as uncommitted changes ("[precondition] Worktree 'w1' has uncommitted changes.").
  - B4 (open): `clean` outside a Git repository surfaces the raw git failure ("[git] Git command failed: git rev-parse --show-toplevel" plus a localized git hint).
  - Not a bug: `list` groups package worktrees under their workspace row (`package_count`).
- L6 T2: `scripts/e2e.sh` + `.githooks/pre-push`; enabled locally with `git config core.hooksPath .githooks`. Verified pass (exit 0) and abort on failure (exit 1). CONTRIBUTING documents e2e-first convention and the hook.
