package collector

import (
	"github.com/kawaiipantsu/aiusagemonitor/internal/config"
	"github.com/kawaiipantsu/aiusagemonitor/internal/model"
)

// Build turns a config into the set of collectors that should run. It never
// returns an error: misconfigured providers are simply skipped (the engine
// surfaces per-collector errors at runtime instead).
func Build(cfg *config.Config) []Collector {
	var cs []Collector
	proxyNeeded := cfg.Proxy.Enabled
	proxyBases := map[model.Provider]string{}

	for _, prov := range model.AllProviders {
		pc := cfg.Providers[prov]
		if pc == nil || !pc.Enabled {
			continue
		}
		r := pc.Resolved()
		if r.BaseURL != "" {
			proxyBases[prov] = r.BaseURL
		}
		switch pc.Collector {
		case config.CollectorLogs:
			cs = append(cs, logCollectorsFor(prov, cfg)...)
		case config.CollectorProxy:
			proxyNeeded = true
		case config.CollectorPoll:
			cs = append(cs, &Poll{
				Prov:     prov,
				Interval: cfg.Poll.Interval,
				APIKey:   r.APIKey,
				AdminKey: r.AdminKey,
				BaseURL:  r.BaseURL,
				Org:      r.Org,
			})
		case config.CollectorOff:
		}
	}

	if proxyNeeded {
		cs = append(cs, &Proxy{Listen: cfg.Proxy.Listen, BaseURLs: proxyBases})
	}
	if cfg.Demo {
		cs = append(cs, &Mock{})
	}
	return cs
}

func logCollectorsFor(prov model.Provider, cfg *config.Config) []Collector {
	iv := cfg.Logs.PollInterval
	bf := cfg.Logs.Backfill
	switch prov {
	case model.ProviderAnthropic:
		cs := []Collector{&ClaudeCode{Dir: cfg.Logs.ClaudeCodeDir, Interval: iv, Backfill: bf}}
		if cfg.Logs.AccountStatusEnabled() {
			cs = append(cs, &ClaudeAccount{})
		}
		return cs
	case model.ProviderOpenAI:
		return []Collector{&Codex{Dir: cfg.Logs.CodexDir, Interval: iv, Backfill: bf}}
	case model.ProviderGoogle:
		return []Collector{&GeminiCLI{Dir: cfg.Logs.GeminiDir, Interval: iv, Backfill: bf}}
	default:
		return nil // xAI has no local CLI transcript format yet
	}
}
