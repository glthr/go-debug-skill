# Skill source (canonical template)

The **single source of truth** for the delve debugging skill is `delve.tmpl`. It is a [Go `text/template`](https://pkg.go.dev/text/template) that defines the shared behavior and agent-specific sections.

## Layout

- **`delve.tmpl`** — Canonical skill content. Conditional blocks (`{{if .PreActionGate}}` etc.) control agent-specific bits (e.g. Pre-action gate for Claude, `/delve` slash command for Cursor).

## Generating skill files

From the repo root:

```bash
make generate-skills
# or
go run ./cmd/skillgen
```

This writes into the repo skill tree (these paths are gitignored; only the template is committed):

| Agent  | Output path                    |
|--------|--------------------------------|
| Claude | `skills/claude/delve.md`       |
| Codex  | `skills/codex/delve/SKILL.md`  |
| Cursor | `skills/cursor/delve-debug.mdc` |

`make install` runs `make skills` first (generating into `skills/out/`), then installs from `skills/out/` into `~/.cursor/rules`, `~/.claude/skills`, and `~/.codex/skills`.

**Review build (no overwrite of repo skills):** generate into a separate folder to manually verify the templating:

```bash
make skills
# writes to skills/out/ (claude/, codex/, cursor/)
# or use a custom folder:
make skills SKILLS_OUT=/tmp/my-review
```

**Do not edit the generated files by hand.** Edit `delve.tmpl` (and if needed `cmd/skillgen/main.go` for frontmatter or variant flags), then run `make generate-skills`.

## Template data (variants)

Controlled in `cmd/skillgen/main.go`:

- **Frontmatter** — YAML front matter (description, name, globs, alwaysApply) per agent.
- **Title** — Main heading text.
- **PreActionGate** — Claude-only “Pre-action gate” section.
- **SlashCommand** — Cursor-only `/delve` slash command block.
- **TriggerConditions** — Cursor-only “Trigger conditions” section.
- **DebugModes** — Debug modes table (Claude, Codex; off for Cursor).
- **CommandReference** — Command reference table (Cursor only).
- **SetupExtra** — Longer setup with “Interactive/script dlv commands” (Cursor only).

Core content (language detection, Quick start, 8-step protocol, Step 0–7, report rules) is shared and identical across agents.
