package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kawaiipantsu/aiusagemonitor/internal/model"
)

// Poll periodically queries a vendor usage / admin API. It emits synthetic
// Events for the delta since the previous poll so the aggregation engine can
// treat polled data and live data uniformly.
type Poll struct {
	Prov     model.Provider
	Interval time.Duration
	APIKey   string // standard key
	AdminKey string // org/admin key (required by OpenAI + Anthropic usage APIs)
	BaseURL  string
	Org      string

	client *http.Client
	seen   map[string]int64 // bucket-id -> total tokens already emitted
}

func (p *Poll) Name() string             { return "poll-" + string(p.Prov) }
func (p *Poll) Provider() model.Provider { return p.Prov }

func (p *Poll) Run(ctx context.Context, out chan<- Emission) error {
	if p.Interval < time.Minute {
		p.Interval = 5 * time.Minute
	}
	p.client = &http.Client{Timeout: 30 * time.Second}
	p.seen = map[string]int64{}

	emit(ctx, out, Emission{Source: p.Name(), Note: "polling every " + p.Interval.String()})
	// Immediate first poll, then on the interval.
	p.once(ctx, out)
	t := time.NewTicker(p.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			p.once(ctx, out)
		}
	}
}

func (p *Poll) once(ctx context.Context, out chan<- Emission) {
	var err error
	switch p.Prov {
	case model.ProviderOpenAI:
		err = p.pollOpenAI(ctx, out)
	case model.ProviderAnthropic:
		err = p.pollAnthropic(ctx, out)
	case model.ProviderXAI:
		err = p.pollXAI(ctx, out)
	case model.ProviderGoogle:
		err = fmt.Errorf("google usage polling is not available; use the proxy or Gemini CLI collector")
	}
	if err != nil {
		emit(ctx, out, Emission{Source: p.Name(), Err: err})
	}
}

func (p *Poll) get(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(b))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return nil, fmt.Errorf("%s -> HTTP %d: %s", shortURL(url), resp.StatusCode, msg)
	}
	return b, nil
}

func shortURL(u string) string {
	if i := strings.Index(u, "?"); i >= 0 {
		return u[:i]
	}
	return u
}

// emitBucket turns a cumulative bucket total into a delta Event.
func (p *Poll) emitBucket(ctx context.Context, out chan<- Emission, id string, start time.Time, mdl string, u model.TokenUsage) {
	total := u.Total()
	prev := p.seen[id]
	if total <= prev {
		return
	}
	// Scale the bundle down to just the new portion (proportional split).
	if prev > 0 && total > 0 {
		ratio := float64(total-prev) / float64(total)
		u = model.TokenUsage{
			InputTokens:      int64(float64(u.InputTokens) * ratio),
			OutputTokens:     int64(float64(u.OutputTokens) * ratio),
			CacheReadTokens:  int64(float64(u.CacheReadTokens) * ratio),
			CacheWriteTokens: int64(float64(u.CacheWriteTokens) * ratio),
			Requests:         u.Requests,
		}
	}
	p.seen[id] = total
	emit(ctx, out, Emission{Source: p.Name(), Event: &model.Event{
		Provider:     p.Prov,
		Source:       p.Name(),
		Time:         start,
		SessionID:    "poll:" + p.Prov.Title() + ":" + start.Format("2006-01-02"),
		SessionLabel: "usage-api",
		Model:        mdl,
		Kind:         "poll",
		Dedup:        "poll:" + id,
		Usage:        u,
	}})
}

// ---- OpenAI -----------------------------------------------------------------

func (p *Poll) pollOpenAI(ctx context.Context, out chan<- Emission) error {
	key := firstNonEmpty(p.AdminKey, p.APIKey)
	if key == "" {
		return fmt.Errorf("OpenAI polling needs an admin key (Settings ▸ OpenAI ▸ admin_key)")
	}
	base := strings.TrimRight(firstNonEmpty(p.BaseURL, "https://api.openai.com"), "/")
	start := time.Now().Add(-24 * time.Hour).Truncate(time.Hour)
	url := fmt.Sprintf("%s/v1/organization/usage/completions?start_time=%d&bucket_width=1h&group_by=model&limit=180",
		base, start.Unix())
	b, err := p.get(ctx, url, map[string]string{
		"Authorization":       "Bearer " + key,
		"OpenAI-Organization": p.Org,
	})
	if err != nil {
		return err
	}
	var resp struct {
		Data []struct {
			StartTime int64 `json:"start_time"`
			Results   []struct {
				InputTokens       int64  `json:"input_tokens"`
				OutputTokens      int64  `json:"output_tokens"`
				InputCachedTokens int64  `json:"input_cached_tokens"`
				NumModelRequests  int64  `json:"num_model_requests"`
				Model             string `json:"model"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return fmt.Errorf("decode openai usage: %w", err)
	}
	for _, bucket := range resp.Data {
		bt := time.Unix(bucket.StartTime, 0)
		for i, r := range bucket.Results {
			id := fmt.Sprintf("oai:%d:%s:%d", bucket.StartTime, r.Model, i)
			p.emitBucket(ctx, out, id, bt, r.Model, model.TokenUsage{
				InputTokens:     r.InputTokens,
				OutputTokens:    r.OutputTokens,
				CacheReadTokens: r.InputCachedTokens,
				Requests:        r.NumModelRequests,
			})
		}
	}
	return nil
}

// ---- Anthropic ------------------------------------------------------------

func (p *Poll) pollAnthropic(ctx context.Context, out chan<- Emission) error {
	key := firstNonEmpty(p.AdminKey, p.APIKey)
	if key == "" {
		return fmt.Errorf("claude polling needs an admin key (Settings ▸ Claude ▸ admin_key)")
	}
	base := strings.TrimRight(firstNonEmpty(p.BaseURL, "https://api.anthropic.com"), "/")
	start := time.Now().Add(-24 * time.Hour).Truncate(time.Hour)
	url := fmt.Sprintf("%s/v1/organization/usage_report/messages?starting_at=%s&bucket_width=1h&limit=168",
		base, start.UTC().Format(time.RFC3339))
	b, err := p.get(ctx, url, map[string]string{
		"x-api-key":         key,
		"anthropic-version": "2023-06-01",
	})
	if err != nil {
		return err
	}
	var resp struct {
		Data []struct {
			StartingAt string `json:"starting_at"`
			Results    []struct {
				UncachedInputTokens      int64  `json:"uncached_input_tokens"`
				OutputTokens             int64  `json:"output_tokens"`
				CacheReadInputTokens     int64  `json:"cache_read_input_tokens"`
				CacheCreationInputTokens int64  `json:"cache_creation_input_tokens"`
				Model                    string `json:"model"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return fmt.Errorf("decode anthropic usage: %w", err)
	}
	for _, bucket := range resp.Data {
		bt := parseTime(bucket.StartingAt)
		for i, r := range bucket.Results {
			id := fmt.Sprintf("ant:%s:%s:%d", bucket.StartingAt, r.Model, i)
			p.emitBucket(ctx, out, id, bt, r.Model, model.TokenUsage{
				InputTokens:      r.UncachedInputTokens,
				OutputTokens:     r.OutputTokens,
				CacheReadTokens:  r.CacheReadInputTokens,
				CacheWriteTokens: r.CacheCreationInputTokens,
				Requests:         1,
			})
		}
	}
	return nil
}

// ---- xAI ---------------------------------------------------------------------

func (p *Poll) pollXAI(ctx context.Context, out chan<- Emission) error {
	key := firstNonEmpty(p.APIKey, p.AdminKey)
	if key == "" {
		return fmt.Errorf("xAI polling needs an API key")
	}
	base := strings.TrimRight(firstNonEmpty(p.BaseURL, "https://api.x.ai"), "/")
	// xAI exposes key metadata but no historical usage endpoint yet; surface
	// the key status so the user at least sees the connection is live.
	b, err := p.get(ctx, base+"/v1/api-key", map[string]string{"Authorization": "Bearer " + key})
	if err != nil {
		return err
	}
	var meta struct {
		RedactedAPIKey string   `json:"redacted_api_key"`
		Name           string   `json:"name"`
		ACLs           []string `json:"acls"`
		APIKeyBlocked  bool     `json:"api_key_blocked"`
	}
	_ = json.Unmarshal(b, &meta)
	status := "key ok"
	if meta.RedactedAPIKey != "" {
		status = "key " + meta.RedactedAPIKey
	}
	if meta.APIKeyBlocked {
		status += " (blocked)"
	}
	emit(ctx, out, Emission{Source: p.Name(), Note: "xAI " + status + "; no historical usage API — use the proxy for live stats"})
	return nil
}
