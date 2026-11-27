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
| T1 | S1–S6 | delegated writer (context backstop: long parent session) | done | (navigator commit) |

## Log

- L1 (2026-11-26, user): "No, ese lo vamos a seguir usando, ahora necesito este escenario, donde la persona ejecute el comando en cualquier lugar de su directorio, Flutree create nombre, se debería poder navegar y seleccionar la carpeta del root y así mismo los paquetes, qué opinas de este approch ?"
- L2: Parent recommended the hybrid design (scope navigator once + filtered repo lists + offer to save scope.root) over navigating to each repo; user: "Si". Today: cmd/flutree/create.go discovers repos in the resolved scope, ui.RunCreateWizard fails at internal/ui/create_wizard.go:91 when none; steps are name → root → packages → branches → review; no filtering. The QA scenario in /tmp/tmp.nl88IDO7l8 must be kept.
- L3 T1 (delegated writer, sonnet): navigator `internal/ui/dir_navigator.go`; keys: `ctrl+o` opens it from the root step, Up/Down (j/k in navigator), Enter opens, `Tab` selects current folder, Esc back/cancel; root/package steps filter by typing (Backspace edits, Esc clears); review `s` toggles saving scope.root. RED: 6 PTY navigator tests failed (CLI printed "No Flutter repositories were discovered…") → GREEN. Differential check vs base for non-interactive create (with/without repos, with/without bad --scope): identical. Parent: writer stayed in surfaces; moved the scope.root save before the JSON/human branches (it was skipped with --json) and made a save failure a stderr warning instead of failing a finished create. Full suite and e2e gate green. Known: on root/package steps j/k became filter text (arrows still move); the PTY harness leaks a terminal colour reply into the name field (pre-existing, tests match by prefix).
