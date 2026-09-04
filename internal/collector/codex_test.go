package collector

import "testing"

func TestExtractCodexTokensLastTokenUsage(t *testing.T) {
	raw := []byte(`{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":50,"cached_input_tokens":10,"output_tokens":25,"reasoning_output_tokens":5},"total_token_usage":{"input_tokens":5000,"output_tokens":900}}}}`)
	tok, ok := extractCodexTokens(raw)
	if !ok {
		t.Fatalf("expected a token match")
	}
	// Must prefer the per-turn delta (last_token_usage) over the cumulative total.
	if tok.InputTokens != 50 || tok.OutputTokens != 25 || tok.CachedInputTokens != 10 || tok.ReasoningTokens != 5 {
		t.Fatalf("got %+v, want the last_token_usage delta, not the cumulative total", tok)
	}
	u := tok.usage()
	if u.InputTokens != 50 || u.OutputTokens != 30 { // output + reasoning
		t.Fatalf("usage() = %+v", u)
	}
}

func TestExtractCodexTokensFallbackUsage(t *testing.T) {
	raw := []byte(`{"type":"response.completed","usage":{"prompt_tokens":12,"completion_tokens":8}}`)
	tok, ok := extractCodexTokens(raw)
	if !ok {
		t.Fatalf("expected a token match via fallback usage object")
	}
	if tok.InputTokens != 12 || tok.OutputTokens != 8 {
		t.Fatalf("got %+v", tok)
	}
}

func TestExtractCodexTokensNoMatch(t *testing.T) {
	raw := []byte(`{"type":"session_meta","id":"abc","model":"gpt-5"}`)
	if _, ok := extractCodexTokens(raw); ok {
		t.Fatalf("session_meta carries no usage and should not match")
	}
}

func TestSessionIDFromPath(t *testing.T) {
	cases := map[string]string{
		"/x/y/rollout-abc-123.jsonl": "abc-123",
		"/x/rollout-.jsonl":          "/x/rollout-.jsonl", // degenerate: falls back to the full path
		"noext":                      "noext",
	}
	for in, want := range cases {
		if got := sessionIDFromPath(in); got != want {
			t.Errorf("sessionIDFromPath(%q) = %q, want %q", in, got, want)
		}
	}
}
