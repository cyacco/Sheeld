// Package modelcatalog is the single source of truth for the supported LLM
// models and their (estimated) prices. It backs both the model dropdown in
// the dashboard and the cost estimation in analytics. Prices are hand-
// maintained in prices.json — providers do not expose pricing via API — so
// they are best-effort estimates, matched to recorded model names by longest
// prefix (e.g. "gpt-4o-2024-08-06" → "gpt-4o").
package modelcatalog

import (
	_ "embed"
	"encoding/json"
	"sort"
)

//go:embed prices.json
var pricesData []byte

// Model is one catalog entry. Prices are USD per 1M tokens; zero means the
// price is unknown (the model still appears in the catalog for the dropdown).
type Model struct {
	Provider    string  `json:"provider"`
	ID          string  `json:"id"`
	InputPer1M  float64 `json:"input_per_1m"`
	OutputPer1M float64 `json:"output_per_1m"`
}

type catalogFile struct {
	AsOf   string  `json:"as_of"`
	Models []Model `json:"models"`
}

var loaded catalogFile

func init() {
	if err := json.Unmarshal(pricesData, &loaded); err != nil {
		panic("modelcatalog: invalid prices.json: " + err.Error())
	}
	// Longest id first so prefix matching prefers the most specific entry.
	sort.SliceStable(loaded.Models, func(i, j int) bool {
		return len(loaded.Models[i].ID) > len(loaded.Models[j].ID)
	})
}

// AsOf reports the month the prices were last reviewed (e.g. "2026-01").
func AsOf() string { return loaded.AsOf }

// Models returns the catalog, optionally filtered by provider (empty = all),
// ordered by provider then id.
func Models(provider string) []Model {
	out := make([]Model, 0, len(loaded.Models))
	for _, m := range loaded.Models {
		if provider == "" || m.Provider == provider {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// lookup finds the catalog entry for a recorded model name: an exact match if
// present, otherwise the longest catalog id that is a prefix of the name.
func lookup(model string) (Model, bool) {
	if model == "" {
		return Model{}, false
	}
	for _, m := range loaded.Models { // already sorted longest-id first
		if m.ID == model {
			return m, true
		}
	}
	for _, m := range loaded.Models {
		if len(model) >= len(m.ID) && model[:len(m.ID)] == m.ID {
			return m, true
		}
	}
	return Model{}, false
}

// Cost returns the estimated USD cost for the given token counts under the
// model's price, and ok=false when the model is unknown or has no price.
func Cost(model string, promptTokens, completionTokens int64) (float64, bool) {
	m, found := lookup(model)
	if !found || (m.InputPer1M == 0 && m.OutputPer1M == 0) {
		return 0, false
	}
	cost := float64(promptTokens)/1e6*m.InputPer1M + float64(completionTokens)/1e6*m.OutputPer1M
	return cost, true
}
