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
