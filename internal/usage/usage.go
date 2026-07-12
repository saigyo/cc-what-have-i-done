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
	TotalCostUSD    *float64         // nil only when nothing was priced
	ByModel         []ModelUsage     // sorted, highest in+out first
	PerTurnCost     map[int]*float64 // top-level turn index -> cost (nil = unpriced)
	PricesAsOf      string
	HasUnknownModel bool        // some tokens came from an unpriced model
	HasAnyUsage     bool        // at least one turn carried usage
	Subagents       TokenCounts // tokens from linked agent sessions
	SubagentsCost   *float64    // nil when nothing in them was priced
	AgentSessions   int         // number of linked agent sessions
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
// per-model figures include nested subagent turns and linked agent sessions
// (s.Agents), tracked separately in the Subagents subtotal; per-turn costs are
// recorded for top-level main-session turns only.
func Compute(s model.Session) Report {
	r := Report{PricesAsOf: PricesAsOf, PerTurnCost: map[int]*float64{}}
	byModel := map[string]*TokenCounts{}
	var order []string

	subByModel := map[string]*TokenCounts{}
	inAgent := false

	accumulate := func(t model.Turn) {
		if t.Usage == nil {
			return
		}
		r.HasAnyUsage = true
		r.Total.add(*t.Usage)
		// Normalize the key so dated and undated ids for the same base model
		// (e.g. claude-haiku-4-5 and claude-haiku-4-5-20251001) group into one
		// row, matching how Lookup resolves prices. A missing model id is
		// labeled so it never renders as a blank row.
		key := stripDateSuffix(t.Model)
		if key == "" {
			key = "<unknown>"
		}
		tc, ok := byModel[key]
		if !ok {
			tc = &TokenCounts{}
			byModel[key] = tc
			order = append(order, key)
		}
		tc.add(*t.Usage)
		if inAgent {
			r.Subagents.add(*t.Usage)
			stc, ok := subByModel[key]
			if !ok {
				stc = &TokenCounts{}
				subByModel[key] = stc
			}
			stc.add(*t.Usage)
		}
	}

	// Totals + per-model: walk top-level and nested subagent turns.
	for _, t := range s.Turns {
		walk(t, accumulate)
	}

	// Linked agent sessions merge into the same totals and per-model rows,
	// tracked separately so the report can show the subagent share.
	r.AgentSessions = len(s.Agents)
	inAgent = true
	for _, a := range s.Agents {
		for _, t := range a.Session.Turns {
			walk(t, accumulate)
		}
	}
	var subTotal float64
	subPriced := false
	for m, tc := range subByModel {
		if p, ok := Lookup(m); ok {
			subTotal += cost(*tc, p)
			subPriced = true
		}
	}
	if subPriced {
		r.SubagentsCost = &subTotal
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
