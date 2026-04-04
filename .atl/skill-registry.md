# Skill Registry

Generated for project: **flutree**

## User Skills

Project-level skill directories were not found (`.agent/skills`, `skills`), so user-level skills are active.
`sdd-*`, `_shared`, and `skill-registry` skills are intentionally excluded.

| Skill | Source | Path | Trigger (from frontmatter description) |
|---|---|---|---|
| go-testing | user | `/Users/endersonvizc/.config/opencode/skills/go-testing/SKILL.md` | When writing Go tests, using teatest, or adding test coverage |
| skill-creator | user | `/Users/endersonvizc/.config/opencode/skills/skill-creator/SKILL.md` | When user asks to create a new skill, add agent instructions, or document patterns for AI |
| branch-pr | user | `/Users/endersonvizc/.config/opencode/skills/branch-pr/SKILL.md` | When creating a pull request, opening a PR, or preparing changes for review |
| issue-creation | user | `/Users/endersonvizc/.config/opencode/skills/issue-creation/SKILL.md` | When creating a GitHub issue, reporting a bug, or requesting a feature |
| judgment-day | user | `/Users/endersonvizc/.config/opencode/skills/judgment-day/SKILL.md` | When user says “judgment day”, “judgment-day”, “review adversarial”, “dual review”, “doble review”, “juzgar”, or “que lo juzguen” |

## Project Conventions

- No convention files found in project root (`agents.md`, `AGENTS.md`, `CLAUDE.md`, `.cursorrules`, `GEMINI.md`, `copilot-instructions.md`).

## Compact Rules

### go-testing
- Prefer table-driven tests in Go.
- For Bubble Tea UI flows, test model state transitions directly.
- Use deterministic tests and explicit expected outputs.

### branch-pr
- PRs must link an approved issue.
- Use one and only one `type:*` label.
- Follow conventional commits and branch naming `type/description`.

### issue-creation
- Use issue templates (no blank issues).
- New issues start as `status:needs-review` and require `status:approved` before PR.

### judgment-day
- Run two independent parallel blind reviews.
- Synthesize overlaps/contradictions, then fix and re-judge.

### skill-creator
- Create skills only for repeatable, non-trivial patterns.
- Keep SKILL.md focused on triggers, critical patterns, and minimal examples.
