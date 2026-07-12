# ccwhid Usage & Cost Section — Design

**Status:** approved (2026-07-12)

**Goal:** Add an opt-in report section that shows token usage and an estimated
cost for a rendered session, computed entirely from the `message.usage` data
already present in Claude Code transcripts — comparable to `/usage` or the
`ccusage` tool, but self-contained and offline.

## Motivation

Every assistant record in a transcript carries a full `usage` object and the
`model` id, e.g.:

```json
"model": "claude-opus-4-8",
"usage": {
  "input_tokens": 10223,
  "output_tokens": 192,
  "cache_creation_input_tokens": 3008,
  "cache_read_input_tokens": 18878,
  "cache_creation": { "ephemeral_1h_input_tokens": 3008, "ephemeral_5m_input_tokens": 0 },
  "server_tool_use": { "web_search_requests": 0, "web_fetch_requests": 0 }
}
```

Token counts are therefore **exact and free to compute**. Only cost requires a
price table, which is the one estimated element of the feature.

## Scope

- **Opt-in** via a `--usage` flag (default off). The section is only computed
  and rendered when requested.
- Displays, when enabled:
  1. A **collapsible "Usage" card** near the top of the report (summary +
     per-model breakdown).
  2. A **per-turn cost badge** on each assistant turn header.
- Cost is an **estimate** from an embedded, dated price table.

### Out of scope for v1

- **Server-tool request fees** (web search / web fetch requests have their own
  per-request pricing). v1 costs LLM token usage only; excluded fees are called
  out in the report footnote and revisited later (see Future Ideas).
- Live/remote price fetching at runtime (the tool is offline by design).

## Data Model

Extend the domain model so usage travels with each turn:

- `model.Usage{ Input, Output, CacheRead, CacheWrite5m, CacheWrite1h int }`
- `model.Turn` gains `Model string` and `Usage *Usage`.

The `cache_creation.ephemeral_5m_input_tokens` / `ephemeral_1h_input_tokens`
split is preserved because 5-minute and 1-hour cache writes are priced
differently. If a record only has the aggregate `cache_creation_input_tokens`
and no split, the whole amount is treated as a 5-minute write (the cheaper,
default TTL).

## Transcript Parsing

`internal/transcript` decodes `message.model` and `message.usage` for each
**assistant** record and attaches a `model.Usage` (plus `Model`) to the turn it
produces. Records without a `usage` object yield `Usage == nil` (no badge, not
counted). User records carry no usage.

## Pricing (`internal/usage`)

- An **embedded static table** (`prices.json`, `go:embed`) keyed by model id.
  Each entry stores only the two base per-million-token USD rates, `input` and
  `output`. Cache rates are **derived** from `input` via the universal Anthropic
  multipliers (5m write = 1.25×, 1h write = 2×, cache read = 0.1×) applied in the
  cost function, so they are not duplicated per model in the table.
- A `PricesAsOf` date string (e.g. `"2026-07"`) is embedded and surfaced in the
  report footnote.
- The table is **populated from Anthropic's published list prices at
  implementation time** (a dev-time lookup to embed accurate numbers); the
  running binary performs no network access. Refreshing prices is a manual
  edit + release.
- Model resolution is exact-id first. `<synthetic>` and any id absent from the
  table resolve to **no price** (tokens still counted; cost shown as `n/a`).

## Computation (`internal/usage`)

`Compute(session) Report` walks the session's turns (main chain and, when
included, nested subagent turns) and aggregates:

- **Per turn:** token totals and, if the model is priced, a cost.
- **Per model:** summed tokens and cost (or `nil` cost when unpriced).
- **Grand totals:** summed tokens; total cost = sum of priced models only.

Cost for a model = Σ over token types of `tokens_type × rate_type ÷ 1_000_000`,
using the 5m/1h split for cache writes.

```
Report{
  Total        TokenCounts
  TotalCostUSD *float64      // nil only if nothing was priced
  ByModel      []ModelUsage  // { Model, Tokens TokenCounts, CostUSD *float64 }
  PerTurnCost  map[int]*float64 // turn index → cost (nil = unpriced/no usage)
  PricesAsOf   string
  HasUnknownModel bool       // some tokens came from an unpriced model
  HasAnyUsage  bool          // false → transcript carried no usage at all
}
```

## Rendering (`internal/render`)

Rendering is gated on `render.Options{ Usage bool }`.

**Usage card** (collapsible, placed after the report meta line):

- Collapsed one-liner:
  `▸ Usage · 973k in+out · ~$12.40 (est.)`
  The headline token figure is **input + output only**, so it is not dominated
  by cache-read volume (which can be orders of magnitude larger).
- Expanded body:
  - Token breakdown: input / output / cache read / cache write.
  - Per-model table: `model · tokens · cost` (or `n/a`).
  - Muted footnote:
    *"Estimated — Anthropic list prices as of `<PricesAsOf>`; excludes
    server-tool fees."* When `HasUnknownModel`, append: *"totals exclude
    unpriced models (shown as n/a)."*

**Per-turn badge:** a small muted element in each assistant turn header, e.g.
`· 12.3k tok · ~$0.18`. Unpriced/synthetic turns show `· 12.3k tok` (no cost).
Turns without usage show no badge.

**No-data case:** with `--usage` set but `HasAnyUsage == false`, render a small
muted note ("no token-usage data in this transcript") instead of an empty card,
so the flag is never silently inert.

Styling reuses existing report design tokens (warm palette, muted text, the
same collapse mechanism as tool cards); no new external assets, preserving the
self-contained/offline guarantee. Usage numbers are not secrets, so redaction
does not touch them.

## CLI & TUI

- **CLI:** add `--usage` (bool, default false) wired to `render.Options.Usage`.
- **TUI:** add an "Include usage & cost" toggle to the options screen (default
  off), carried on `tui.Selection` and mapped to the same option.

## Error Handling & Edge Cases

- Unpriced model → tokens shown, cost `n/a`, footnote notes excluded totals.
- Missing/partial usage on a record → that turn contributes no cost and no
  badge; parsing never fails on absent fields.
- Malformed `usage` JSON → treated as absent (consistent with the parser's
  existing tolerance), not a fatal error.

## Testing

- `internal/usage`:
  - price lookup: known id returns rates; unknown / `<synthetic>` returns
    not-found.
  - `Compute` over a fixture session spanning two models with a 5m/1h cache
    split → exact per-type token totals and cost; an unpriced model yields
    `nil` cost and sets `HasUnknownModel`.
  - headline in+out figure excludes cache tokens.
- `internal/transcript`: a fixture asserts `model`/`usage` (incl. cache split)
  are decoded onto the produced turns; a record without usage yields
  `Usage == nil`.
- `internal/render`: the usage card renders only with `Usage: true`; a per-turn
  badge is present; the unknown-model footnote appears; the no-data note renders
  when usage is absent.

## Component Boundaries

- `internal/usage` owns pricing and aggregation; it depends only on
  `internal/model`. It has no rendering or I/O concerns and is unit-tested in
  isolation.
- `internal/transcript` is the only place that knows the raw `usage` JSON shape.
- `internal/render` consumes a `usage.Report` and knows nothing about pricing
  math.

## Files

- New: `internal/usage/pricing.go`, `internal/usage/usage.go`,
  `internal/usage/prices.json`, `internal/usage/*_test.go`.
- Edit: `internal/model/model.go`, `internal/transcript/raw.go`,
  `internal/transcript/parse.go`, `internal/render/render.go`,
  `internal/render/assets/report.html.tmpl`, `internal/render/assets/styles.css`,
  `cmd/ccwhid/main.go`, `internal/tui/tui.go`, `README.md`.

## Future Ideas

- **`--pricing <file>` override.** Allow loading a JSON price file at runtime to
  override the built-in table and/or supply rates for models the embedded table
  does not know (so users can correct stale prices or price custom/unknown
  models without rebuilding). The file would be merged over the embedded
  defaults by model id; the report footnote would note when an override file was
  used. Deferred from v1 to keep the initial surface small; the pricing lookup
  is designed so an override layer can slot in cleanly.
- **Server-tool fees.** Include web-search / web-fetch request costs
  (`usage.server_tool_use`) once per-request rates are added to the table.
- **Aggregate across sessions.** A future mode could total usage/cost across all
  sessions in a project (closer to `ccusage`'s daily rollups).
