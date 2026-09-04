package collector

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/kawaiipantsu/aiusagemonitor/internal/model"
)

// ClaudeAccount watches Claude Code's own local state for the CLI's login
// type (Claude.ai subscription vs. a pay-as-you-go console API key) and its
// Pro/Max-style session and weekly usage allowance — the "low priority" mode
// Claude Code drops into once the rolling 5-hour session window is spent.
//
// This is deliberately narrow: it reads two small, well-known cache files
// Claude Code itself maintains and only extracts plan/quota *metadata*. It
// never reads, logs or emits the OAuth access/refresh tokens, nor the
// account's email or name.
type ClaudeAccount struct {
	CredentialsPath string // default ~/.claude/.credentials.json
	StatePath       string // default ~/.claude.json
	Interval        time.Duration

	lastCredMod  time.Time
	lastStateMod time.Time
	lastFP       string
}

func (c *ClaudeAccount) Name() string             { return "claude-account" }
func (c *ClaudeAccount) Provider() model.Provider { return model.ProviderAnthropic }

func (c *ClaudeAccount) credPath() string {
	if c.CredentialsPath != "" {
		return expandHome(c.CredentialsPath)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", ".credentials.json")
}

func (c *ClaudeAccount) statePath() string {
	if c.StatePath != "" {
		return expandHome(c.StatePath)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude.json")
}

// credentialsFile is the small slice of ~/.claude/.credentials.json this
// collector reads. accessToken/refreshToken are intentionally absent below
// so they are never even unmarshalled.
type credentialsFile struct {
	ClaudeAiOauth *struct {
		SubscriptionType string `json:"subscriptionType"`
		RateLimitTier    string `json:"rateLimitTier"`
	} `json:"claudeAiOauth"`
}

// claudeState is the small slice of ~/.claude.json this collector reads.
type claudeState struct {
	OauthAccount *struct {
		OrganizationType          string `json:"organizationType"`
		BillingType               string `json:"billingType"`
		OrganizationRateLimitTier string `json:"organizationRateLimitTier"`
	} `json:"oauthAccount"`
	CachedUsageUtilization *struct {
		FetchedAtMs int64 `json:"fetchedAtMs"`
		Utilization struct {
			FiveHour *utilWindow  `json:"five_hour"`
			SevenDay *utilWindow  `json:"seven_day"`
			Limits   []limitEntry `json:"limits"`
		} `json:"utilization"`
	} `json:"cachedUsageUtilization"`
}

type utilWindow struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    string  `json:"resets_at"`
}

type limitEntry struct {
	Kind     string  `json:"kind"`
	Percent  float64 `json:"percent"`
	Severity string  `json:"severity"`
	ResetsAt string  `json:"resets_at"`
	IsActive bool    `json:"is_active"`
}

func (c *ClaudeAccount) Run(ctx context.Context, out chan<- Emission) error {
	if c.Interval <= 0 {
		c.Interval = 5 * time.Second
	}
	emit(ctx, out, Emission{Source: c.Name(), Note: "watching " + c.statePath()})
	c.poll(ctx, out)
	t := time.NewTicker(c.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			c.poll(ctx, out)
		}
	}
}

func (c *ClaudeAccount) poll(ctx context.Context, out chan<- Emission) {
	credPath, statePath := c.credPath(), c.statePath()

	credMod, ok1 := modTime(credPath)
	stateMod, ok2 := modTime(statePath)
	if !ok1 && !ok2 {
		return // neither file exists yet (Claude Code never run, or a fresh machine)
	}
	if credMod.Equal(c.lastCredMod) && stateMod.Equal(c.lastStateMod) && c.lastFP != "" {
		return // nothing changed since the last poll; skip the reparse
	}
	c.lastCredMod, c.lastStateMod = credMod, stateMod

	status := buildAccountStatus(credPath, statePath)
	if status == nil {
		return
	}
	fp := string(status.Login) + status.PlanLabel
	for _, w := range status.Windows {
		fp += w.Kind + itoa(int(w.Used))
	}
	c.lastFP = fp
	emit(ctx, out, Emission{Source: c.Name(), Account: status})
}

func modTime(path string) (time.Time, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, false
	}
	return info.ModTime(), true
}

// buildAccountStatus reads both files fresh and derives an AccountStatus.
// Missing/unreadable files degrade gracefully rather than erroring — this
// runs on every user's machine, most of whom will have neither file in the
// exact shape we expect forever (Claude Code's own cache format may change).
func buildAccountStatus(credPath, statePath string) *model.AccountStatus {
	var creds credentialsFile
	_ = readJSON(credPath, &creds)
	var state claudeState
	_ = readJSON(statePath, &state)

	var login model.LoginKind
	var plan string
	switch {
	case os.Getenv("ANTHROPIC_API_KEY") != "" || os.Getenv("ANTHROPIC_AUTH_TOKEN") != "":
		// Anthropic's own tooling prefers an explicit key/token over a stored
		// browser login when both are present, so check this first.
		login = model.LoginAPIKey
		plan = "API Console (pay-as-you-go)"
	case creds.ClaudeAiOauth != nil && creds.ClaudeAiOauth.SubscriptionType != "":
		login = model.LoginSubscription
		plan = planLabel(state, creds)
	default:
		return nil // no recognisable auth state; say nothing rather than guess
	}

	st := &model.AccountStatus{
		Provider:  model.ProviderAnthropic,
		Source:    "claude-account",
		Login:     login,
		PlanLabel: plan,
		Observed:  time.Now(),
	}

	if u := state.CachedUsageUtilization; u != nil {
		if u.FetchedAtMs > 0 {
			st.FetchedAt = time.UnixMilli(u.FetchedAtMs)
		}
		byKind := map[string]limitEntry{}
		for _, l := range u.Utilization.Limits {
			byKind[l.Kind] = l
		}
		addWindow := func(kind string, w *utilWindow, limitKind string) {
			if w == nil {
				return
			}
			uw := model.UsageWindow{Kind: kind, Used: w.Utilization, ResetAt: parseTime(w.ResetsAt)}
			if l, ok := byKind[limitKind]; ok {
				uw.Active = l.IsActive
				uw.Severity = l.Severity
			}
			st.Windows = append(st.Windows, uw)
			if uw.Active && uw.Severity == "critical" {
				st.LowPriority = true
			}
		}
		addWindow("session", u.Utilization.FiveHour, "session")
		addWindow("weekly", u.Utilization.SevenDay, "weekly_all")
	}

	return st
}

func planLabel(state claudeState, creds credentialsFile) string {
	if state.OauthAccount != nil && state.OauthAccount.OrganizationType != "" {
		switch state.OauthAccount.OrganizationType {
		case "claude_pro":
			return "Pro"
		case "claude_max":
			return "Max"
		case "claude_team":
			return "Team"
		case "claude_enterprise":
			return "Enterprise"
		default:
			return state.OauthAccount.OrganizationType
		}
	}
	if creds.ClaudeAiOauth != nil && creds.ClaudeAiOauth.SubscriptionType != "" {
		return titleCase(creds.ClaudeAiOauth.SubscriptionType)
	}
	return "Claude.ai subscription"
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	b := []byte(s)
	if b[0] >= 'a' && b[0] <= 'z' {
		b[0] -= 'a' - 'A'
	}
	return string(b)
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
