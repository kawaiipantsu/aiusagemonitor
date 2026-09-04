// Command aiusagemonitor is a terminal UI that shows live token usage, rate
// limits and cost for OpenAI, Claude (Anthropic), Google Gemini and xAI, plus
// historic summaries backed by a local SQLite database.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kawaiipantsu/aiusagemonitor/internal/app"
	"github.com/kawaiipantsu/aiusagemonitor/internal/config"
	"github.com/kawaiipantsu/aiusagemonitor/internal/engine"
	"github.com/kawaiipantsu/aiusagemonitor/internal/store"
	"github.com/kawaiipantsu/aiusagemonitor/internal/version"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "report":
			os.Exit(runReport(os.Args[2:]))
		case "proxy":
			os.Exit(runProxy(os.Args[2:]))
		case "paths":
			os.Exit(runPaths(os.Args[2:]))
		case "version", "--version", "-version":
			fmt.Println(version.String())
			return
		case "help", "--help", "-h":
			printUsage()
			return
		}
	}
	os.Exit(runTUI(os.Args[1:]))
}

func printUsage() {
	fmt.Print(`aiusagemonitor — live TUI usage/limit monitor for OpenAI, Claude, Google & xAI

Usage:
  aiusagemonitor [flags]            launch the TUI (default)
  aiusagemonitor report [flags]     print a usage summary and exit
  aiusagemonitor proxy [flags]      run the capture proxy headlessly (no TUI)
  aiusagemonitor paths              print config/database locations
  aiusagemonitor version            print the version and exit

Flags (TUI):
  -config PATH     config file (default ~/.config/aiusagemonitor/config.yaml)
  -db PATH         history database (default alongside the config file)
  -demo            clean showcase: synthetic traffic only, real collectors off
  -theme NAME      start with this theme
  -no-mouse        disable mouse reporting

Run "aiusagemonitor report -h" or "aiusagemonitor proxy -h" for those flags.
`)
}

func loadConfig(fs *flag.FlagSet) (*config.Config, error) {
	cfgPath := fs.Lookup("config").Value.String()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, err
	}
	if db := fs.Lookup("db").Value.String(); db != "" {
		cfg.Storage.Path = db
	}
	return cfg, nil
}

func runTUI(args []string) int {
	fs := flag.NewFlagSet("aiusagemonitor", flag.ExitOnError)
	fs.String("config", config.DefaultPath(), "config file path")
	fs.String("db", "", "history database path (overrides config)")
	demo := fs.Bool("demo", false, "show synthetic traffic only (no real collectors) — a clean showcase")
	themeName := fs.String("theme", "", "start with this theme")
	noMouse := fs.Bool("no-mouse", false, "disable mouse reporting")
	_ = fs.Parse(args)

	cfg, err := loadConfig(fs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 1
	}
	cfg.SetPath(fs.Lookup("config").Value.String())
	if *demo {
		// -demo is a clean showcase: disable every real collector so the
		// synthetic feed is all that's shown. (The "Demo data" toggle in
		// Settings behaves differently: it *adds* synthetic traffic on top
		// of whatever real collectors are already running.)
		cfg.Demo = true
		for _, pc := range cfg.Providers {
			pc.Enabled = false
		}
		cfg.Proxy.Enabled = false
	}
	if *themeName != "" {
		cfg.UI.Theme = *themeName
	}
	if *noMouse {
		cfg.UI.MouseEnabled = false
	}

	st, err := store.Open(cfg.Storage.Path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "store:", err)
		return 1
	}
	defer func() { _ = st.Close() }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	eng := engine.New(st, cfg)
	eng.Start(ctx)
	defer eng.Stop()

	model := app.New(ctx, cfg, st, eng)
	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if cfg.UI.MouseEnabled {
		opts = append(opts, tea.WithMouseCellMotion())
	}
	p := tea.NewProgram(model, opts...)

	go func() {
		<-ctx.Done()
		p.Quit()
	}()

	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "tui:", err)
		return 1
	}
	return 0
}

func runProxy(args []string) int {
	fs := flag.NewFlagSet("aiusagemonitor proxy", flag.ExitOnError)
	fs.String("config", config.DefaultPath(), "config file path")
	fs.String("db", "", "history database path (overrides config)")
	listen := fs.String("listen", "", "listen address (overrides config)")
	_ = fs.Parse(args)

	cfg, err := loadConfig(fs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 1
	}
	if *listen != "" {
		cfg.Proxy.Listen = *listen
	}
	cfg.Proxy.Enabled = true

	st, err := store.Open(cfg.Storage.Path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "store:", err)
		return 1
	}
	defer func() { _ = st.Close() }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	eng := engine.New(st, cfg)
	eng.Start(ctx)
	fmt.Printf("aiusagemonitor proxy listening on http://%s (routes: /openai /anthropic /google /xai). Ctrl+C to stop.\n", cfg.Proxy.Listen)

	sub := eng.Subscribe()
	seenNotes := map[string]string{}
	lastErrCount := 0
	go func() {
		for s := range sub {
			for name, note := range s.Notes {
				if seenNotes[name] != note {
					seenNotes[name] = note
					fmt.Printf("[%s] %s: %s\n", time.Now().Format("15:04:05"), name, note)
				}
			}
			if n := len(s.Errors); n > lastErrCount {
				for _, e := range s.Errors[lastErrCount:n] {
					fmt.Printf("[%s] %s error: %s\n", e.Time.Format("15:04:05"), e.Source, e.Err)
				}
				lastErrCount = n
			}
		}
	}()

	<-ctx.Done()
	eng.Stop()
	return 0
}

func runPaths(args []string) int {
	fs := flag.NewFlagSet("aiusagemonitor paths", flag.ExitOnError)
	_ = fs.Parse(args)
	fmt.Println("config:", config.DefaultPath())
	fmt.Println("database:", config.DefaultDBPath())
	fmt.Println("config dir:", config.DefaultDir())
	return 0
}

func runReport(args []string) int {
	fs := flag.NewFlagSet("aiusagemonitor report", flag.ExitOnError)
	fs.String("config", config.DefaultPath(), "config file path")
	fs.String("db", "", "history database path (overrides config)")
	rangeStr := fs.String("range", "7d", "time range: 1h, 24h, 7d, 30d, 90d, all")
	asJSON := fs.Bool("json", false, "print machine-readable JSON instead of text")
	_ = fs.Parse(args)

	cfg, err := loadConfig(fs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 1
	}
	st, err := store.Open(cfg.Storage.Path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "store:", err)
		return 1
	}
	defer func() { _ = st.Close() }()

	span, err := parseRange(*rangeStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "range:", err)
		return 1
	}
	to := time.Now()
	from := to.Add(-span)
	if *rangeStr == "all" {
		from = time.Unix(0, 0)
	}

	ctx := context.Background()
	totals, err := st.RangeTotals(ctx, from, to)
	if err != nil {
		fmt.Fprintln(os.Stderr, "query:", err)
		return 1
	}

	if *asJSON {
		out, _ := json.MarshalIndent(totals, "", "  ")
		fmt.Println(string(out))
		return 0
	}

	fmt.Printf("aiusagemonitor report — %s to %s (%s)\n\n", from.Format(time.RFC3339), to.Format(time.RFC3339), *rangeStr)
	fmt.Printf("total tokens : %d (in %d / out %d / cache-read %d / cache-write %d)\n",
		totals.Usage.Total(), totals.Usage.InputTokens, totals.Usage.OutputTokens, totals.Usage.CacheReadTokens, totals.Usage.CacheWriteTokens)
	fmt.Printf("total cost   : $%.4f\n", totals.CostUSD)
	fmt.Printf("requests     : %d\n", totals.Usage.Requests)
	fmt.Printf("sessions     : %d\n", totals.Sessions)
	fmt.Printf("events       : %d\n\n", totals.Events)

	fmt.Println("by provider:")
	for p, u := range totals.ByProvider {
		fmt.Printf("  %-10s %10d tok   $%.4f\n", p.Title(), u.Total(), totals.CostByProv[p])
	}
	fmt.Println("\nby model:")
	for m, u := range totals.ByModel {
		fmt.Printf("  %-24s %10d tok   $%.4f\n", m, u.Total(), totals.CostByModel[m])
	}
	return 0
}

func parseRange(s string) (time.Duration, error) {
	switch s {
	case "all":
		return 0, nil
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	// Support "7d" / "30d" / "90d" which time.ParseDuration rejects.
	if len(s) > 1 && s[len(s)-1] == 'd' {
		var days int
		if _, err := fmt.Sscanf(s, "%dd", &days); err == nil {
			return time.Duration(days) * 24 * time.Hour, nil
		}
	}
	return 0, fmt.Errorf("unrecognised range %q (try 1h, 24h, 7d, 30d, 90d, all)", s)
}
