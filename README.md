# ccwhid — cc-what-have-i-done

Turn a Claude Code session transcript into a browsable, self-contained static
HTML report. See what you prompted and what Claude Code did — and commit the
report into your repo as a documentation artifact.

> **Status:** under active development. See
> [`docs/superpowers/plans/`](docs/superpowers/plans/) for the implementation plan.

## Install

```bash
go install github.com/saigyo/cc-what-have-i-done/cmd/ccwhid@latest
```

Or build locally:

```bash
go build -o ccwhid ./cmd/ccwhid
```

## Usage

```bash
ccwhid                      # browse sessions in an interactive TUI
ccwhid --latest             # render the most recent session
ccwhid --session <id>       # render a specific session (id or prefix)
ccwhid --session <id> --open
```

The report is written to `./ccwhid-report/<session-short>/` by default (override
with `--out`). Open `index.html` in any browser — no server needed.

### Flags

| Flag | Description |
|------|-------------|
| `--session <id>` | Session id or unambiguous prefix to render |
| `--project <path\|name>` | Scope `--latest` to a project |
| `--latest` | Render the most recent session |
| `--out <dir>` | Output directory |
| `--title <str>` | Override the report title |
| `--include-subagents` | Include subagent (Task) activity (default true; `--include-subagents=false` to omit) |
| `--no-redact` | Disable secret redaction |
| `--force` | Overwrite a non-empty output directory |
| `--open` | Open the report in a browser when done |

## Redaction

By default, ccwhid scrubs common secret shapes (AWS keys, API tokens, JWTs,
`KEY=`/`TOKEN=`/`SECRET=` assignments) and rewrites your home directory to `~`.
This is best-effort defense-in-depth — **review generated reports before
committing them.** Disable with `--no-redact`.

## What's included

Your prompts, Claude's replies (rendered markdown + syntax-highlighted code),
tool calls with collapsible detail, `Edit`/`Write` diffs, collapsed thinking
blocks, and — optionally — nested subagent activity. System reminders and
attachments are omitted for readability.

## License

[MIT](LICENSE) © 2026 Markus Ackermann
