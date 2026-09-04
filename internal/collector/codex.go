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

// Codex tails OpenAI Codex CLI rollout transcripts under
// ~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl.
//
// The rollout format is still evolving, so parsing is deliberately tolerant:
// we look for a token-count payload and prefer the per-turn delta
// (info.last_token_usage) over cumulative totals.
type Codex struct {
	Dir      string
	Interval time.Duration
	Backfill time.Duration
}

func (c *Codex) Name() string             { return "codex-cli" }
func (c *Codex) Provider() model.Provider { return model.ProviderOpenAI }

func (c *Codex) root() string {
	if c.Dir != "" {
		return expandHome(c.Dir)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex", "sessions")
}

type codexTokens struct {
	InputTokens         int64 `json:"input_tokens"`
	OutputTokens        int64 `json:"output_tokens"`
	CachedInputTokens   int64 `json:"cached_input_tokens"`
	CacheCreationTokens int64 `json:"cache_creation_tokens"`
	ReasoningTokens     int64 `json:"reasoning_output_tokens"`
	TotalTokens         int64 `json:"total_tokens"`
}

func (t codexTokens) usage() model.TokenUsage {
	return model.TokenUsage{
		InputTokens:      t.InputTokens,
		OutputTokens:     t.OutputTokens + t.ReasoningTokens,
		CacheReadTokens:  t.CachedInputTokens,
		CacheWriteTokens: t.CacheCreationTokens,
		Requests:         1,
	}
}
func (t codexTokens) empty() bool {
	return t.InputTokens == 0 && t.OutputTokens == 0 && t.CachedInputTokens == 0 &&
		t.ReasoningTokens == 0 && t.TotalTokens == 0
}

// codexRecord captures the fields we may find across format versions.
type codexRecord struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
	// session_meta variants
	ID    string `json:"id"`
	Model string `json:"model"`
	Cwd   string `json:"cwd"`
}

func (c *Codex) Run(ctx context.Context, out chan<- Emission) error {
	root := c.root()
	// per-file rolling context so events can be attributed to a session/model
	type fileCtx struct {
		session string
		model   string
		label   string
		seq     int
	}
	fctx := map[string]*fileCtx{}

	handler := func(path string, raw []byte) []Emission {
		fc := fctx[path]
		if fc == nil {
			fc = &fileCtx{session: sessionIDFromPath(path)}
			fctx[path] = fc
		}
		var rec codexRecord
		if err := json.Unmarshal(raw, &rec); err != nil {
			return nil
		}
		// Learn session metadata.
		switch rec.Type {
		case "session_meta", "session.created", "turn_context", "turn.context":
			meta := struct {
				ID      string `json:"id"`
				Model   string `json:"model"`
				Cwd     string `json:"cwd"`
				Payload struct {
					ID    string `json:"id"`
					Model string `json:"model"`
					Cwd   string `json:"cwd"`
				} `json:"payload"`
			}{}
			_ = json.Unmarshal(raw, &meta)
			if id := firstNonEmpty(meta.ID, meta.Payload.ID, rec.ID); id != "" {
				fc.session = id
			}
			if m := firstNonEmpty(meta.Model, meta.Payload.Model, rec.Model); m != "" {
				fc.model = m
			}
			if cwd := firstNonEmpty(meta.Cwd, meta.Payload.Cwd, rec.Cwd); cwd != "" {
				fc.label = filepath.Base(cwd)
			}
			return nil
		}

		tok, ok := extractCodexTokens(raw)
		if !ok || tok.empty() {
			return nil
		}
		fc.seq++
		ts := parseTime(rec.Timestamp)
		label := fc.label
		if label == "" {
			label = "codex"
		}
		ev := &model.Event{
			Provider:     model.ProviderOpenAI,
			Source:       c.Name(),
			Time:         ts,
			SessionID:    fc.session,
			SessionLabel: label,
			Model:        fc.model,
			Kind:         "message",
			Dedup:        "cx:" + fc.session + ":" + itoa(fc.seq),
			Usage:        tok.usage(),
		}
		return []Emission{{Event: ev, Source: c.Name()}}
	}

	t := newDirTailer([]string{root}, ".jsonl", c.Interval, c.Backfill, handler)
	emit(ctx, out, Emission{Source: c.Name(), Note: "watching " + root})
	return t.run(ctx, out, c.Name())
}

// extractCodexTokens hunts through a record for a last_token_usage delta,
// falling back to a bare token_usage / usage object.
func extractCodexTokens(raw []byte) (codexTokens, bool) {
	// Fast reject.
	if !strings.Contains(string(raw), "token") && !strings.Contains(string(raw), "usage") {
		return codexTokens{}, false
	}
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return codexTokens{}, false
	}
	if info := findMap(probe, "info"); info != nil {
		if last := findMap(info, "last_token_usage"); last != nil {
			return mapToTokens(last), true
		}
		if tu := findMap(info, "token_usage"); tu != nil {
			return mapToTokens(tu), true
		}
	}
	for _, key := range []string{"last_token_usage", "token_usage", "usage"} {
		if m := findMap(probe, key); m != nil {
			return mapToTokens(m), true
		}
	}
	return codexTokens{}, false
}

func mapToTokens(m map[string]any) codexTokens {
	gi := func(keys ...string) int64 {
		for _, k := range keys {
			if v, ok := m[k]; ok {
				switch n := v.(type) {
				case float64:
					return int64(n)
				case int64:
					return n
				}
			}
		}
		return 0
	}
	return codexTokens{
		InputTokens:         gi("input_tokens", "prompt_tokens"),
		OutputTokens:        gi("output_tokens", "completion_tokens"),
		CachedInputTokens:   gi("cached_input_tokens", "cache_read_input_tokens", "cached_tokens"),
		CacheCreationTokens: gi("cache_creation_tokens", "cache_creation_input_tokens"),
		ReasoningTokens:     gi("reasoning_output_tokens", "reasoning_tokens"),
		TotalTokens:         gi("total_tokens"),
	}
}

// findMap does a shallow-then-deep search for a nested object by key.
func findMap(m map[string]any, key string) map[string]any {
	if v, ok := m[key]; ok {
		if mm, ok := v.(map[string]any); ok {
			return mm
		}
	}
	for _, v := range m {
		if mm, ok := v.(map[string]any); ok {
			if got := findMap(mm, key); got != nil {
				return got
			}
		}
	}
	return nil
}

func sessionIDFromPath(p string) string {
	base := filepath.Base(p)
	base = strings.TrimPrefix(base, "rollout-")
	base = strings.TrimSuffix(base, ".jsonl")
	if base == "" {
		return p
	}
	return base
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
