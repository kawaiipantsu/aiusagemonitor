package collector

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kawaiipantsu/aiusagemonitor/internal/model"
)

// sampleCredentials and sampleState mirror the real files' shapes with
// fabricated values — no real account data. accessToken/refreshToken are
// present exactly as Claude Code writes them, to prove buildAccountStatus
// never needs (or reads) them into anything we emit.
const sampleCredentials = `{
	"claudeAiOauth": {
		"accessToken": "not-a-real-token",
		"refreshToken": "not-a-real-token",
		"expiresAt": 1999999999000,
		"refreshTokenExpiresAt": 1999999999000,
		"scopes": ["user:inference"],
		"subscriptionType": "pro",
		"rateLimitTier": "default_claude_ai"
	}
}`

const sampleState = `{
	"oauthAccount": {
		"organizationType": "claude_pro",
		"billingType": "stripe_subscription",
		"organizationRateLimitTier": "default_claude_ai"
	},
	"cachedUsageUtilization": {
		"fetchedAtMs": 1999999999000,
		"utilization": {
			"five_hour": {"utilization": 100, "resets_at": "2099-01-01T22:30:00+00:00"},
			"seven_day": {"utilization": 29, "resets_at": "2099-01-05T09:00:00+00:00"},
			"limits": [
				{"kind": "session", "percent": 100, "severity": "critical", "is_active": true},
				{"kind": "weekly_all", "percent": 29, "severity": "normal", "is_active": false}
			]
		}
	}
}`

func writeFixtures(t *testing.T, credJSON, stateJSON string) (credPath, statePath string) {
	t.Helper()
	dir := t.TempDir()
	credPath = filepath.Join(dir, "credentials.json")
	statePath = filepath.Join(dir, "state.json")
	if credJSON != "" {
		if err := os.WriteFile(credPath, []byte(credJSON), 0o600); err != nil {
			t.Fatal(err)
		}
	} else {
		credPath = filepath.Join(dir, "missing-credentials.json")
	}
	if stateJSON != "" {
		if err := os.WriteFile(statePath, []byte(stateJSON), 0o600); err != nil {
			t.Fatal(err)
		}
	} else {
		statePath = filepath.Join(dir, "missing-state.json")
	}
	return credPath, statePath
}

func TestBuildAccountStatusSubscriptionLowPriority(t *testing.T) {
	_ = os.Unsetenv("ANTHROPIC_API_KEY")
	_ = os.Unsetenv("ANTHROPIC_AUTH_TOKEN")
	credPath, statePath := writeFixtures(t, sampleCredentials, sampleState)

	st := buildAccountStatus(credPath, statePath)
	if st == nil {
		t.Fatal("expected a non-nil AccountStatus")
	}
	if st.Login != model.LoginSubscription {
		t.Errorf("Login = %q, want subscription", st.Login)
	}
	if st.PlanLabel != "Pro" {
		t.Errorf("PlanLabel = %q, want Pro", st.PlanLabel)
	}
	if !st.LowPriority {
		t.Errorf("LowPriority = false, want true (session window is critical+active at 100%%)")
	}

	session, ok := st.WindowByKind("session")
	if !ok {
		t.Fatal("expected a session window")
	}
	if session.Used != 100 {
		t.Errorf("session.Used = %v, want 100", session.Used)
	}
	if session.Remaining() != 0 {
		t.Errorf("session.Remaining() = %v, want 0", session.Remaining())
	}

	weekly, ok := st.WindowByKind("weekly")
	if !ok {
		t.Fatal("expected a weekly window")
	}
	if weekly.Used != 29 {
		t.Errorf("weekly.Used = %v, want 29", weekly.Used)
	}
	if got := weekly.Remaining(); got != 71 {
		t.Errorf("weekly.Remaining() = %v, want 71 (matches the '71%% allowance left' the user sees)", got)
	}
	if weekly.Active {
		t.Errorf("weekly window should not be the active/binding constraint here")
	}

	wantFetched := time.UnixMilli(1999999999000)
	if !st.FetchedAt.Equal(wantFetched) {
		t.Errorf("FetchedAt = %v, want %v (the CLI's own refresh time, not our poll time — it can lag far behind 'now')", st.FetchedAt, wantFetched)
	}
}

func TestBuildAccountStatusAPIKeyEnvTakesPrecedence(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-not-a-real-key")
	credPath, statePath := writeFixtures(t, sampleCredentials, sampleState)

	st := buildAccountStatus(credPath, statePath)
	if st == nil {
		t.Fatal("expected a non-nil AccountStatus")
	}
	if st.Login != model.LoginAPIKey {
		t.Errorf("Login = %q, want api_key (env var must win over a stored OAuth credential)", st.Login)
	}
}

func TestBuildAccountStatusNoCredentials(t *testing.T) {
	_ = os.Unsetenv("ANTHROPIC_API_KEY")
	_ = os.Unsetenv("ANTHROPIC_AUTH_TOKEN")
	credPath, statePath := writeFixtures(t, "", "")

	if st := buildAccountStatus(credPath, statePath); st != nil {
		t.Errorf("expected nil when neither an API key nor a stored OAuth credential exists, got %+v", st)
	}
}

func TestBuildAccountStatusObservedIsFresh(t *testing.T) {
	_ = os.Unsetenv("ANTHROPIC_API_KEY")
	credPath, statePath := writeFixtures(t, sampleCredentials, sampleState)
	before := time.Now()
	st := buildAccountStatus(credPath, statePath)
	if st == nil {
		t.Fatal("expected a status")
	}
	if st.Observed.Before(before) || st.Observed.After(time.Now()) {
		t.Errorf("Observed = %v, want a timestamp taken during this call", st.Observed)
	}
}
