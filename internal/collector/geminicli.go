package collector

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kawaiipantsu/aiusagemonitor/internal/model"
)

// GeminiCLI tails Google Gemini CLI logs under ~/.gemini/tmp/<hash>/logs.json
// and chat checkpoints. Support is best-effort / experimental: the format
// carries far less usage detail than Claude Code or Codex.
type GeminiCLI struct {
	Dir      string
	Interval time.Duration
	Backfill time.Duration
}

func (g *GeminiCLI) Name() string             { return "gemini-cli" }
func (g *GeminiCLI) Provider() model.Provider { return model.ProviderGoogle }

func (g *GeminiCLI) root() string {
	if g.Dir != "" {
		return expandHome(g.Dir)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".gemini", "tmp")
}

func (g *GeminiCLI) Run(ctx context.Context, out chan<- Emission) error {
	root := g.root()
	seq := map[string]int{}
	handler := func(path string, raw []byte) []Emission {
		// logs.json is a JSON array, so it arrives as one blob rather than
		// line-delimited; handle both a single object and an array element.
		trimmed := strings.TrimSpace(string(raw))
		trimmed = strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]")
		trimmed = strings.TrimSuffix(trimmed, ",")
		if trimmed == "" || trimmed == "[]" {
			return nil
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(trimmed), &rec); err != nil {
			return nil
		}
		um := findMap(rec, "usageMetadata")
		if um == nil {
			um = findMap(rec, "usage_metadata")
		}
		if um == nil {
			return nil
		}
		gi := func(keys ...string) int64 {
			for _, k := range keys {
				if v, ok := um[k]; ok {
					if f, ok := v.(float64); ok {
						return int64(f)
					}
				}
			}
			return 0
		}
		u := model.TokenUsage{
			InputTokens:     gi("promptTokenCount", "prompt_token_count"),
			OutputTokens:    gi("candidatesTokenCount", "candidates_token_count") + gi("thoughtsTokenCount", "thoughts_token_count"),
			CacheReadTokens: gi("cachedContentTokenCount", "cached_content_token_count"),
			Requests:        1,
		}
		if u.Total() == 0 {
			return nil
		}
		sess := filepath.Base(filepath.Dir(path))
		seq[path]++
		mdl, _ := rec["model"].(string)
		ev := &model.Event{
			Provider:     model.ProviderGoogle,
			Source:       g.Name(),
			Time:         time.Now(),
			SessionID:    sess,
			SessionLabel: "gemini",
			Model:        mdl,
			Kind:         "message",
			Dedup:        "gm:" + sess + ":" + itoa(seq[path]),
			Usage:        u,
		}
		return []Emission{{Event: ev, Source: g.Name()}}
	}
	t := newDirTailer([]string{root}, ".json", g.Interval, g.Backfill, handler)
	emit(ctx, out, Emission{Source: g.Name(), Note: "watching " + root + " (experimental)"})
	return t.run(ctx, out, g.Name())
}
