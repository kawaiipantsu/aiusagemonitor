package collector

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kawaiipantsu/aiusagemonitor/internal/model"
)

const sampleClaudeLine = `{"type":"assistant","timestamp":"2026-09-04T12:00:00.000Z","sessionId":"abc123","cwd":"/home/me/code/webapp","uuid":"u-1","requestId":"req-1","message":{"model":"claude-sonnet-5","role":"assistant","usage":{"input_tokens":120,"output_tokens":340,"cache_creation_input_tokens":10,"cache_read_input_tokens":5000}}}`

func TestClaudeCodeParsesTranscriptLine(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "-home-me-code-webapp")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessDir, "abc123.jsonl"), []byte(sampleClaudeLine+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &ClaudeCode{Dir: dir, Interval: 10 * time.Millisecond, Backfill: time.Hour}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out := make(chan Emission, 16)
	go func() { _ = c.Run(ctx, out) }()

	var got *model.Event
	for got == nil {
		select {
		case em := <-out:
			if em.Event != nil {
				got = em.Event
			}
		case <-ctx.Done():
			t.Fatal("timed out waiting for an event")
		}
	}

	if got.Provider != model.ProviderAnthropic {
		t.Errorf("Provider = %v, want anthropic", got.Provider)
	}
	if got.Model != "claude-sonnet-5" {
		t.Errorf("Model = %q, want claude-sonnet-5", got.Model)
	}
	if got.SessionID != "abc123" {
		t.Errorf("SessionID = %q, want abc123", got.SessionID)
	}
	if got.SessionLabel != "webapp" {
		t.Errorf("SessionLabel = %q, want webapp (basename of cwd)", got.SessionLabel)
	}
	want := model.TokenUsage{InputTokens: 120, OutputTokens: 340, CacheReadTokens: 5000, CacheWriteTokens: 10, Requests: 1}
	if got.Usage != want {
		t.Errorf("Usage = %+v, want %+v", got.Usage, want)
	}
	if got.Dedup == "" {
		t.Errorf("Dedup should be set so re-reading the file can't double-count")
	}
}

func TestClaudeCodeIgnoresNonAssistantLines(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "s.jsonl"), []byte(`{"type":"user","timestamp":"2026-09-04T12:00:00.000Z"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &ClaudeCode{Dir: dir, Interval: 10 * time.Millisecond, Backfill: time.Hour}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	out := make(chan Emission, 16)
	go func() { _ = c.Run(ctx, out) }()
	for {
		select {
		case em := <-out:
			if em.Event != nil {
				t.Fatalf("a user-role line without usage should never produce an event: %+v", em.Event)
			}
		case <-ctx.Done():
			return
		}
	}
}
