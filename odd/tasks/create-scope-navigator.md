Objective: let `flutree create <name>` run from any directory and pick the scope folder interactively, then the root and packages from the repos found there.
Branch: `feat/create-scope-navigator` (stacked on `feat/e2e-suite`) · Delivery: `ask-on-risk` · Runner: `./scripts/e2e.sh` (PTY e2e tests) + `go test ./...`.

## Specs

- S1 — User request: "donde la persona ejecute el comando en cualquier lugar de su directorio, Flutree create nombre, se debería poder navegar y seleccionar la carpeta del root y así mismo los paquetes". Accepted design (user: "Si"): hybrid — choose the scope folder with a navigator once, then pick root and packages from the discovered repos.
- S2 — Interactive `create` whose resolved scope (`--scope` > `scope.root` > `.`) has no Flutter repositories opens a directory navigator starting at that scope instead of failing with "No Flutter repositories found in scope.". When repos are found the wizard starts as today, and a visible key opens the navigator from the root-repo step to change scope.
- S3 — Navigator: shows the current absolute path and its non-hidden subdirectories (sorted) plus `..`; Enter opens a directory, a visible key selects the current directory as scope. After selecting, flutree discovers repos there; if none are found it stays in the navigator with a message naming the directory.
- S4 — Root and package steps keep listing only discovered repos and gain type-to-filter (case-insensitive substring over repo name, package name and path).
- S5 — When the scope was chosen in the navigator and differs from the persisted `scope.root`, the review step offers "Save <dir> as default scope (scope.root)" (visible toggle, on by default); on apply it is persisted with the existing config service. Assumption: default on because it is shown explicitly.
- S6 — Non-interactive mode and explicit flags behave as before. Out of scope: the add-repo wizard and multiple scope folders.

## Tasks

| ID | Specs | Route | Status | Commit |
| --- | --- | --- | --- | --- |
| T1 | S1–S6 | delegated writer (context backstop: long parent session) | in progress | — |

## Log

- L1 (2026-11-26, user): "No, ese lo vamos a seguir usando, ahora necesito este escenario, donde la persona ejecute el comando en cualquier lugar de su directorio, Flutree create nombre, se debería poder navegar y seleccionar la carpeta del root y así mismo los paquetes, qué opinas de este approch ?"
- L2: Parent recommended the hybrid design (scope navigator once + filtered repo lists + offer to save scope.root) over navigating to each repo; user: "Si". Today: cmd/flutree/create.go discovers repos in the resolved scope, ui.RunCreateWizard fails at internal/ui/create_wizard.go:91 when none; steps are name → root → packages → branches → review; no filtering. The QA scenario in /tmp/tmp.nl88IDO7l8 must be kept.
