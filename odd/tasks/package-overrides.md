Objective: every selected package worktree resolves its dependencies on other selected packages from their worktrees, not from source paths, git or hosted versions.
Branch: `feat/package-overrides` (stacked on `docs/readme-refresh`) · Delivery: `ask-on-risk` · Runner: `./scripts/e2e.sh` + `go test ./...`.

## Specs

- S1 — User: "Creo que también me hace falta hacer override de los sub paquetes cuando hay dependencias entre sí, podríamos investigar eso ? A ver cómo lo manejamos dentro de flutree ?" → after the investigation (L2), user approved the proposal: "Si".
- S2 — `create` and `add-repo` write a `pubspec_overrides.yaml` in each selected package worktree with: (a) the package's transitive dependencies that are other selected packages (direct + through other selected packages), pointing at their worktrees; (b) the package pubspec's own existing `dependency_overrides`, carried with the same rules as the root (all shapes, relative paths rebased from the package's source repo to its worktree, in-repo paths kept); (c) path dependencies of that package rebased so they resolve from its worktree. Never an entry for the package itself; no entries for packages outside its graph; no file when nothing is needed. Precedence mirrors the root: selected worktrees > existing file > the package's own overrides > derived path deps; user-edited entries survive (same rule as `dropStaleGenerated`).
- S3 — Each package file is ignored through that package repo's `.git/info/exclude` (anchored `/pubspec_overrides.yaml`), so `complete` without `--force` still works.
- S4 — `add-repo` regenerates the files of all package worktrees (a new package can be a dependency of existing ones) and of the root.
- S5 — Packages that use Dart pub workspaces (`resolution: workspace` in their pubspec) are skipped with a note in the result; out of scope for now.
- S6 — MCP: `plan_create` lists the per-package override files and contents; `workspace_status` reports for each package worktree whether its override file exists. Docs (README "Workspace layout", docs/mcp.md, docs/usage.md) updated.

## Tasks

| ID | Specs | Route | Status | Commit |
| --- | --- | --- | --- | --- |
| T1 | S2–S6 | delegated writer (context backstop) | done | 5a0a6a6 |

## Log

- L1 (2026-11-27, user): "Creo que también me hace falta hacer override de los sub paquetes cuando hay dependencias entre sí, podríamos investigar eso ? A ver cómo lo manejamos dentro de flutree ?"
- L2 Investigation (parent, real dart in scratch): overrides were written only in the root (create_service.go:246, add_repo_service.go:283) while `flutree pubget` runs pub get inside each package. Case A: pkg_b → pkg_a by path with a different source layout (`../../libs/pkg-a`) → `flutree pubget` fails: "could not find package pkg_a at \"../../libs/pkg-a\"". Case B: sibling layout works by coincidence. Case C: git (or hosted) dep → pkg-b worktree resolves pkg_a from the pub cache, not the pkg-a worktree. A hand-written package-level `pubspec_overrides.yaml` fixes it (`dart run` → `b+a@worktree`). Overrides for packages outside the graph are NOT ignored by pub (added to package_config), and an unexcluded file shows as `??` in the package repo.
- L3 (2026-11-27, user): "Si" (implement as proposed, on by default).
- L4 T1 (5a0a6a6): RED 9 tests on a base copy → GREEN; shared `renderOverrides` computes the root and each package file; transitive closure follows `dependencies`/`dev_dependencies` of the package and only `dependencies` of transitive nodes; root output byte-identical (existing override tests unchanged). Parent: suite green in both locales; QA scenario /tmp/tmp.nl88IDO7l8 — only pkg-b gets a file (`pkg_a: ../pkg-a`), repos clean, pubget and complete exit 0.
- L5 PR #57 Codex P2: transitive members' dev_dependencies leaked into other package files and the root file (pre-existing for the root since 2.0.0). Fixed: only repos[0] (the package being resolved) contributes dev_dependencies; RED→GREEN TestE2EPackageOverridesIgnoreTransitiveDevDependencies; suite green in both locales.
