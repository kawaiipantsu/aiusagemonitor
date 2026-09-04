// Package pricing converts token counts into approximate USD cost.
//
// The numbers here are best-effort public list prices and WILL drift. Users can
// override any model price from the config file (see config.Pricing).
package pricing

import (
	"sort"
	"strings"
	"sync"

	"github.com/kawaiipantsu/aiusagemonitor/internal/model"
)

// Rate is USD per 1,000,000 tokens for each accounting bucket.
type Rate struct {
	Input      float64 `yaml:"input" json:"input"`
	Output     float64 `yaml:"output" json:"output"`
	CacheRead  float64 `yaml:"cache_read" json:"cache_read"`
	CacheWrite float64 `yaml:"cache_write" json:"cache_write"`
}

// Cost applies the rate to a usage bundle.
func (r Rate) Cost(u model.TokenUsage) float64 {
	const perM = 1_000_000.0
	return float64(u.InputTokens)/perM*r.Input +
		float64(u.OutputTokens)/perM*r.Output +
		float64(u.CacheReadTokens)/perM*r.CacheRead +
		float64(u.CacheWriteTokens)/perM*r.CacheWrite
}

// defaults maps a model-id prefix to a rate. Longest prefix wins.
var defaults = map[string]Rate{
	// OpenAI
	"gpt-4o-mini":    {Input: 0.15, Output: 0.60, CacheRead: 0.075},
	"gpt-4o":         {Input: 2.50, Output: 10.00, CacheRead: 1.25},
	"gpt-4.1-mini":   {Input: 0.40, Output: 1.60, CacheRead: 0.10},
	"gpt-4.1-nano":   {Input: 0.10, Output: 0.40, CacheRead: 0.025},
	"gpt-4.1":        {Input: 2.00, Output: 8.00, CacheRead: 0.50},
	"gpt-5-mini":     {Input: 0.25, Output: 2.00, CacheRead: 0.025},
	"gpt-5":          {Input: 1.25, Output: 10.00, CacheRead: 0.125},
	"o4-mini":        {Input: 1.10, Output: 4.40, CacheRead: 0.275},
	"o3-mini":        {Input: 1.10, Output: 4.40, CacheRead: 0.55},
	"o3":             {Input: 2.00, Output: 8.00, CacheRead: 0.50},
	"o1-mini":        {Input: 1.10, Output: 4.40},
	"o1":             {Input: 15.00, Output: 60.00},
	"text-embedding": {Input: 0.02},

	// Anthropic (Claude)
	"claude-3-haiku":    {Input: 0.25, Output: 1.25, CacheRead: 0.03, CacheWrite: 0.30},
	"claude-3-5-haiku":  {Input: 0.80, Output: 4.00, CacheRead: 0.08, CacheWrite: 1.00},
	"claude-3-5-sonnet": {Input: 3.00, Output: 15.00, CacheRead: 0.30, CacheWrite: 3.75},
	"claude-3-7-sonnet": {Input: 3.00, Output: 15.00, CacheRead: 0.30, CacheWrite: 3.75},
	"claude-haiku-4":    {Input: 1.00, Output: 5.00, CacheRead: 0.10, CacheWrite: 1.25},
	"claude-sonnet-4":   {Input: 3.00, Output: 15.00, CacheRead: 0.30, CacheWrite: 3.75},
	"claude-opus-4":     {Input: 15.00, Output: 75.00, CacheRead: 1.50, CacheWrite: 18.75},
	"claude-3-opus":     {Input: 15.00, Output: 75.00, CacheRead: 1.50, CacheWrite: 18.75},
	"claude-sonnet-5":   {Input: 3.00, Output: 15.00, CacheRead: 0.30, CacheWrite: 3.75},
	"claude-opus-5":     {Input: 15.00, Output: 75.00, CacheRead: 1.50, CacheWrite: 18.75},

	// Google (Gemini)
	"gemini-1.5-flash-8b": {Input: 0.0375, Output: 0.15},
	"gemini-1.5-flash":    {Input: 0.075, Output: 0.30},
	"gemini-1.5-pro":      {Input: 1.25, Output: 5.00},
	"gemini-2.0-flash":    {Input: 0.10, Output: 0.40, CacheRead: 0.025},
	"gemini-2.5-flash":    {Input: 0.30, Output: 2.50, CacheRead: 0.075},
	"gemini-2.5-pro":      {Input: 1.25, Output: 10.00, CacheRead: 0.31},

	// xAI (Grok)
	"grok-code-fast": {Input: 0.20, Output: 1.50, CacheRead: 0.02},
	"grok-3-mini":    {Input: 0.30, Output: 0.50},
	"grok-3":         {Input: 3.00, Output: 15.00},
	"grok-4-fast":    {Input: 0.20, Output: 0.50},
	"grok-4":         {Input: 3.00, Output: 15.00, CacheRead: 0.75},
	"grok-beta":      {Input: 5.00, Output: 15.00},
}

// Table is a resolver with optional user overrides.
type Table struct {
	mu        sync.RWMutex
	overrides map[string]Rate
	keys      []string // sorted longest-first, defaults + overrides
}

// NewTable builds a resolver. overrides may be nil.
func NewTable(overrides map[string]Rate) *Table {
	t := &Table{overrides: map[string]Rate{}}
	for k, v := range overrides {
		t.overrides[strings.ToLower(k)] = v
	}
	t.rebuild()
	return t
}

func (t *Table) rebuild() {
	seen := map[string]struct{}{}
	var keys []string
	for k := range defaults {
		keys = append(keys, k)
		seen[k] = struct{}{}
	}
	for k := range t.overrides {
		if _, ok := seen[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j]
	})
	t.keys = keys
}

// SetOverride adds or replaces a price at runtime (used by the settings UI).
func (t *Table) SetOverride(modelID string, r Rate) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.overrides[strings.ToLower(modelID)] = r
	t.rebuild()
}

// Rate resolves the best matching rate for a model id. ok is false when no
// entry matched (cost is then assumed zero).
func (t *Table) Rate(modelID string) (Rate, bool) {
	id := strings.ToLower(strings.TrimSpace(modelID))
	if id == "" {
		return Rate{}, false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	if r, ok := t.overrides[id]; ok {
		return r, true
	}
	for _, k := range t.keys {
		if strings.HasPrefix(id, k) || strings.Contains(id, k) {
			if r, ok := t.overrides[k]; ok {
				return r, true
			}
			return defaults[k], true
		}
	}
	return Rate{}, false
}

// Cost is a convenience wrapper: resolve + apply.
func (t *Table) Cost(modelID string, u model.TokenUsage) float64 {
	r, ok := t.Rate(modelID)
	if !ok {
		return 0
	}
	return r.Cost(u)
}

// Defaults returns a copy of the built-in price list (for display / export).
func Defaults() map[string]Rate {
	out := make(map[string]Rate, len(defaults))
	for k, v := range defaults {
		out[k] = v
	}
	return out
}
