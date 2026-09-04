package collector

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/kawaiipantsu/aiusagemonitor/internal/model"
)

// ClaudeCode tails Claude Code session transcripts under ~/.claude/projects.
// Each assistant line carries message.usage with the four token counters.
type ClaudeCode struct {
	Dir      string
	Interval time.Duration
	Backfill time.Duration
}

func (c *ClaudeCode) Name() string             { return "claude-code" }
func (c *ClaudeCode) Provider() model.Provider { return model.ProviderAnthropic }

func (c *ClaudeCode) root() string {
	if c.Dir != "" {
		return expandHome(c.Dir)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects")
}

// claudeLine is the subset of a transcript record we care about.
type claudeLine struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	SessionID string `json:"sessionId"`
	Cwd       string `json:"cwd"`
	UUID      string `json:"uuid"`
	RequestID string `json:"requestId"`
	Message   struct {
		Model string `json:"model"`
		Role  string `json:"role"`
		Usage struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

func (c *ClaudeCode) Run(ctx context.Context, out chan<- Emission) error {
	root := c.root()
	handler := func(path string, raw []byte) []Emission {
		var l claudeLine
		if err := json.Unmarshal(raw, &l); err != nil {
			return nil
		}
		u := l.Message.Usage
		if l.Type != "assistant" || (u.InputTokens == 0 && u.OutputTokens == 0 &&
			u.CacheReadInputTokens == 0 && u.CacheCreationInputTokens == 0) {
			return nil
		}
		ts := parseTime(l.Timestamp)
		label := projectLabel(l.Cwd, path, root)
		dedup := l.RequestID
		if dedup == "" {
			dedup = l.UUID
		}
		ev := &model.Event{
			Provider:     model.ProviderAnthropic,
			Source:       c.Name(),
			Time:         ts,
			SessionID:    l.SessionID,
			SessionLabel: label,
			Model:        l.Message.Model,
			Kind:         "message",
			Dedup:        "cc:" + l.SessionID + ":" + dedup,
			Usage: model.TokenUsage{
				InputTokens:      u.InputTokens,
				OutputTokens:     u.OutputTokens,
				CacheReadTokens:  u.CacheReadInputTokens,
				CacheWriteTokens: u.CacheCreationInputTokens,
				Requests:         1,
			},
		}
		return []Emission{{Event: ev, Source: c.Name()}}
	}
	t := newDirTailer([]string{root}, ".jsonl", c.Interval, c.Backfill, handler)
	emit(ctx, out, Emission{Source: c.Name(), Note: "watching " + root})
	return t.run(ctx, out, c.Name())
}

// projectLabel derives a friendly session label from the recorded cwd, or from
// the transcript directory name (Claude Code encodes the project path there
// with slashes replaced by dashes).
func projectLabel(cwd, path, root string) string {
	if cwd != "" {
		return filepath.Base(cwd)
	}
	rel, err := filepath.Rel(root, filepath.Dir(path))
	if err != nil || rel == "." {
		return "session"
	}
	// "-Users-me-code-foo" -> "foo"
	base := filepath.Base(rel)
	for i := len(base) - 1; i >= 0; i-- {
		if base[i] == '-' {
			return base[i+1:]
		}
	}
	return base
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Now()
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Now()
}
