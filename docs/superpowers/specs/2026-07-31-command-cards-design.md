# Slash-Command Cards — Design

**Status:** approved (2026-07-31)

**Goal:** A Claude Code slash command renders as one card containing the
invocation and its output, with one clean sidebar prompt entry — instead
of today's two cards and two markup-riddled prompt entries.

## Background

A slash command produces two consecutive `user` records in the
transcript:

1. Input: a single text block like
   `<command-name>/model</command-name>\n<command-message>model</command-message>\n<command-args></command-args>`
   (whitespace between tags varies).
2. Output: a single text block like
   `<local-command-stdout>Set model to \x1b[1mFable 5\x1b[22m …</local-command-stdout>`
   (may contain ANSI; `<local-command-stderr>` also exists).

Today both become ordinary user turns: goldmark drops the unknown tags in
the cards, but the plain-text prompt previews expose the raw markup.

## Decisions

- **Invocation text:** command name + args, joined with a space when args
  are non-empty — `/model`, `/code-review ultra 21`. The
  `<command-message>` echo is dropped.
- **Merge:** parse-time. The output record does not become its own turn;
  its text attaches to the preceding command turn. One turn → one card →
  one prompt entry. Absorption requires the output record to immediately
  follow the command record among main-chain turns (tracked via explicit
  open-command parse state, not inferred from an empty `Output`); an
  output record seen after the command already received output — even
  empty output — is treated as orphaned.
- **Representation:** a new block type, not string rewriting — the
  renderer needs to distinguish invocation (code-styled line) from output
  (monospace block).
- **ANSI:** kept in the model, stripped at render (consistent with tool
  results).

## Design

### Model (`internal/model`)

```go
const BlockCommand BlockType = "command"

// Command is a slash-command invocation and its captured output.
type Command struct {
	Invocation string // "/model", "/code-review ultra 21"
	Output     string // local-command stdout/stderr text; may contain ANSI
}
```

`Block` gains `Command *model.Command` (set only for `BlockCommand`).

### Parser (`internal/transcript`)

Two extractors over a turn's text (operating on the concatenated text of
a user turn's blocks; in practice these turns have exactly one text
block):

- `parseCommand(text) (invocation string, ok bool)` — `ok` when a
  `<command-name>` tag is present; invocation is the tag's content plus,
  when non-empty, a space and the `<command-args>` content. Surrounding
  whitespace trimmed.
- `parseCommandOutput(text) (out string, ok bool)` — `ok` when a
  `<local-command-stdout>` or `<local-command-stderr>` tag is present;
  `out` is the concatenated tag contents (stdout first, then stderr,
  separated by a newline when both are non-empty), trimmed of
  leading/trailing whitespace.

Main-chain loop changes, after `buildTurn` returns a user turn:

- Command input turn (`parseCommand` ok): the turn's blocks are replaced
  by a single `BlockCommand` block. Kind stays `TurnUser`; timestamp and
  everything else unchanged.
- Output turn (`parseCommandOutput` ok): absorption is governed by explicit
  parse state, an `openCommand` flag that is true only while the
  immediately preceding appended main-chain turn is a command turn still
  awaiting its output record. If `openCommand` is true, the extracted text
  becomes that command turn's `Output` (even when the text is empty) and
  the flag is cleared; the output turn itself is not appended. Otherwise
  (orphan — the command already received its output, or nothing suitable
  precedes), the turn IS appended as a plain user turn with its text
  blocks replaced by the extracted output text, so raw tags never leak.
  Because openness is tracked explicitly rather than inferred from
  `Output == ""`, a second output record following one that was already
  absorbed (even an empty one) is correctly orphaned instead of
  overwriting the first.
- Non-user turns and turns without the markup: unchanged. Sidechain
  (subagent) records are untouched — slash commands do not occur there.

### Renderer (`internal/render`)

- `renderTurnBody` gains a `BlockCommand` case:

  ```html
  <div class="command">
    <code class="command-invocation">/model</code>
    <pre class="command-output">Set model to Fable 5 …</pre>   <!-- only when Output != "" -->
  </div>
  ```

  Both parts HTML-escaped; output passed through `StripANSI`.
- `turnPlainText` gains a `BlockCommand` case contributing
  `Invocation + " " + Output` (ANSI already stripped by the existing
  final `StripANSI`), so the sidebar preview starts with `/model` and
  search matches the output.
- CSS: `.command-invocation` — small code chip (monospace, `--code-bg`
  background, padding, border-radius); `.command-output` — reuses the
  visual style of `pre` blocks already in cards (no new look), muted
  color. No JS changes.

## Error handling

- Command turn with no following output record: card shows the
  invocation line only (`Output` empty).
- Orphaned output record: plain user card with the clean output text.
- Malformed/unclosed tags: extractors require a matching closing tag;
  without one they report `ok == false` and the turn passes through
  unchanged (current behavior).
- Empty `<command-args>`: invocation is the bare name — no trailing
  space.

## Testing

- Extractor unit tests: name only; name + args; missing closing tag →
  not ok; stdout only; stderr only; stdout + stderr; ANSI preserved in
  extraction.
- Parser tests (fixture JSONL lines): command + stdout records → one
  `TurnUser` with a single `BlockCommand` (invocation `/model`, output
  set); command with no output → `Output` empty; orphaned stdout →
  plain user turn, clean text, no tags; a normal user prompt between
  command and stdout keeps the stdout orphaned (no mis-attachment); a
  second (stray) output record following one already absorbed — even an
  empty one — is orphaned rather than mis-attached to the closed command.
- Render tests: command card markup (`command-invocation` code,
  `command-output` pre), ANSI stripped from output, HTML escaped;
  `turnPlainText`/prompt preview shows `/model …` with no `<` characters.
- E2E: a session with a slash command renders one card and one sidebar
  entry for it.

## Out of scope

- `<bash-input>`/`<bash-stdout>` (`!` prefix) records.
- Collapsing long command output (no `<details>`; output is typically
  short).
- Sidechain/subagent command handling.
