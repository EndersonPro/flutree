Objective: make flutree's MCP server and its installer a first-class, AI-native interface: agents understand the model, plan safely, ask the user, and drive the full workspace flow.
Branch: `feat/mcp-ai-native` (stacked on `feat/create-scope-navigator`) · Delivery: `ask-on-risk` · Runner: `./scripts/e2e.sh` (JSON-RPC e2e) + `go test ./...`.

## Specs

- S1 — User: "Necesito mejorar todo el servidor mcp, pulirlo y darle todas las herramientas que necesitará un agente para entender el funcionamiento, lanzar prompts de pregunta al usuario y todo el flujo, además de mejorar la instalación del mcp en los agentes esto si es la parte más interesante de todo el proyecto, ese soporte nativo a IA". User approved the phased plan below ("Si y delega").
- S2 (phase 1, understand) — Server `instructions` explaining the model (workspace = root + package worktrees, `pubspec_overrides.yaml`, branches, registry, SDK setup, lifecycle) and the recommended flow. Resources: `flutree://guide`, `flutree://workspaces`, `flutree://workspace/{name}` (paths, branches, overrides content, git status per worktree), `flutree://config`.
- S3 (phase 2, tools) — Exploration: `discover_repos`, `inspect_repo` (is Flutter, package name, branches, SDK pin, path deps), `list_branches`. Planning: `plan_create` (dry plan, no side effects) and `create_workspace` gated by confirmation (like the CLI `APPLY`). Inspection: `workspace_status`. Structured results; errors carry category and hint. Existing tools keep working (backward compatible names/arguments) or are explicitly superseded with the old name kept as an alias.
- S4 (phase 3, ask the user) — Elicitation when required input is missing (base branch, packages) or before destructive actions (`complete`, `--force`). When the client does not support elicitation, the tool result tells the agent exactly what to ask. MCP prompts for the flows: create workspace, add package, finish workspace.
- S5 (phase 4, install) — More clients: Cursor, VS Code (Copilot), Gemini CLI, Claude Desktop, Windsurf, besides Claude Code, OpenCode and Codex; `--scope project|user`; `--dry-run`; `flutree mcp uninstall`; `flutree mcp doctor` (binary path, config entry per client, server handshake).
- S6 — Every phase is covered by e2e tests through the real binary (JSON-RPC over stdio; a simulated client answers elicitation). Docs: `docs/mcp.md` for the server, README install section.

## Tasks

| ID | Specs | Route | Status | Commit |
| --- | --- | --- | --- | --- |
| T1 | S2, S3, S6 | delegated writer A (parallel unit; owns internal/infra/mcp, cmd/flutree/mcp.go) | in progress | — |
| T2 | S5, S6 | delegated writer B (parallel unit; owns internal/infra/mcpinstall, cmd/flutree/mcp_install.go) | in progress | — |
| T3 | S4, S6 | delegated writer after T1 (same files as T1) | pending | — |

## Log

- L1 (2026-11-26, user): "Como enviamos todo sin genera una nueva versión aún? Necesito mejorar todo el servidor mcp, pulirlo y darle todas las herramientas que necesitará un agente para entender el funcionamiento, lanzar prompts de pregunta al usuario y todo el flujo, además de mejorar la instalación del mcp en los agentes esto si es la parte más interesante de todo el proyecto, ese soporte nativo a IA"
- L2: Release answer: release-please.yml runs on push to main but goreleaser only runs `if: release_created == 'true'`, i.e. when the release PR is merged; merging to main does not release. Current MCP: 9 CLI-mirroring tools, no instructions/resources/prompts/elicitation; mcp-go v0.54.1 has WithInstructions, WithResourceCapabilities, WithPromptCapabilities, WithElicitation. Installer: claude-code, opencode, codex.
- L3 (2026-11-26, user): "Si y delega". Parent split `newMCPInstallCmd` into cmd/flutree/mcp_install.go (mechanical move, e2e green) so the two writers own disjoint files.
