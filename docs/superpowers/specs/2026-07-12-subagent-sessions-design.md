# Subagent Sessions — Design

**Date:** 2026-07-12
**Status:** Approved

## Goal

Make subagent work first-class in ccwhid reports:

1. Agent results fed back into the main session always render as a card
   attributed to the agent — never as a "You" card.
2. The transcripts of a root session's subagent sessions can optionally be
   included in the report as separate linked HTML pages.
3. When subagent sessions are included, their token usage and estimated cost
   roll up into the main usage summary.

## Background: how subagent data appears on disk

Three generations of subagent storage exist in Claude Code transcripts:

1. **In-file sidechains** — records with `isSidechain: true` inside the root
   session's own `.jsonl`. Already handled by ccwhid (`--include-subagents`
   renders them inline inside the Task tool card).
2. **Linked agent-session files (modern Agent tool)** —
   `<projectDir>/<sessionId>/subagents/agent-<agentId>.jsonl`, one file per
   agent, each with a sibling `agent-<agentId>.meta.json`:

   ```json
   {
     "agentType": "general-purpose",
     "description": "Implement Task 12: Profiles view",
     "toolUseId": "toolu_0185fny8NUzysKdpquY27j3q",
     "spawnDepth": 1
   }
   ```

   Records inside the agent file are ordinary user/assistant records
   (they carry `isSidechain: true` and an `agentId` field). `toolUseId`
   links the agent to the Task/Agent tool call in the parent transcript.
   `spawnDepth` can exceed 1 (agents spawned by agents); all files live
   flat in the one `subagents/` dir regardless of depth.

   Observed scale: a single root session with 256 agent files totalling
   82 MB of JSONL. This rules out inlining transcripts into one HTML file.
3. **Flat SDK-spawned session files** — separate top-level session files with
   `entrypoint: sdk-*` / `promptSource: "sdk"` (e.g. review-hook runs). They
   carry **no parent-session linkage** and cannot be reliably attributed to a
   root session.

**Agent results in the main timeline** arrive in one of two shapes:

- Synchronous agent: the `tool_result` attaches to the Agent/Task tool call
  (already rendered inside the tool card — correct today).
- Background agent: a `user` record whose message content is a **string**
  starting with `<task-notification>`, e.g.

  ```
  <task-notification>
  <task-id>acb6584f99f2f81fd</task-id>
  <tool-use-id>toolu_0112a19E...</tool-use-id>
  <output-file>/private/tmp/.../tasks/acb6584f99f2f81fd.output</output-file>
  <status>completed</status>
  <summary>Agent "Implement Task 12: Profiles view" finished</summary>
  <note>...may notify more than once...</note>
  <result>...full agent report, markdown...</result>
  </task-notification>
  ```

  ccwhid currently renders this as a plain user turn — the "You" card bug.
  The `<task-id>` value equals the `agentId` in the linked file name;
  `<tool-use-id>` identifies the originating tool call.

## Decisions

| Decision | Choice |
|---|---|
| Transcript delivery | Separate linked HTML pages, one per agent session |
| Usage presentation | Merged per-model table + "of which subagents" line |
| Flag | Reuse `--include-subagents` for both sidechains and linked agent sessions |
| Result card | Agent-attributed card at its timeline position |

## Design

### 1. Data model

`model.Session` gains:

```go
Agents []AgentSession
```

```go
// AgentSession is a subagent session file linked to this root session.
type AgentSession struct {
    ID          string  // agentId, from the file name agent-<ID>.jsonl
    Description string  // from meta.json; falls back to ID
    AgentType   string  // from meta.json; may be empty
    ToolUseID   string  // links to the spawning tool call; may be empty
    SpawnDepth  int     // from meta.json; informational only
    Session     Session // the parsed agent transcript
}
```

New turn kind and fields for the notification card:

```go
const TurnAgentResult TurnKind = "agent-result" // alongside "user"/"assistant"
```

`model.Turn` gains `AgentID`, `AgentStatus`, `AgentSummary string` (set only
for `TurnAgentResult` turns). The `<result>` body becomes the turn's single
text block, rendered as markdown.

### 2. Parser: task-notification detection

In `internal/transcript`, when a `user` record's message content is a string
whose trimmed text starts with `<task-notification>`:

- Parse the `task-id`, `tool-use-id`, `status`, `summary`, and `result`
  elements with tolerant string scanning (no XML parser; the payload is
  pseudo-XML with unescaped markdown inside `<result>`).
- Produce a `TurnAgentResult` turn: `AgentID` = task-id, `AgentStatus` =
  status, `AgentSummary` = summary, blocks = one text block containing the
  result body. If `<result>` is missing or empty, use the summary text as
  the body.
- If parsing fails to find both `<task-id>` and `<summary>`, fall back to a
  plain user turn (current behavior) — never drop the record.
- A task-id may notify more than once; each notification renders as its own
  card at its timeline position (no dedup).

This fix is **unconditional** — it applies regardless of `--include-subagents`
and `--usage`. Misattribution is a bug, not an option.

### 3. Loader: linked agent sessions

New package-level function in `internal/transcript`:

```go
// LoadAgentSessions parses <dir>/subagents/agent-*.jsonl, where dir is the
// transcript path minus ".jsonl". Returns them sorted by file name.
func LoadAgentSessions(transcriptPath string, opts Options) ([]model.AgentSession, error)
```

- Called from `run.go` only when `--include-subagents` is set.
- Each agent file is parsed with the existing `Parse`. Inside an agent's own
  file its records are the main chain, even though they carry
  `isSidechain: true`; the parser must treat them as main-chain records when
  parsing an agent file. Concretely: `Options` gains `AgentFile bool`; when
  set, `IsSidechain` on records is ignored.
- Metadata from `agent-<ID>.meta.json`; a missing or unreadable meta.json is
  tolerated (Description falls back to ID, other fields zero).
- A missing `subagents/` dir yields an empty slice, no error. An unparsable
  agent file is skipped with a warning to stderr, not a fatal error.

### 4. Renderer: agent-result cards and subagent pages

**Agent-result card** (always): a `TurnAgentResult` turn renders as a card
with role label `Agent` plus the summary (e.g. `Agent · Implement Task 12:
Profiles view`), a status badge (`completed` / other), and the result body
as markdown. Distinct accent styling from You/Claude cards. Search text and
prompt-jump behavior treat it like an assistant card (it is not a prompt).

**Subagent pages** (when agent sessions are loaded):

- Output layout: `outDir/subagents/agent-<ID>.html`, sharing
  `outDir/assets/` via relative paths (`../assets/...`).
- Each page uses the same report template: title = agent description,
  subtitle shows agent type and ID, plus a back-link to `../index.html`.
- Each page gets its own usage card when `--usage` is set (computed over
  that agent session only, current single-session semantics).
- Links from the main report into the pages:
  - On the Agent/Task **tool card** whose `tool_use` ID equals an agent's
    `ToolUseID`.
  - On every **agent-result card** whose `AgentID` matches an agent file.
- Subagent pages themselves may contain Agent tool cards and agent-result
  cards for deeper agents (spawnDepth 2); the same matching renders links to
  sibling pages in the same directory. Link resolution uses the full agent
  set of the root session.
- Agent files referenced by no tool card and no notification are still
  rendered as pages (unreachable by link); acceptable.

### 5. Usage rollup

When agent sessions are loaded and `--usage` is set:

- `usage.Compute` gains the agent sessions as input (signature stays
  session-centric: it reads `s.Agents`). Agent turns merge into the same
  per-model aggregation (same model-id normalization) and into `Total`.
- `Report` gains a subagent subtotal:

  ```go
  Subagents      TokenCounts // tokens from linked agent sessions
  SubagentsCost  *float64    // nil when nothing in them was priced
  AgentSessions  int         // number of linked agent sessions counted
  ```

- The usage card shows, below the table:
  `of which subagents: <N tok in+out> · ~$X (M sessions)` — omitted when no
  agent sessions were counted.
- Footnote when agent sessions are included: "Includes M linked subagent
  sessions. Server-tool fees are excluded." Current wording stays when the
  flag is off or no agent files exist.
- Per-turn badges remain main-session-only.
- In-file sidechain turns keep their current treatment (already counted via
  `walk`).

### 6. Flags & TUI

- `--include-subagents` now means "show subagent work": render in-file
  sidechains inline (unchanged) **and** discover + render linked
  agent-session pages. Help text updated.
- TUI: the existing subagents option's label is updated to reflect the
  broader meaning. No new option.
- README: document the new behavior and the page layout.

## Out of scope

- Flat SDK-spawned session files (no linkage) — excluded; documented as a
  known limitation in the README.
- Inlining full agent transcripts into the main HTML.
- Deduplicating repeat notifications for the same task-id.
- Pricing-file override (still a future idea from the usage spec).

## Testing

- **Parser:** task-notification record → `TurnAgentResult` with correct
  AgentID/Status/Summary and result body; user text that merely mentions
  `<task-notification>` mid-string stays a user turn; malformed notification
  falls back to a user turn; multiple notifications for one task-id each
  produce a turn.
- **Loader:** finds `agent-*.jsonl` under `<sessionId>/subagents/`; missing
  dir → empty; missing meta.json → ID fallback; `AgentFile` option makes
  sidechain-flagged records parse as main chain.
- **Usage:** rollup adds agent-session tokens to totals and per-model rows;
  subtotal line values; `AgentSessions` count; footnote variants.
- **Renderer:** agent-result card carries `Agent ·` label and status badge
  (not "You"); subagent pages are written to `subagents/agent-<ID>.html`;
  tool cards and result cards carry links when a matching agent exists and
  none when not; back-link present on agent pages.
