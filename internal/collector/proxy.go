package collector

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kawaiipantsu/aiusagemonitor/internal/model"
)

// Proxy is a local reverse proxy that records every AI API request routed
// through it. Point your SDK's base URL at:
//
//	http://<listen>/openai      (OpenAI)
//	http://<listen>/anthropic   (Anthropic / Claude)
//	http://<listen>/google      (Gemini)
//	http://<listen>/xai         (xAI / Grok)
//
// The first path segment selects the upstream; the rest of the path and the
// body are forwarded unchanged, including auth headers.
type Proxy struct {
	Listen   string
	BaseURLs map[model.Provider]string // optional per-provider overrides
	client   *http.Client
}

func (p *Proxy) Name() string             { return "proxy" }
func (p *Proxy) Provider() model.Provider { return "" }

var defaultBase = map[model.Provider]string{
	model.ProviderOpenAI:    "https://api.openai.com",
	model.ProviderAnthropic: "https://api.anthropic.com",
	model.ProviderGoogle:    "https://generativelanguage.googleapis.com",
	model.ProviderXAI:       "https://api.x.ai",
}

func (p *Proxy) base(prov model.Provider) string {
	if p.BaseURLs != nil {
		if b := strings.TrimSpace(p.BaseURLs[prov]); b != "" {
			return strings.TrimRight(b, "/")
		}
	}
	return defaultBase[prov]
}

func (p *Proxy) Run(ctx context.Context, out chan<- Emission) error {
	if p.Listen == "" {
		p.Listen = "127.0.0.1:8317"
	}
	if p.client == nil {
		p.client = &http.Client{
			// No client timeout: streaming responses can run for minutes. The
			// request context governs lifetime instead.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				MaxIdleConns:          32,
				IdleConnTimeout:       90 * time.Second,
				ForceAttemptHTTP2:     true,
				ResponseHeaderTimeout: 60 * time.Second,
				DialContext:           (&net.Dialer{Timeout: 15 * time.Second}).DialContext,
			},
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { p.handle(ctx, out, w, r) })

	srv := &http.Server{Addr: p.Listen, Handler: mux, ReadHeaderTimeout: 20 * time.Second}
	ln, err := net.Listen("tcp", p.Listen)
	if err != nil {
		emit(ctx, out, Emission{Source: p.Name(), Err: err})
		return err
	}
	emit(ctx, out, Emission{Source: p.Name(), Note: "listening on http://" + p.Listen})

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (p *Proxy) handle(ctx context.Context, out chan<- Emission, w http.ResponseWriter, r *http.Request) {
	seg, rest := splitFirstSegment(r.URL.Path)
	prov, ok := model.ParseProvider(seg)
	if !ok {
		http.Error(w, "aiusagemonitor proxy: prefix the path with /openai, /anthropic, /google or /xai", http.StatusBadGateway)
		return
	}
	target, err := url.Parse(p.base(prov))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	target.Path = singleJoin(target.Path, rest)
	target.RawQuery = r.URL.RawQuery

	body, _ := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	_ = r.Body.Close()

	upReq, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), bytes.NewReader(body))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	copyHeaders(upReq.Header, r.Header)
	upReq.Header.Del("Accept-Encoding") // we want to read the body
	upReq.Host = target.Host

	resp, err := p.client.Do(upReq)
	if err != nil {
		http.Error(w, "upstream error: "+err.Error(), http.StatusBadGateway)
		emit(ctx, out, Emission{Source: p.Name(), Err: err})
		return
	}
	defer func() { _ = resp.Body.Close() }()

	// Rate-limit headers are available before the body is read.
	reqModel := modelFromRequest(prov, body, rest)
	for _, l := range parseLimitHeaders(prov, resp.Header, reqModel) {
		ll := l
		emit(ctx, out, Emission{Source: p.Name(), Limit: &ll})
	}

	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	var buf bytes.Buffer
	tee := io.TeeReader(resp.Body, limitedWriter{&buf, 8 << 20})
	if f, ok := w.(http.Flusher); ok {
		_ = streamCopy(w, tee, f)
	} else {
		_, _ = io.Copy(w, tee)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		u, mdl := extractProxyUsage(prov, resp.Header.Get("Content-Type"), buf.Bytes())
		if mdl == "" {
			mdl = reqModel
		}
		if !u.IsZero() {
			u.Requests = 1
			emit(ctx, out, Emission{Source: p.Name(), Event: &model.Event{
				Provider:     prov,
				Source:       p.Name(),
				Time:         time.Now(),
				SessionID:    sessionFromHeaders(r.Header),
				SessionLabel: "proxy",
				Model:        mdl,
				Kind:         "proxy",
				Usage:        u,
			}})
		}
	}
}

// --- helpers -------------------------------------------------------------

type limitedWriter struct {
	w io.Writer
	n int
}

func (l limitedWriter) Write(p []byte) (int, error) {
	if l.n <= 0 {
		return len(p), nil
	}
	if len(p) > l.n {
		p = p[:l.n]
	}
	return l.w.Write(p)
}

func streamCopy(dst io.Writer, src io.Reader, f http.Flusher) error {
	b := make([]byte, 16*1024)
	for {
		n, err := src.Read(b)
		if n > 0 {
			if _, werr := dst.Write(b[:n]); werr != nil {
				return werr
			}
			f.Flush()
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func splitFirstSegment(p string) (seg, rest string) {
	p = strings.TrimPrefix(p, "/")
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i], "/" + p[i+1:]
	}
	return p, "/"
}

func singleJoin(a, b string) string {
	a = strings.TrimRight(a, "/")
	b = strings.TrimLeft(b, "/")
	if a == "" {
		return "/" + b
	}
	return a + "/" + b
}

func copyHeaders(dst, src http.Header) {
	for k, vs := range src {
		switch strings.ToLower(k) {
		case "connection", "proxy-connection", "keep-alive", "transfer-encoding", "upgrade", "content-length":
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

func sessionFromHeaders(h http.Header) string {
	for _, k := range []string{"X-Aium-Session", "X-Session-Id", "X-Request-Session"} {
		if v := h.Get(k); v != "" {
			return v
		}
	}
	return "proxy-" + time.Now().Format("2006-01-02")
}

// modelFromRequest best-effort pulls the model id out of the request body or,
// for Gemini, the URL path (/models/gemini-x:generateContent).
func modelFromRequest(prov model.Provider, body []byte, path string) string {
	if prov == model.ProviderGoogle {
		if i := strings.Index(path, "/models/"); i >= 0 {
			rest := path[i+len("/models/"):]
			if j := strings.IndexAny(rest, ":/"); j >= 0 {
				return rest[:j]
			}
			return rest
		}
	}
	var m struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &m) == nil && m.Model != "" {
		return m.Model
	}
	return ""
}

func parseResetTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	if d, err := time.ParseDuration(s); err == nil {
		return time.Now().Add(d)
	}
	if secs, err := strconv.ParseFloat(strings.TrimSuffix(s, "s"), 64); err == nil {
		return time.Now().Add(time.Duration(secs * float64(time.Second)))
	}
	return time.Time{}
}

func hf(h http.Header, key string) (float64, bool) {
	v := h.Get(key)
	if v == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", ""), 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

func parseLimitHeaders(prov model.Provider, h http.Header, mdl string) []model.Limit {
	now := time.Now()
	var out []model.Limit
	add := func(kind model.LimitKind, limH, remH, resetH, window string) {
		lim, ok1 := hf(h, limH)
		rem, ok2 := hf(h, remH)
		if !ok1 && !ok2 {
			return
		}
		out = append(out, model.Limit{
			Provider: prov, Kind: kind, Model: mdl, Window: window,
			Limit: lim, Remaining: rem, Observed: now,
			ResetAt: parseResetTime(h.Get(resetH)),
		})
	}
	switch prov {
	case model.ProviderAnthropic:
		add(model.LimitRequests, "anthropic-ratelimit-requests-limit", "anthropic-ratelimit-requests-remaining", "anthropic-ratelimit-requests-reset", "1m")
		add(model.LimitTokens, "anthropic-ratelimit-tokens-limit", "anthropic-ratelimit-tokens-remaining", "anthropic-ratelimit-tokens-reset", "1m")
		add(model.LimitInputTokens, "anthropic-ratelimit-input-tokens-limit", "anthropic-ratelimit-input-tokens-remaining", "anthropic-ratelimit-input-tokens-reset", "1m")
		add(model.LimitOutputTokens, "anthropic-ratelimit-output-tokens-limit", "anthropic-ratelimit-output-tokens-remaining", "anthropic-ratelimit-output-tokens-reset", "1m")
	default: // OpenAI, xAI, and OpenAI-compatible gateways
		add(model.LimitRequests, "x-ratelimit-limit-requests", "x-ratelimit-remaining-requests", "x-ratelimit-reset-requests", "")
		add(model.LimitTokens, "x-ratelimit-limit-tokens", "x-ratelimit-remaining-tokens", "x-ratelimit-reset-tokens", "")
	}
	return out
}

// extractProxyUsage pulls a usage bundle + model id out of a completed
// response body (JSON or SSE).
func extractProxyUsage(prov model.Provider, contentType string, body []byte) (model.TokenUsage, string) {
	if len(body) == 0 {
		return model.TokenUsage{}, ""
	}
	if strings.Contains(contentType, "text/event-stream") || bytes.HasPrefix(bytes.TrimSpace(body), []byte("data:")) {
		return extractSSEUsage(prov, body)
	}
	return extractJSONUsage(prov, body)
}

func extractSSEUsage(prov model.Provider, body []byte) (model.TokenUsage, string) {
	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
	var acc model.TokenUsage
	var mdl string
	haveStart := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		u, m := extractJSONUsage(prov, []byte(payload))
		if m != "" {
			mdl = m
		}
		switch prov {
		case model.ProviderAnthropic:
			// message_start carries input+cache; message_delta carries the
			// running output_tokens. Keep max output, first non-zero input.
			if u.InputTokens > 0 && !haveStart {
				acc.InputTokens = u.InputTokens
				acc.CacheReadTokens = u.CacheReadTokens
				acc.CacheWriteTokens = u.CacheWriteTokens
				haveStart = true
			}
			if u.OutputTokens > acc.OutputTokens {
				acc.OutputTokens = u.OutputTokens
			}
		default:
			// OpenAI / Gemini stream: usage (when present) is cumulative; the
			// last one wins.
			if !u.IsZero() {
				acc = u
			}
		}
	}
	return acc, mdl
}

func extractJSONUsage(prov model.Provider, body []byte) (model.TokenUsage, string) {
	var probe map[string]any
	if json.Unmarshal(body, &probe) != nil {
		return model.TokenUsage{}, ""
	}
	mdl, _ := probe["model"].(string)
	// Anthropic streaming wraps the message.
	if msg := findMap(probe, "message"); msg != nil {
		if m, ok := msg["model"].(string); ok && m != "" {
			mdl = m
		}
	}

	if prov == model.ProviderGoogle {
		um := findMap(probe, "usageMetadata")
		if um == nil {
			return model.TokenUsage{}, mdl
		}
		gi := func(k string) int64 {
			if f, ok := um[k].(float64); ok {
				return int64(f)
			}
			return 0
		}
		return model.TokenUsage{
			InputTokens:     gi("promptTokenCount"),
			OutputTokens:    gi("candidatesTokenCount") + gi("thoughtsTokenCount"),
			CacheReadTokens: gi("cachedContentTokenCount"),
		}, mdl
	}

	usage := findMap(probe, "usage")
	if usage == nil {
		return model.TokenUsage{}, mdl
	}
	gi := func(m map[string]any, keys ...string) int64 {
		for _, k := range keys {
			if f, ok := m[k].(float64); ok {
				return int64(f)
			}
		}
		return 0
	}
	var cachedRead int64
	if d := findMap(usage, "prompt_tokens_details"); d != nil {
		cachedRead = gi(d, "cached_tokens")
	}
	if d := findMap(usage, "input_tokens_details"); d != nil {
		cachedRead += gi(d, "cached_tokens")
	}
	u := model.TokenUsage{
		InputTokens: gi(usage, "input_tokens", "prompt_tokens"),
		OutputTokens: gi(usage, "output_tokens", "completion_tokens") +
			gi(usage, "reasoning_tokens"),
		CacheReadTokens:  gi(usage, "cache_read_input_tokens") + cachedRead,
		CacheWriteTokens: gi(usage, "cache_creation_input_tokens"),
	}
	// OpenAI reports prompt_tokens inclusive of cached; separate them out.
	if u.CacheReadTokens > 0 && u.InputTokens >= u.CacheReadTokens && gi(usage, "prompt_tokens") > 0 {
		u.InputTokens -= u.CacheReadTokens
	}
	return u, mdl
}
