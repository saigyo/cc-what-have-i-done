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

func TestComputeGroupsDatedAndUndatedModelIDs(t *testing.T) {
	s := model.Session{Turns: []model.Turn{
		{Kind: model.TurnAssistant, Model: "claude-haiku-4-5", Usage: u(100, 10, 0, 0, 0)},
		{Kind: model.TurnAssistant, Model: "claude-haiku-4-5-20251001", Usage: u(200, 20, 0, 0, 0)},
	}}
	r := Compute(s)
	if len(r.ByModel) != 1 {
		t.Fatalf("ByModel has %d rows, want 1 (dated+undated grouped)", len(r.ByModel))
	}
	m := r.ByModel[0]
	if m.Model != "claude-haiku-4-5" {
		t.Errorf("grouped model id = %q, want claude-haiku-4-5", m.Model)
	}
	if m.Tokens.Input != 300 || m.Tokens.Output != 30 {
		t.Errorf("grouped tokens = %+v, want input300/output30", m.Tokens)
	}
}

func TestComputeLabelsMissingModelID(t *testing.T) {
	s := model.Session{Turns: []model.Turn{
		{Kind: model.TurnAssistant, Model: "", Usage: u(100, 10, 0, 0, 0)},
	}}
	r := Compute(s)
	if len(r.ByModel) != 1 || r.ByModel[0].Model != "<unknown>" {
		t.Fatalf("missing model id should be labeled <unknown>, got %+v", r.ByModel)
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
