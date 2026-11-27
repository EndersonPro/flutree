Objective: make end-to-end CLI tests the primary test layer and run them locally before every push; then fix the user's reported bugs.
Branch: `feat/e2e-suite` (stacked on `feat/cobra-migration`) · Delivery: `ask-on-risk` · Runner: `go test ./integration/...` via `scripts/e2e.sh`.

## Specs

- S1 — "necesito implementar primordialmente pruebas e2e a la cli, no usar más test unitarios a menos que sean necesarios". New coverage is written as e2e tests that build and run the real `flutree` binary; unit tests only when e2e cannot reach the behavior. Documented as a contributor convention.
- S2 — Cover the commands that lack e2e coverage today: `pubget`, `clean`, `complete` error paths, MCP tool calls (assumption: derived from the coverage map, not listed by the user).
- S3 — "esos serán los que se ejecuten antes de cada PR que lance en local". User chose "Hook pre-push": a versioned `.githooks/pre-push` runs the e2e suite on every `git push`, enabled once with `git config core.hooksPath .githooks`.
- S4 — "y corregir unos bugs" — first reported bug is S5; B3/B4 remain candidates until the user confirms them.
- S5 — "Cuando creo un pubspec_override este no está teniendo en cuenta los overrides que ya existen en el pubspec principal, los ignora, debe tenerlos en cuenta al momento de crear el grafo de dependencias entre multirepos". The generated `pubspec_overrides.yaml` (create and add-repo) must carry every `dependency_overrides` entry of the root `pubspec.yaml` — version, `path`, `git`, `hosted` — with relative paths rebased so they still point at the same directory from the worktree; flutree's package worktree entries win on conflict.
- S6 — "además de que necesito que agreguemos una prueba real o realizar pruebas de que el multirepo funcione". An e2e test resolves a generated multi-repo workspace with the real Dart SDK (`dart pub get` + `dart run`) and proves the root uses the package worktrees and the carried overrides; skipped only when `dart` is not on PATH.

## Tasks

| ID | Specs | Route | Status | Commit |
| --- | --- | --- | --- | --- |
| T1 | S1, S2 | inline | done | 6d110e4, 7e32ef0 |
| T2 | S3 | inline | done | 9fa5443 |
| T3 | S5, S6 | inline | done | cbad141 + correction |

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
- L7 Review 3cd035a..9fa5443 (high: executable hook, shell script): granted, approved, acknowledged. Follow-ups (non-blocking): `create --json` in interactive mode hides the dry plan before the APPLY prompt; JSON-purity subtests depend on order (complete runs last) and shadow `tc`; a failed shared build is reported by every test; B4 test pins known-bad output.
- L8 (2026-11-26, user): "Cuando creo un pubspec_override este no está teniendo en cuenta los overrides que ya existen en el pubspec principal, los ignora, debe tenerlos en cuenta al momento de crear el grafo de dependencias entre multirepos además de que necesito que agreguemos una prueba real o realizar pruebas de que el multirepo funcione"
- L9 Evidence: Dart 3.14 test — a `pubspec_overrides.yaml` replaces (does not merge) `pubspec.yaml` `dependency_overrides`. flutree today migrates only `path:` overrides and copies relative paths verbatim: reproduced `intl: 0.19.0` and a `git:` override dropped, `shared: path: ../shared` left pointing at `<worktrees>/w1/root/shared`. Fix uses gopkg.in/yaml.v3 (already in go.sum) instead of line parsing.
- L10 T3: RED — TestE2ECreateCarriesRootPubspecDependencyOverrides / TestE2EAddRepoCarries... (intl and git overrides dropped, shared path wrong) and TestE2ERealDartResolvesMultiRepoWorkspace (dart: "could not find package shared at \"../shared-fork\""). GREEN — override logic moved to internal/app/pubspec_overrides.go on yaml.v3: carries every override shape, rebases relative paths from the source repo to the worktree, precedence package worktrees > existing pubspec_overrides.yaml > pubspec.yaml. Real Dart test runs (not skipped) and passes. Removed 4 unit tests of the deleted line parser (behavior now covered end-to-end). `go test ./...` and scripts/e2e.sh green. Risk: item 5 (gopkg.in/yaml.v3 becomes a direct dependency).
- L11 Verification of cbad141 (independent verifier): S6 pass; S5 partial. Blocker: in-repo path overrides (`path: packages/local`) were rebased to the source checkout, so the worktree copy was ignored (real dart printed `source-checkout`). Correction: keep paths that stay inside the source repo; surface the YAML parse cause in the error hint. RED → GREEN on TestE2EInRepoPathOverrideStaysInsideWorktree (real dart prints `worktree-copy`) and TestE2EMalformedRootPubspecReportsParseError; TestE2EAddRepoTwiceKeepsRebasedOverrides pins idempotence. Known, not fixed: a failed create leaves an empty container dir and branch behind (pre-existing rollback gap); plain `dependencies: path:` siblings are not rewritten (pre-existing).
- L12 Review 9fa5443..39e5678 (medium, item 5 escalation): granted, approved, acknowledged.
