// Package config loads and persists the on-disk YAML configuration.
//
// Secrets (API keys) are stored in this file. It is written with 0600
// permissions and every string value supports ${ENV_VAR} / $ENV_VAR
// expansion so users can keep keys out of the file entirely.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kawaiipantsu/aiusagemonitor/internal/model"
	"github.com/kawaiipantsu/aiusagemonitor/internal/pricing"
	"gopkg.in/yaml.v3"
)

// CollectorKind selects how usage is gathered for a provider.
type CollectorKind string

const (
	// CollectorLogs tails local AI-CLI session transcripts (Claude Code,
	// Codex CLI, Gemini CLI). No API key required.
	CollectorLogs CollectorKind = "logs"
	// CollectorProxy captures traffic routed through the built-in local proxy.
	CollectorProxy CollectorKind = "proxy"
	// CollectorPoll periodically queries the vendor usage / admin API.
	CollectorPoll CollectorKind = "poll"
	// CollectorOff disables the provider.
	CollectorOff CollectorKind = "off"
)

// AllCollectorKinds is used by the settings UI to cycle values.
var AllCollectorKinds = []CollectorKind{CollectorLogs, CollectorProxy, CollectorPoll, CollectorOff}

// ProviderConfig is per-vendor connection settings.
type ProviderConfig struct {
	Enabled   bool          `yaml:"enabled"`
	Collector CollectorKind `yaml:"collector"`
	// APIKey is the standard secret used by the proxy (forwarded upstream) and
	// as a fallback for polling.
	APIKey string `yaml:"api_key"`
	// AdminKey is an org/admin scoped key required by some usage APIs
	// (OpenAI organization usage, Anthropic usage report).
	AdminKey string `yaml:"admin_key"`
	// BaseURL overrides the upstream endpoint (self-hosted gateways, Azure...).
	BaseURL string `yaml:"base_url"`
	// Models optionally narrows which model ids are shown for this provider.
	Models []string `yaml:"models"`
	// Org / Project scoping for OpenAI-style APIs.
	Org     string `yaml:"org,omitempty"`
	Project string `yaml:"project,omitempty"`
}

// LogsConfig configures the local transcript tailers.
type LogsConfig struct {
	// Paths overrides auto-detection. Empty means "use the built-in defaults
	// for each detected CLI".
	ClaudeCodeDir string `yaml:"claude_code_dir"`
	CodexDir      string `yaml:"codex_dir"`
	GeminiDir     string `yaml:"gemini_dir"`
	// PollInterval is how often directories are re-scanned for new lines.
	PollInterval time.Duration `yaml:"poll_interval"`
	// Backfill controls how much history is ingested on first start.
	Backfill time.Duration `yaml:"backfill"`
	// ClaudeAccountStatus reads Claude Code's own local cache (~/.claude.json,
	// ~/.claude/.credentials.json) to show the login type (Claude.ai
	// subscription vs. a console API key) and the Pro/Max session & weekly
	// usage allowance, including "low priority" throttling. It only ever
	// reads plan/quota metadata — never the OAuth tokens, email or name.
	ClaudeAccountStatus *bool `yaml:"claude_account_status,omitempty"`
}

// AccountStatusEnabled reports whether the Claude account-status reader
// should run (default true; only "false" turns it off).
func (l LogsConfig) AccountStatusEnabled() bool {
	return l.ClaudeAccountStatus == nil || *l.ClaudeAccountStatus
}

// ProxyConfig configures the built-in capture proxy.
type ProxyConfig struct {
	Enabled bool   `yaml:"enabled"`
	Listen  string `yaml:"listen"` // host:port
	// TLS is optional; when Cert/Key are set the proxy serves HTTPS.
	Cert string `yaml:"cert,omitempty"`
	Key  string `yaml:"key,omitempty"`
}

// PollConfig holds shared polling knobs.
type PollConfig struct {
	Interval time.Duration `yaml:"interval"`
}

// StorageConfig controls the history database.
type StorageConfig struct {
	Path          string        `yaml:"path"`
	RetentionDays int           `yaml:"retention_days"` // 0 = keep forever
	FlushInterval time.Duration `yaml:"flush_interval"`
}

// UIConfig holds appearance / behaviour preferences.
type UIConfig struct {
	Theme        string        `yaml:"theme"`
	GraphStyle   string        `yaml:"graph_style"` // "braille" | "block" | "bar"
	RefreshRate  time.Duration `yaml:"refresh_rate"`
	WindowMin    int           `yaml:"window_minutes"` // dashboard time window
	Use24Hour    bool          `yaml:"use_24h"`
	StartView    string        `yaml:"start_view"` // dashboard|sessions|history|profile|waterfall|settings
	MouseEnabled bool          `yaml:"mouse_enabled"`
	// NerdFont switches chrome icons (tab bar, status bar) to Nerd Font
	// glyphs. Off by default: they render as tofu/boxes without a patched
	// Nerd Font installed in the terminal.
	NerdFont bool `yaml:"nerd_font"`
}

// Config is the whole document.
type Config struct {
	Providers map[model.Provider]*ProviderConfig `yaml:"providers"`
	Logs      LogsConfig                         `yaml:"logs"`
	Proxy     ProxyConfig                        `yaml:"proxy"`
	Poll      PollConfig                         `yaml:"poll"`
	Storage   StorageConfig                      `yaml:"storage"`
	UI        UIConfig                           `yaml:"ui"`
	// Pricing overrides keyed by model-id prefix.
	Pricing map[string]pricing.Rate `yaml:"pricing,omitempty"`
	// Demo, when true, runs the synthetic data generator so the UI has
	// something to show without any real connection.
	Demo bool `yaml:"demo"`

	path string `yaml:"-"`
}

// Path returns the file this config was loaded from (may be empty).
func (c *Config) Path() string { return c.path }

// SetPath records where Save should write.
func (c *Config) SetPath(p string) { c.path = p }

// DefaultDir returns the per-user config directory for the app.
func DefaultDir() string {
	if d, err := os.UserConfigDir(); err == nil && d != "" {
		return filepath.Join(d, "aiusagemonitor")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "aiusagemonitor")
}

// DefaultPath is the default config file location.
func DefaultPath() string { return filepath.Join(DefaultDir(), "config.yaml") }

// DefaultDBPath is the default history database location.
func DefaultDBPath() string { return filepath.Join(DefaultDir(), "history.db") }

// New returns a config populated with sensible defaults.
func New() *Config {
	c := &Config{
		Providers: map[model.Provider]*ProviderConfig{},
		Logs: LogsConfig{
			PollInterval: 2 * time.Second,
			Backfill:     24 * time.Hour,
		},
		Proxy: ProxyConfig{
			Enabled: false,
			Listen:  "127.0.0.1:8317",
		},
		Poll: PollConfig{Interval: 5 * time.Minute},
		Storage: StorageConfig{
			Path:          DefaultDBPath(),
			RetentionDays: 90,
			FlushInterval: 3 * time.Second,
		},
		UI: UIConfig{
			Theme:        "midnight",
			GraphStyle:   "braille",
			RefreshRate:  time.Second,
			WindowMin:    60,
			Use24Hour:    true,
			StartView:    "dashboard",
			MouseEnabled: true,
		},
	}
	for _, p := range model.AllProviders {
		kind := CollectorLogs
		if p == model.ProviderXAI {
			kind = CollectorProxy // no local CLI transcripts for Grok yet
		}
		c.Providers[p] = &ProviderConfig{
			Enabled:   p == model.ProviderAnthropic || p == model.ProviderOpenAI,
			Collector: kind,
		}
	}
	return c
}

// Load reads path, applying defaults for anything missing. A missing file is
// not an error: the defaults are returned and path is remembered for Save.
func Load(path string) (*Config, error) {
	c := New()
	c.path = path
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return c, nil
		}
		return nil, err
	}
	// Decode into the defaulted struct so unspecified keys keep their defaults.
	if err := yaml.Unmarshal(raw, c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if c.Providers == nil {
		c.Providers = map[model.Provider]*ProviderConfig{}
	}
	for _, p := range model.AllProviders {
		if c.Providers[p] == nil {
			c.Providers[p] = &ProviderConfig{Collector: CollectorLogs}
		}
		if c.Providers[p].Collector == "" {
			c.Providers[p].Collector = CollectorLogs
		}
	}
	c.normalise()
	return c, nil
}

func (c *Config) normalise() {
	if c.Logs.PollInterval <= 0 {
		c.Logs.PollInterval = 2 * time.Second
	}
	if c.Poll.Interval < time.Minute {
		c.Poll.Interval = time.Minute
	}
	if c.Storage.Path == "" {
		c.Storage.Path = DefaultDBPath()
	}
	if c.Storage.FlushInterval <= 0 {
		c.Storage.FlushInterval = 3 * time.Second
	}
	if c.UI.RefreshRate < 200*time.Millisecond {
		c.UI.RefreshRate = time.Second
	}
	if c.UI.WindowMin <= 0 {
		c.UI.WindowMin = 60
	}
	if c.UI.Theme == "" {
		c.UI.Theme = "midnight"
	}
	if c.UI.GraphStyle == "" {
		c.UI.GraphStyle = "braille"
	}
	if c.Proxy.Listen == "" {
		c.Proxy.Listen = "127.0.0.1:8317"
	}
}

// Save writes the config to its path (creating parent dirs) with 0600 perms.
func (c *Config) Save() error {
	if c.path == "" {
		return errors.New("config: no path set")
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return err
	}
	out, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

// Expand resolves ${VAR} / $VAR references in a stored string. A value that is
// not a reference is returned unchanged.
func Expand(s string) string {
	if s == "" || !strings.Contains(s, "$") {
		return s
	}
	return os.Expand(s, func(k string) string { return os.Getenv(k) })
}

// Resolved returns a copy of the provider config with secrets expanded.
func (pc ProviderConfig) Resolved() ProviderConfig {
	out := pc
	out.APIKey = Expand(pc.APIKey)
	out.AdminKey = Expand(pc.AdminKey)
	out.BaseURL = Expand(pc.BaseURL)
	out.Org = Expand(pc.Org)
	out.Project = Expand(pc.Project)
	return out
}

// PricingTable builds a resolver from the configured overrides.
func (c *Config) PricingTable() *pricing.Table { return pricing.NewTable(c.Pricing) }
