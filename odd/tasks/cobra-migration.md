Objective: replace the hand-rolled stdlib `flag` CLI in `cmd/flutree` with spf13/cobra, adopting Cobra's help and error conventions.
Branch: `feat/cobra-migration` · Delivery: `ask-on-risk` · Runner: `go test ./...` (integration builds the binary).

## Specs

- S1 — Migrate the CLI to Cobra: "hagamos la migración a cobra que lo hace mejor que nosotros". Every existing command, subcommand and flag keeps working: `create`, `add-repo`, `config set|get`, `list`, `complete`, `pubget`, `clean`, `update`, `version`, `mcp serve|install`.
- S2 — Visible CLI behavior: user chose "Adoptar lo de Cobra" — "Ayuda y errores al estilo Cobra. Hay que actualizar tests y docs, y es un cambio incompatible para quien tenga scripts que lean esa salida, así que pide una versión mayor (2.0.0)." Help is Cobra-generated; unknown commands and flag errors use Cobra/pflag messages.
- S3 — Preserved (assumption, not requested to change): exit codes (input = 2, unexpected = 1), the `[category] message` / `Hint:` stderr envelope, the `--json` JSON error envelope, `--json` accepted before or after the command, `mcp serve` rejecting `--json`, `version` and `--version` printing the same bare value, domain validation messages (e.g. "Missing worktree name.").
- S4 — Docs and tests updated to the new output; commit marked breaking so release-please cuts 2.0.0.

## Tasks

| ID | Specs | Route | Status | Commit |
| --- | --- | --- | --- | --- |
| T1 | S1, S2, S3 | inline (one coherent unit, no writer reason) | done | 847edb8 |
| T2 | S4 | inline | done | (docs commit) |

## Log

- L1 (2026-11-26, user): "Entonces quitemos esa skill y hagamos la migración a cobra que lo hace mejor que nosotros."
- L2 (2026-11-26, user decision): "Adoptar lo de Cobra".
- L3: gentleman-bubbletea skill removed. Baseline `go test ./...` green. Pinned strings to update: subcommand usage lines (`[options]` → `[flags]`), root help hint, "No such command". Engram mirror pending: no Engram tools in this session.
- Risk: item 3 (CLI output contract, requested) and item 5 (new dependency spf13/cobra).
- L4 T1: RED observed (12 failing tests: help usage lines, unknown command, completion, flag errors, flags before name, group help, config unknown action). GREEN: `go test ./...` all ok. Differential check vs `main` on 28 invocations: identical except intended `[options]`→`[flags]` hints and leading `--json` now formatting output (old code ignored it; pinned by TestLeadingJSONFlagFormatsCommandOutput). golangci-lint not installed locally — runs in CI.
- L5 T2: README (2.0 notes, global `--json`, `completion` section, `[flags]` usage) and docs/architecture.md (Cobra command layer, how to add a command). Docs contract tests ok.
