# Usage & Cost Section Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an opt-in (`--usage`) report section showing token usage and estimated cost, computed from the `message.usage` data already in Claude Code transcripts.

**Architecture:** `internal/transcript` attaches per-turn `Model` + `Usage` to the domain model; a new `internal/usage` package owns an embedded price table and the aggregation/cost math (returning a `usage.Report`); `internal/render` renders a collapsible usage card + per-turn badges when enabled; the CLI `--usage` flag and a TUI toggle gate it.

**Tech Stack:** Go 1.26, `go:embed` (prices.json), `html/template`, existing goldmark/lipgloss/cobra/bubbletea stack. No new dependencies.

## Global Constraints

- Opt-in: usage is off by default; only computed/rendered when requested.
- Self-contained/offline: no runtime network access; no new external refs in embedded assets.
- Cost is an estimate from an embedded, dated table (`usage.PricesAsOf = "2026-07"`).
- Cache pricing multipliers (relative to base input price, universal across models): 5-minute cache write = 1.25×, 1-hour cache write = 2×, cache read = 0.1×.
- Unknown/`<synthetic>` model → tokens counted, cost shown as `n/a`; such tokens excluded from the cost total.
- Collapsed headline token figure counts **input + output only** (never cache tokens).
- v1 excludes server-tool request fees (web search/fetch).
- Redaction does not touch usage numbers.
- Aggregate totals include nested subagent turns; per-turn badges are for top-level turns only.

## File Structure

- `internal/model/model.go` — add `Usage` type; add `Model`, `Usage` fields to `Turn`.
- `internal/transcript/raw.go` — decode `message.model` + `message.usage`.
- `internal/transcript/parse.go` — attach model/usage to assistant turns.
- `internal/usage/pricing.go` — embedded price table + `Lookup`.
- `internal/usage/prices.json` — the embedded per-model base rates.
- `internal/usage/usage.go` — `TokenCounts`, `ModelUsage`, `Report`, `Compute`.
- `internal/render/render.go` — build usage view model + per-turn badges; `Options.Usage`.
- `internal/render/format.go` — token/cost formatting helpers.
- `internal/render/assets/report.html.tmpl` — usage card + badge markup.
- `internal/render/assets/styles.css` — usage card/badge styles.
- `cmd/ccwhid/main.go`, `cmd/ccwhid/run.go` — `--usage` flag → render option.
- `internal/tui/tui.go` — "Include usage & cost" toggle.
- `README.md` — document the flag + section.

---

### Task 1: Parse per-turn model & usage

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/transcript/raw.go`
- Modify: `internal/transcript/parse.go`
- Test: `internal/transcript/parse_test.go`

**Interfaces:**
- Produces: `model.Usage{ Input, Output, CacheRead, CacheWrite5m, CacheWrite1h int }`; `model.Turn.Model string`; `model.Turn.Usage *model.Usage`.

- [ ] **Step 1: Add the model types**

In `internal/model/model.go`, add after the `Turn` struct's current fields (extend the struct and add the `Usage` type):

```go
// Turn is a single user or assistant message, holding ordered content blocks.
type Turn struct {
	Kind      TurnKind
	Timestamp time.Time
	Blocks    []Block
	Model     string // assistant model id (empty for user turns / when absent)
	Usage     *Usage // token usage for this assistant turn; nil when absent
}

// Usage holds the token counts reported for one assistant message. Cache writes
// keep the 5-minute / 1-hour split because they are priced differently.
type Usage struct {
	Input        int
	Output       int
	CacheRead    int
	CacheWrite5m int
	CacheWrite1h int
}
```

- [ ] **Step 2: Write the failing test**

In `internal/transcript/parse_test.go`, add:

```go
func TestParseAttachesModelAndUsage(t *testing.T) {
	lines := strings.Join([]string{
		`{"type":"user","message":{"role":"user","content":"hi"},"timestamp":"2026-07-12T10:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","model":"claude-opus-4-8","content":[{"type":"text","text":"hello"}],"usage":{"input_tokens":100,"output_tokens":20,"cache_read_input_tokens":500,"cache_creation_input_tokens":30,"cache_creation":{"ephemeral_5m_input_tokens":10,"ephemeral_1h_input_tokens":20}}},"timestamp":"2026-07-12T10:00:01Z"}`,
		`{"type":"assistant","message":{"role":"assistant","model":"claude-opus-4-8","content":[{"type":"text","text":"no usage here"}]},"timestamp":"2026-07-12T10:00:02Z"}`,
	}, "\n")
	s, err := Parse(strings.NewReader(lines), Options{})
	if err != nil {
		t.Fatal(err)
	}
	// turns: [user, assistant-with-usage, assistant-without-usage]
	if len(s.Turns) != 3 {
		t.Fatalf("got %d turns, want 3", len(s.Turns))
	}
	if s.Turns[0].Usage != nil {
		t.Error("user turn should have nil usage")
	}
	a := s.Turns[1]
	if a.Model != "claude-opus-4-8" {
		t.Errorf("Model = %q", a.Model)
	}
	if a.Usage == nil {
		t.Fatal("assistant usage is nil")
	}
	want := model.Usage{Input: 100, Output: 20, CacheRead: 500, CacheWrite5m: 10, CacheWrite1h: 20}
	if *a.Usage != want {
		t.Errorf("Usage = %+v, want %+v", *a.Usage, want)
	}
	if s.Turns[2].Usage != nil {
		t.Error("assistant without usage should have nil Usage")
	}
}

func TestParseAggregateCacheCreationDefaultsTo5m(t *testing.T) {
	// Only the aggregate cache_creation_input_tokens is present (no split).
	line := `{"type":"assistant","message":{"role":"assistant","model":"claude-sonnet-5","content":[{"type":"text","text":"x"}],"usage":{"input_tokens":1,"output_tokens":1,"cache_creation_input_tokens":40}},"timestamp":"2026-07-12T10:00:00Z"}`
	s, err := Parse(strings.NewReader(line), Options{})
	if err != nil {
		t.Fatal(err)
	}
	u := s.Turns[0].Usage
	if u == nil || u.CacheWrite5m != 40 || u.CacheWrite1h != 0 {
		t.Errorf("aggregate cache_creation should default to 5m: %+v", u)
	}
}
```

Ensure the test file imports `"strings"` and `"github.com/saigyo/cc-what-have-i-done/internal/model"` (add if missing).

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/transcript/ -run TestParseAttaches -v`
Expected: FAIL — compile error (`Turn` has no field `Model`/`Usage`) or assertion failure.

- [ ] **Step 4: Decode usage in raw.go**

In `internal/transcript/raw.go`, add (keep this package free of the `model` import — return a local struct):

```go
// apiUsage mirrors the message.usage object of an assistant record.
type apiUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheCreation            struct {
		Ephemeral5m int `json:"ephemeral_5m_input_tokens"`
		Ephemeral1h int `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
}

// decodeMessageMeta extracts the model id and usage (if any) from a raw message.
func decodeMessageMeta(raw json.RawMessage) (modelID string, usage *apiUsage) {
	if len(raw) == 0 {
		return "", nil
	}
	var m struct {
		Model string    `json:"model"`
		Usage *apiUsage `json:"usage"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", nil
	}
	return m.Model, m.Usage
}
```

- [ ] **Step 5: Attach usage to assistant turns in parse.go**

In `internal/transcript/parse.go`, add a helper and call it inside `buildTurn` just before its final `return turn`:

```go
// messageMeta converts a record's message model/usage into turn fields. Only
// assistant records carry usage; the aggregate cache_creation figure (with no
// 5m/1h split) is attributed to the cheaper 5-minute write.
func messageMeta(raw json.RawMessage) (string, *model.Usage) {
	id, u := decodeMessageMeta(raw)
	if u == nil {
		return id, nil
	}
	w5, w1 := u.CacheCreation.Ephemeral5m, u.CacheCreation.Ephemeral1h
	if w5 == 0 && w1 == 0 && u.CacheCreationInputTokens > 0 {
		w5 = u.CacheCreationInputTokens
	}
	return id, &model.Usage{
		Input:        u.InputTokens,
		Output:       u.OutputTokens,
		CacheRead:    u.CacheReadInputTokens,
		CacheWrite5m: w5,
		CacheWrite1h: w1,
	}
}
```

Then in `buildTurn`, replace the trailing block:

```go
	if len(turn.Blocks) == 0 {
		return nil
	}
	return turn
}
```

with:

```go
	if len(turn.Blocks) == 0 {
		return nil
	}
	if rec.Type == "assistant" {
		turn.Model, turn.Usage = messageMeta(rec.Message)
	}
	return turn
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/transcript/ -v`
Expected: PASS (new tests plus existing ones).

- [ ] **Step 7: Commit**

```bash
git add internal/model/model.go internal/transcript/raw.go internal/transcript/parse.go internal/transcript/parse_test.go
git commit -m "feat(transcript): parse per-turn model and token usage"
```

---

### Task 2: Embedded price table

**Files:**
- Create: `internal/usage/pricing.go`
- Create: `internal/usage/prices.json`
- Test: `internal/usage/pricing_test.go`

**Interfaces:**
- Produces: `usage.Price{ Input, Output float64 }`; `usage.Lookup(modelID string) (Price, bool)`; `usage.PricesAsOf` (const string); unexported cache multiplier consts `cacheWrite5mMult=1.25`, `cacheWrite1hMult=2.0`, `cacheReadMult=0.1`.

- [ ] **Step 1: Create the price data**

Create `internal/usage/prices.json` (base per-million-token USD rates; cache rates are derived from `input` via the universal multipliers). Values are Anthropic list prices captured 2026-07:

```json
{
  "claude-opus-4-8":   { "input": 5,  "output": 25 },
  "claude-opus-4-7":   { "input": 5,  "output": 25 },
  "claude-opus-4-6":   { "input": 5,  "output": 25 },
  "claude-opus-4-5":   { "input": 5,  "output": 25 },
  "claude-opus-4-1":   { "input": 15, "output": 75 },
  "claude-sonnet-5":   { "input": 2,  "output": 10 },
  "claude-sonnet-4-6": { "input": 3,  "output": 15 },
  "claude-sonnet-4-5": { "input": 3,  "output": 15 },
  "claude-haiku-4-5":  { "input": 1,  "output": 5 },
  "claude-fable-5":    { "input": 10, "output": 50 }
}
```

- [ ] **Step 2: Write the failing test**

Create `internal/usage/pricing_test.go`:

```go
package usage

import "testing"

func TestLookupKnownModel(t *testing.T) {
	p, ok := Lookup("claude-opus-4-8")
	if !ok {
		t.Fatal("claude-opus-4-8 not found")
	}
	if p.Input != 5 || p.Output != 25 {
		t.Errorf("price = %+v, want {5 25}", p)
	}
}

func TestLookupStripsDateSuffix(t *testing.T) {
	if _, ok := Lookup("claude-haiku-4-5-20251001"); !ok {
		t.Error("dated model id should resolve after stripping the date suffix")
	}
}

func TestLookupUnknownAndSynthetic(t *testing.T) {
	if _, ok := Lookup("<synthetic>"); ok {
		t.Error("<synthetic> should not be priced")
	}
	if _, ok := Lookup("gpt-4"); ok {
		t.Error("unknown model should not be priced")
	}
}

func TestPricesAsOfSet(t *testing.T) {
	if PricesAsOf == "" {
		t.Error("PricesAsOf must be set")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/usage/ -v`
Expected: FAIL — package/functions do not exist.

- [ ] **Step 4: Implement pricing.go**

Create `internal/usage/pricing.go`:

```go
// Package usage computes token totals and estimated cost for a rendered
// session from the per-turn usage data attached by the transcript parser.
package usage

import (
	_ "embed"
	"encoding/json"
)

//go:embed prices.json
var pricesJSON []byte

// PricesAsOf is the month the embedded list prices were captured. Surfaced in
// the report so readers know how current the cost estimate is.
const PricesAsOf = "2026-07"

// Cache pricing multipliers relative to a model's base input price. These are
// universal across Claude models (Anthropic prompt-caching pricing).
const (
	cacheWrite5mMult = 1.25
	cacheWrite1hMult = 2.0
	cacheReadMult    = 0.1
)

// Price holds a model's per-million-token USD rates for base input and output.
type Price struct {
	Input  float64 `json:"input"`
	Output float64 `json:"output"`
}

var prices map[string]Price

func init() {
	if err := json.Unmarshal(pricesJSON, &prices); err != nil {
		panic("usage: invalid embedded prices.json: " + err.Error())
	}
}

// Lookup returns the price for a model id and whether it is known. A trailing
// "-YYYYMMDD" date suffix (used by some model ids) is stripped on a miss.
func Lookup(modelID string) (Price, bool) {
	if p, ok := prices[modelID]; ok {
		return p, true
	}
	if norm := stripDateSuffix(modelID); norm != modelID {
		if p, ok := prices[norm]; ok {
			return p, true
		}
	}
	return Price{}, false
}

// stripDateSuffix removes a trailing "-YYYYMMDD" (dash + 8 digits) from id.
func stripDateSuffix(id string) string {
	if len(id) < 9 {
		return id
	}
	tail := id[len(id)-9:]
	if tail[0] != '-' {
		return id
	}
	for _, c := range tail[1:] {
		if c < '0' || c > '9' {
			return id
		}
	}
	return id[:len(id)-9]
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/usage/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/usage/pricing.go internal/usage/prices.json internal/usage/pricing_test.go
git commit -m "feat(usage): embedded model price table with lookup"
```

---

### Task 3: Usage computation

**Files:**
- Create: `internal/usage/usage.go`
- Test: `internal/usage/usage_test.go`

**Interfaces:**
- Consumes: `model.Session`, `model.Turn.Usage`, `model.Turn.Model`; `Lookup`, cache multiplier consts, `PricesAsOf` from Task 2.
- Produces:
  - `usage.TokenCounts{ Input, Output, CacheRead, CacheWrite5m, CacheWrite1h int }` with method `InOut() int`.
  - `usage.ModelUsage{ Model string; Tokens TokenCounts; CostUSD *float64 }`.
  - `usage.Report{ Total TokenCounts; TotalCostUSD *float64; ByModel []ModelUsage; PerTurnCost map[int]*float64; PricesAsOf string; HasUnknownModel bool; HasAnyUsage bool }`.
  - `usage.Compute(s model.Session) Report`.

- [ ] **Step 1: Write the failing test**

Create `internal/usage/usage_test.go`:

```go
package usage

import (
	"testing"

	"github.com/saigyo/cc-what-have-i-done/internal/model"
)

func u(in, out, read, w5, w1 int) *model.Usage {
	return &model.Usage{Input: in, Output: out, CacheRead: read, CacheWrite5m: w5, CacheWrite1h: w1}
}

func TestComputeTotalsAndCost(t *testing.T) {
	s := model.Session{Turns: []model.Turn{
		{Kind: model.TurnUser},
		{Kind: model.TurnAssistant, Model: "claude-opus-4-8", Usage: u(1_000_000, 1_000_000, 1_000_000, 1_000_000, 0)},
		{Kind: model.TurnAssistant, Model: "<synthetic>", Usage: u(500, 0, 0, 0, 0)},
	}}
	r := Compute(s)

	if !r.HasAnyUsage {
		t.Fatal("HasAnyUsage should be true")
	}
	if !r.HasUnknownModel {
		t.Error("synthetic model should set HasUnknownModel")
	}
	// Totals include all token types across all models.
	if r.Total.Input != 1_000_500 || r.Total.Output != 1_000_000 {
		t.Errorf("totals wrong: %+v", r.Total)
	}
	// opus-4-8 input $5, output $25/MTok; cache read 0.1x input; 5m write 1.25x input.
	// cost = 5 + 25 + (1e6*5*0.1/1e6=0.5) + (1e6*5*1.25/1e6=6.25) = 36.75
	if r.TotalCostUSD == nil {
		t.Fatal("TotalCostUSD is nil")
	}
	if got := *r.TotalCostUSD; got < 36.74 || got > 36.76 {
		t.Errorf("total cost = %.4f, want 36.75", got)
	}
	// per-turn cost: turn index 1 priced, index 2 (synthetic) nil, index 0 (user) absent.
	if r.PerTurnCost[1] == nil {
		t.Error("turn 1 should have a cost")
	}
	if c, ok := r.PerTurnCost[2]; !ok || c != nil {
		t.Error("synthetic turn should be present with nil cost")
	}
	if _, ok := r.PerTurnCost[0]; ok {
		t.Error("user turn should not appear in PerTurnCost")
	}
}

func TestComputeCountsSubagentTurnsInTotals(t *testing.T) {
	sub := model.Subagent{Turns: []model.Turn{
		{Kind: model.TurnAssistant, Model: "claude-opus-4-8", Usage: u(1_000_000, 0, 0, 0, 0)},
	}}
	s := model.Session{Turns: []model.Turn{
		{Kind: model.TurnAssistant, Model: "claude-opus-4-8", Usage: u(1_000_000, 0, 0, 0, 0),
			Blocks: []model.Block{{Type: model.BlockToolUse, Tool: &model.ToolCall{Name: "Task", Subagents: []model.Subagent{sub}}}}},
	}}
	r := Compute(s)
	// Both the top-level and the nested subagent input tokens count.
	if r.Total.Input != 2_000_000 {
		t.Errorf("Total.Input = %d, want 2000000 (incl. subagent)", r.Total.Input)
	}
	// But only the top-level turn gets a per-turn cost entry.
	if len(r.PerTurnCost) != 1 {
		t.Errorf("PerTurnCost entries = %d, want 1 (top-level only)", len(r.PerTurnCost))
	}
}

func TestComputeNoUsage(t *testing.T) {
	s := model.Session{Turns: []model.Turn{{Kind: model.TurnUser}, {Kind: model.TurnAssistant}}}
	r := Compute(s)
	if r.HasAnyUsage {
		t.Error("HasAnyUsage should be false when no turn has usage")
	}
	if r.TotalCostUSD != nil {
		t.Error("TotalCostUSD should be nil with no usage")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/usage/ -run TestCompute -v`
Expected: FAIL — `Compute`, `Report`, etc. undefined.

- [ ] **Step 3: Implement usage.go**

Create `internal/usage/usage.go`:

```go
package usage

import (
	"sort"

	"github.com/saigyo/cc-what-have-i-done/internal/model"
)

// TokenCounts is a bucket of token totals by billing category.
type TokenCounts struct {
	Input        int
	Output       int
	CacheRead    int
	CacheWrite5m int
	CacheWrite1h int
}

// InOut is the base input+output total, excluding cache tokens. Used for the
// report's headline figure so cache-read volume does not dominate it.
func (t TokenCounts) InOut() int { return t.Input + t.Output }

func (t *TokenCounts) add(u model.Usage) {
	t.Input += u.Input
	t.Output += u.Output
	t.CacheRead += u.CacheRead
	t.CacheWrite5m += u.CacheWrite5m
	t.CacheWrite1h += u.CacheWrite1h
}

// ModelUsage is the token total and cost for one model in a session.
type ModelUsage struct {
	Model   string
	Tokens  TokenCounts
	CostUSD *float64 // nil when the model is unpriced
}

// Report is the computed usage for a session.
type Report struct {
	Total           TokenCounts
	TotalCostUSD    *float64        // nil only when nothing was priced
	ByModel         []ModelUsage    // sorted, highest in+out first
	PerTurnCost     map[int]*float64 // top-level turn index -> cost (nil = unpriced)
	PricesAsOf      string
	HasUnknownModel bool // some tokens came from an unpriced model
	HasAnyUsage     bool // at least one turn carried usage
}

// cost returns the USD cost of a token bucket at a model's price.
func cost(t TokenCounts, p Price) float64 {
	inRate := p.Input / 1e6
	return float64(t.Input)*inRate +
		float64(t.Output)*(p.Output/1e6) +
		float64(t.CacheWrite5m)*inRate*cacheWrite5mMult +
		float64(t.CacheWrite1h)*inRate*cacheWrite1hMult +
		float64(t.CacheRead)*inRate*cacheReadMult
}

// Compute aggregates token usage and estimated cost for a session. Totals and
// per-model figures include nested subagent turns; per-turn costs are recorded
// for top-level turns only.
func Compute(s model.Session) Report {
	r := Report{PricesAsOf: PricesAsOf, PerTurnCost: map[int]*float64{}}
	byModel := map[string]*TokenCounts{}
	var order []string

	accumulate := func(t model.Turn) {
		if t.Usage == nil {
			return
		}
		r.HasAnyUsage = true
		r.Total.add(*t.Usage)
		tc, ok := byModel[t.Model]
		if !ok {
			tc = &TokenCounts{}
			byModel[t.Model] = tc
			order = append(order, t.Model)
		}
		tc.add(*t.Usage)
	}

	// Totals + per-model: walk top-level and nested subagent turns.
	for _, t := range s.Turns {
		walk(t, accumulate)
	}

	// Per-turn cost for top-level turns only.
	for i, t := range s.Turns {
		if t.Usage == nil {
			continue
		}
		if p, ok := Lookup(t.Model); ok {
			c := cost(counts(*t.Usage), p)
			r.PerTurnCost[i] = &c
		} else {
			r.PerTurnCost[i] = nil
		}
	}

	// Per-model rows + grand total cost.
	var total float64
	priced := false
	for _, m := range order {
		tc := *byModel[m]
		mu := ModelUsage{Model: m, Tokens: tc}
		if p, ok := Lookup(m); ok {
			c := cost(tc, p)
			mu.CostUSD = &c
			total += c
			priced = true
		} else {
			r.HasUnknownModel = true
		}
		r.ByModel = append(r.ByModel, mu)
	}
	if priced {
		r.TotalCostUSD = &total
	}
	sort.SliceStable(r.ByModel, func(i, j int) bool {
		return r.ByModel[i].Tokens.InOut() > r.ByModel[j].Tokens.InOut()
	})
	return r
}

// counts converts a model.Usage into a TokenCounts.
func counts(u model.Usage) TokenCounts {
	return TokenCounts{Input: u.Input, Output: u.Output, CacheRead: u.CacheRead,
		CacheWrite5m: u.CacheWrite5m, CacheWrite1h: u.CacheWrite1h}
}

// walk visits a turn and any turns nested in its Task subagents.
func walk(t model.Turn, fn func(model.Turn)) {
	fn(t)
	for _, b := range t.Blocks {
		if b.Tool == nil {
			continue
		}
		for _, sub := range b.Tool.Subagents {
			for _, st := range sub.Turns {
				walk(st, fn)
			}
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/usage/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usage/usage.go internal/usage/usage_test.go
git commit -m "feat(usage): compute token totals and estimated cost"
```

---

### Task 4: Render the usage card and per-turn badges

**Files:**
- Create: `internal/render/format.go`
- Modify: `internal/render/render.go`
- Modify: `internal/render/assets/report.html.tmpl`
- Modify: `internal/render/assets/styles.css`
- Test: `internal/render/format_test.go`
- Test: `internal/render/render_test.go`

**Interfaces:**
- Consumes: `usage.Compute`, `usage.Report`, `usage.ModelUsage` from Task 3; `model.Session` with per-turn `Usage`/`Model` from Task 1.
- Produces: `render.Options.Usage bool` (new field); usage card + per-turn badges in output.

- [ ] **Step 1: Write the failing formatting test**

Create `internal/render/format_test.go`:

```go
package render

import "testing"

func TestFormatTokens(t *testing.T) {
	cases := map[int]string{0: "0", 192: "192", 12300: "12k", 973000: "973k", 1_200_000: "1.2M"}
	for n, want := range cases {
		if got := formatTokens(n); got != want {
			t.Errorf("formatTokens(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestFormatCost(t *testing.T) {
	if got := formatCost(12.4); got != "$12.40" {
		t.Errorf("formatCost(12.4) = %q, want $12.40", got)
	}
	if got := formatCost(0.001); got != "<$0.01" {
		t.Errorf("formatCost(0.001) = %q, want <$0.01", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/render/ -run TestFormat -v`
Expected: FAIL — `formatTokens`/`formatCost` undefined.

- [ ] **Step 3: Implement format.go**

Create `internal/render/format.go`:

```go
package render

import (
	"fmt"
	"strconv"
)

// formatTokens renders a token count compactly: 192, 12k, 973k, 1.2M.
func formatTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.0fk", float64(n)/1e3)
	default:
		return strconv.Itoa(n)
	}
}

// formatCost renders a USD cost as $X.XX, or <$0.01 for tiny non-zero amounts.
func formatCost(c float64) string {
	if c > 0 && c < 0.005 {
		return "<$0.01"
	}
	return fmt.Sprintf("$%.2f", c)
}
```

- [ ] **Step 4: Run formatting test to verify it passes**

Run: `go test ./internal/render/ -run TestFormat -v`
Expected: PASS.

- [ ] **Step 5: Add the render view model and option**

In `internal/render/render.go`:

(a) Add the import for the usage package to the existing import block:

```go
	"github.com/saigyo/cc-what-have-i-done/internal/model"
	"github.com/saigyo/cc-what-have-i-done/internal/usage"
```

(b) Extend `Options`:

```go
// Options configures a render.
type Options struct {
	Title string
	Usage bool // render the token-usage & cost section
}
```

(c) Add `Badge` to `turnView` and `Usage` to `viewData`:

```go
type viewData struct {
	Title     string
	Session   model.Session
	StartedAt string
	TurnCount int
	Prompts   []promptRef
	Turns     []turnView
	Usage     *usageView
}

type turnView struct {
	Index      int
	Kind       string
	RoleLabel  string
	SearchText string
	Body       template.HTML
	Badge      string // per-turn usage badge, e.g. "12k tok · ~$0.18"
}
```

(d) Add the usage view model types (near the other view types):

```go
type usageView struct {
	HasAny   bool
	Headline string       // collapsed one-line summary
	Rows     []usageRow   // token breakdown
	Models   []usageModel // per-model table rows
	Footnote string
}

type usageRow struct {
	Label string
	Value string
}

type usageModel struct {
	Model  string
	Tokens string
	Cost   string // "$1.23" or "n/a"
}
```

(e) Change `Site` to pass `opts` into `buildViewModel`:

```go
	data := buildViewModel(s, title, opts)
```

(f) Update `buildViewModel`'s signature and body. Replace the whole function with:

```go
func buildViewModel(s model.Session, title string, opts Options) viewData {
	d := viewData{
		Title:     title,
		Session:   s,
		TurnCount: len(s.Turns),
	}
	if !s.StartedAt.IsZero() {
		d.StartedAt = s.StartedAt.Format("2006-01-02 15:04")
	}

	var rep usage.Report
	if opts.Usage {
		rep = usage.Compute(s)
		d.Usage = buildUsageView(rep)
	}

	for i, t := range s.Turns {
		plain := turnPlainText(t)
		if t.Kind == model.TurnUser {
			d.Prompts = append(d.Prompts, promptRef{Index: i, Preview: preview(plain, 60)})
		}
		tv := turnView{
			Index:      i,
			Kind:       string(t.Kind),
			RoleLabel:  roleLabel(t.Kind),
			SearchText: strings.ToLower(plain),
			Body:       renderTurnBody(t),
		}
		if opts.Usage && t.Usage != nil {
			tv.Badge = turnBadge(*t.Usage, rep.PerTurnCost[i])
		}
		d.Turns = append(d.Turns, tv)
	}
	return d
}

// turnBadge formats the per-turn usage badge: "12k tok" plus "· ~$0.18" if priced.
func turnBadge(u model.Usage, costUSD *float64) string {
	b := formatTokens(u.Input+u.Output) + " tok"
	if costUSD != nil {
		b += " · ~" + formatCost(*costUSD)
	}
	return b
}

// buildUsageView turns a usage.Report into the template-facing view model.
func buildUsageView(r usage.Report) *usageView {
	v := &usageView{HasAny: r.HasAnyUsage}
	if !r.HasAnyUsage {
		v.Headline = "Usage · no token-usage data"
		return v
	}
	headline := "Usage · " + formatTokens(r.Total.InOut()) + " in+out"
	if r.TotalCostUSD != nil {
		headline += " · ~" + formatCost(*r.TotalCostUSD) + " (est.)"
	}
	v.Headline = headline

	v.Rows = []usageRow{
		{"input", formatTokens(r.Total.Input)},
		{"output", formatTokens(r.Total.Output)},
		{"cache read", formatTokens(r.Total.CacheRead)},
		{"cache write", formatTokens(r.Total.CacheWrite5m + r.Total.CacheWrite1h)},
	}
	for _, m := range r.ByModel {
		row := usageModel{Model: m.Model, Tokens: formatTokens(m.Tokens.InOut()) + " in+out", Cost: "n/a"}
		if m.CostUSD != nil {
			row.Cost = formatCost(*m.CostUSD)
		}
		v.Models = append(v.Models, row)
	}
	foot := "Estimated — Anthropic list prices as of " + r.PricesAsOf + "; excludes server-tool fees."
	if r.HasUnknownModel {
		foot += " Totals exclude unpriced models (shown as n/a)."
	}
	v.Footnote = foot
	return v
}
```

- [ ] **Step 6: Add the template markup**

In `internal/render/assets/report.html.tmpl`, insert the usage card immediately after the closing `</section>` of `session-head` (before `{{ range .Turns }}`):

```html
    </section>
    {{ with .Usage }}
    <details class="usage-card">
      <summary class="usage-summary">{{ .Headline }}</summary>
      <div class="usage-body">
        {{ if .HasAny }}
        <table class="usage-tokens">
          {{ range .Rows }}<tr><td>{{ .Label }}</td><td>{{ .Value }}</td></tr>{{ end }}
        </table>
        <table class="usage-models">
          <tr><th>model</th><th>tokens</th><th>est. cost</th></tr>
          {{ range .Models }}<tr><td>{{ .Model }}</td><td>{{ .Tokens }}</td><td>{{ .Cost }}</td></tr>{{ end }}
        </table>
        {{ end }}
        <p class="usage-note">{{ .Footnote }}</p>
      </div>
    </details>
    {{ end }}
```

Then change the turn header line from:

```html
      <div class="turn-role">{{ .RoleLabel }}</div>
```

to:

```html
      <div class="turn-role">{{ .RoleLabel }}{{ if .Badge }}<span class="usage-badge">{{ .Badge }}</span>{{ end }}</div>
```

Note: in the no-data case the `{{ if .HasAny }}` guard hides both tables, `Footnote` is empty (renders an empty `<p>`), and the collapsed headline "Usage · no token-usage data" conveys the state.

- [ ] **Step 7: Add styles**

Append to `internal/render/assets/styles.css`:

```css
.usage-card { margin: 0 0 1.5rem; border: 1px solid var(--border, #e5e2d8); border-radius: 8px; background: var(--card, #fff); }
.usage-summary { cursor: pointer; padding: 0.6rem 0.9rem; font-weight: 600; }
.usage-body { padding: 0 0.9rem 0.9rem; font-size: 0.9rem; }
.usage-tokens td:last-child, .usage-models td, .usage-models th { text-align: right; padding: 0.1rem 0.6rem; }
.usage-tokens td:first-child, .usage-models td:first-child { text-align: left; }
.usage-models { margin-top: 0.6rem; border-collapse: collapse; }
.usage-note { color: #8a8578; font-size: 0.8rem; margin-top: 0.6rem; }
.usage-badge { margin-left: 0.5rem; color: #8a8578; font-size: 0.78rem; font-weight: 400; }
```

(These reuse the report's existing custom properties where present and fall back to literals, matching the file's established pattern.)

- [ ] **Step 8: Write the render integration test**

Add to `internal/render/render_test.go`:

```go
func TestSiteRendersUsageWhenEnabled(t *testing.T) {
	s := model.Session{
		ID: "x", Title: "T",
		Turns: []model.Turn{
			{Kind: model.TurnUser, Blocks: []model.Block{{Type: model.BlockText, Text: "hi"}}},
			{Kind: model.TurnAssistant, Model: "claude-opus-4-8",
				Usage:  &model.Usage{Input: 1000, Output: 200},
				Blocks: []model.Block{{Type: model.BlockText, Text: "ok"}}},
		},
	}
	dir := t.TempDir()
	if err := Site(s, dir, Options{Usage: true}); err != nil {
		t.Fatal(err)
	}
	html := readIndex(t, dir)
	for _, want := range []string{"usage-card", "usage-badge", "in+out", "prices as of", "claude-opus-4-8"} {
		if !strings.Contains(html, want) {
			t.Errorf("usage output missing %q", want)
		}
	}
}

func TestSiteOmitsUsageByDefault(t *testing.T) {
	s := model.Session{ID: "x", Title: "T", Turns: []model.Turn{
		{Kind: model.TurnAssistant, Model: "claude-opus-4-8", Usage: &model.Usage{Input: 1000, Output: 200},
			Blocks: []model.Block{{Type: model.BlockText, Text: "ok"}}},
	}}
	dir := t.TempDir()
	if err := Site(s, dir, Options{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readIndex(t, dir), "usage-card") {
		t.Error("usage card should be absent without Options.Usage")
	}
}

// readIndex reads the generated index.html.
func readIndex(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
```

The footnote text contains "Anthropic list prices as of 2026-07"; the test matches the lower-cased-insensitive substring "prices as of" — since the template emits "prices as of" verbatim, keep the assertion string exactly "prices as of". If `render_test.go` lacks imports `os`/`path/filepath`, add them.

- [ ] **Step 9: Run tests to verify they pass**

Run: `go test ./internal/render/ -v`
Expected: PASS (new and existing tests).

- [ ] **Step 10: Commit**

```bash
git add internal/render/
git commit -m "feat(render): usage & cost card and per-turn badges"
```

---

### Task 5: CLI flag, TUI toggle, and docs

**Files:**
- Modify: `cmd/ccwhid/main.go`
- Modify: `cmd/ccwhid/run.go`
- Modify: `internal/tui/tui.go`
- Modify: `internal/tui/tui_test.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: `render.Options.Usage` from Task 4.
- Produces: `--usage` flag; `tui.Selection.Usage bool`; option row on the TUI options screen.

- [ ] **Step 1: Add the CLI flag**

In `cmd/ccwhid/main.go`, add to the `options` struct:

```go
	open             bool
	usage            bool
```

Register the flag in `newRootCmd` (after the `--open` flag):

```go
	f.BoolVar(&opts.usage, "usage", false, "include a token-usage & estimated-cost section")
```

- [ ] **Step 2: Thread it into generate**

In `cmd/ccwhid/run.go`, change the `render.Site` call in `generate`:

```go
	if err := render.Site(sess, outDir, render.Options{Title: opts.title, Usage: opts.usage}); err != nil {
		return "", err
	}
```

- [ ] **Step 3: Map the TUI selection**

In `cmd/ccwhid/main.go`, inside `run`, where TUI selection is applied (after `opts.open = sel.Open`), add:

```go
		opts.usage = sel.Usage
```

- [ ] **Step 4: Write the failing TUI test**

Add to `internal/tui/tui_test.go`:

```go
func TestOptionsUsageToggle(t *testing.T) {
	m := newModel(testGroups())
	m = send(m, key("enter"), key("enter")) // open project, select session -> options
	// move to the usage toggle row and toggle it on
	m = send(m, key("down"), key("down"), key("down"))
	if m.optCursor != optUsage {
		t.Fatalf("optCursor = %d, want optUsage (%d)", m.optCursor, optUsage)
	}
	m = send(m, key(" "))
	if !m.sel.Usage {
		t.Fatal("space on usage row should enable Selection.Usage")
	}
}
```

- [ ] **Step 5: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestOptionsUsageToggle -v`
Expected: FAIL — `optUsage` / `Selection.Usage` undefined.

- [ ] **Step 6: Implement the TUI toggle**

In `internal/tui/tui.go`:

(a) Add `Usage` to `Selection`:

```go
type Selection struct {
	Session          discovery.SessionInfo
	IncludeSubagents bool
	Redact           bool
	Open             bool
	Usage            bool
	OutDir           string
	Canceled         bool
}
```

(b) Insert `optUsage` into the option index block, before `optOutDir`:

```go
const (
	optSubagents = iota
	optRedact
	optOpen
	optUsage
	optOutDir
	optGenerate
	optionCount
)
```

(c) In `updateOptions`, add a case in the `" ", "enter"` switch (alongside the other toggles):

```go
		case optUsage:
			m.sel.Usage = !m.sel.Usage
```

(d) In `viewOptions`, add the toggle to the `toggles` slice so it renders and is navigable:

```go
	toggles := []struct {
		label string
		on    bool
	}{
		{"Include subagents", m.sel.IncludeSubagents},
		{"Redact secrets", m.sel.Redact},
		{"Open in browser when done", m.sel.Open},
		{"Include usage & cost", m.sel.Usage},
	}
```

Because the toggles slice is indexed by cursor position, its order must match the `opt*` constants (subagents, redact, open, usage). The output-dir and generate rows already render after the toggles loop, so no further ordering changes are needed.

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/tui/ ./cmd/... -v`
Expected: PASS.

- [ ] **Step 8: Update the README**

In `README.md`, add a row to the flags table (after `--open`):

```markdown
| `--usage` | Include a token-usage & estimated-cost section (default off) |
```

And add a section after "What's included":

```markdown
## Token usage & cost

Pass `--usage` (or tick "Include usage & cost" in the TUI) to add a collapsible
**Usage** card and per-turn cost badges. Token counts come straight from the
transcript's `usage` data and are exact; cost is an **estimate** from a built-in
Anthropic list-price table (dated in the report footnote) — unknown models show
tokens with cost `n/a`, and server-tool fees are not included. Prices are
embedded, so this works fully offline.
```

- [ ] **Step 9: Full verification**

Run:
```bash
gofmt -l . && go vet ./... && go test ./...
```
Expected: no gofmt output, vet clean, all tests pass.

- [ ] **Step 10: Commit**

```bash
git add cmd/ccwhid/main.go cmd/ccwhid/run.go internal/tui/tui.go internal/tui/tui_test.go README.md
git commit -m "feat: --usage flag and TUI toggle for the usage section"
```

---

## Notes for the implementer

- The exact-cost assertion in Task 3 uses round numbers so the arithmetic is checkable by hand (opus-4.8: input $5, output $25 per MTok; cache read 0.1× input; cache write-5m 1.25× input). Do not "adjust" prices to make a test pass — the prices live in `prices.json` and the multipliers in `pricing.go`.
- `<synthetic>` is deliberately absent from `prices.json`; it must resolve as unpriced.
- Keep the offline guarantee: no `http`/CDN/`url()`/`@font-face`/external refs in the CSS you add.
