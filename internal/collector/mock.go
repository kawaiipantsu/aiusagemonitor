package collector

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/kawaiipantsu/aiusagemonitor/internal/model"
)

// Mock is a synthetic data generator. It produces plausible traffic for every
// provider plus rate-limit buckets that deplete and reset, so the UI has
// something lively to show without any real connection (aiusagemonitor --demo).
type Mock struct {
	Rate time.Duration // event cadence; default 900ms
}

func (m *Mock) Name() string             { return "demo" }
func (m *Mock) Provider() model.Provider { return "" }

type mockProv struct {
	prov    model.Provider
	models  []string
	session string
	label   string
	reqLim  float64
	tokLim  float64
	reqRem  float64
	tokRem  float64
	reset   time.Time
	phase   float64
}

func (m *Mock) Run(ctx context.Context, out chan<- Emission) error {
	if m.Rate <= 0 {
		m.Rate = 900 * time.Millisecond
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	now := time.Now()
	provs := []*mockProv{
		{prov: model.ProviderAnthropic, models: []string{"claude-sonnet-5", "claude-haiku-4-5"}, session: "cc-" + shortID(rng), label: "webapp", reqLim: 1000, tokLim: 400_000, reset: now.Add(time.Minute)},
		{prov: model.ProviderOpenAI, models: []string{"gpt-5", "gpt-4.1-mini", "o4-mini"}, session: "cx-" + shortID(rng), label: "api-svc", reqLim: 5000, tokLim: 2_000_000, reset: now.Add(time.Minute)},
		{prov: model.ProviderGoogle, models: []string{"gemini-2.5-pro", "gemini-2.5-flash"}, session: "gm-" + shortID(rng), label: "batch", reqLim: 2000, tokLim: 1_000_000, reset: now.Add(time.Minute)},
		{prov: model.ProviderXAI, models: []string{"grok-4", "grok-code-fast-1"}, session: "xa-" + shortID(rng), label: "agent", reqLim: 480, tokLim: 2_000_000, reset: now.Add(time.Minute)},
	}
	for _, p := range provs {
		p.reqRem, p.tokRem = p.reqLim, p.tokLim
		p.phase = rng.Float64() * math.Pi * 2
	}

	// Backfill ~45 min of history so History/Profile aren't empty on launch.
	m.backfill(ctx, out, provs, rng)

	emit(ctx, out, Emission{Source: m.Name(), Note: "synthetic demo data"})
	tick := time.NewTicker(m.Rate)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case t := <-tick.C:
			p := provs[rng.Intn(len(provs))]
			m.step(ctx, out, p, rng, t, false)
		}
	}
}

func (m *Mock) backfill(ctx context.Context, out chan<- Emission, provs []*mockProv, rng *rand.Rand) {
	start := time.Now().Add(-45 * time.Minute)
	for t := start; t.Before(time.Now()); t = t.Add(20 * time.Second) {
		if rng.Float64() < 0.4 {
			continue
		}
		p := provs[rng.Intn(len(provs))]
		m.step(ctx, out, p, rng, t, true)
	}
}

func (m *Mock) step(ctx context.Context, out chan<- Emission, p *mockProv, rng *rand.Rand, t time.Time, historical bool) {
	// Diurnal-ish wave so graphs have shape.
	wave := 0.5 + 0.5*math.Sin(p.phase+float64(t.Unix())/420.0)
	in := int64((300 + rng.Float64()*2500) * (0.4 + wave))
	cacheRead := int64(float64(in) * (0.3 + rng.Float64()*0.5))
	cacheWrite := int64(float64(in) * rng.Float64() * 0.2)
	outTok := int64((80 + rng.Float64()*1400) * (0.4 + wave))
	u := model.TokenUsage{InputTokens: in, OutputTokens: outTok, CacheReadTokens: cacheRead, CacheWriteTokens: cacheWrite, Requests: 1}

	mdl := p.models[rng.Intn(len(p.models))]
	emit(ctx, out, Emission{Source: m.Name(), Event: &model.Event{
		Provider:     p.prov,
		Source:       m.Name(),
		Time:         t,
		SessionID:    p.session,
		SessionLabel: p.label,
		Model:        mdl,
		Kind:         "demo",
		Usage:        u,
	}})

	if historical {
		return
	}
	// Deplete + reset the live limit buckets.
	now := time.Now()
	if now.After(p.reset) {
		p.reqRem, p.tokRem = p.reqLim, p.tokLim
		p.reset = now.Add(time.Minute)
	}
	p.reqRem = math.Max(0, p.reqRem-float64(1+rng.Intn(3)))
	p.tokRem = math.Max(0, p.tokRem-float64(u.Total()))
	for _, l := range []model.Limit{
		{Provider: p.prov, Kind: model.LimitRequests, Window: "1m", Limit: p.reqLim, Remaining: p.reqRem, ResetAt: p.reset, Observed: now, Model: mdl},
		{Provider: p.prov, Kind: model.LimitTokens, Window: "1m", Limit: p.tokLim, Remaining: p.tokRem, ResetAt: p.reset, Observed: now, Model: mdl},
	} {
		ll := l
		emit(ctx, out, Emission{Source: m.Name(), Limit: &ll})
	}
}

func shortID(rng *rand.Rand) string {
	const alpha = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 6)
	for i := range b {
		b[i] = alpha[rng.Intn(len(alpha))]
	}
	return string(b)
}
