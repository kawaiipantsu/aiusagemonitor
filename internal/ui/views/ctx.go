// Package views implements each full-screen tab of the TUI (Dashboard,
// Sessions, History, Profile, Settings, Help). Every view is a small
// self-contained Bubble Tea-style model driven explicitly by internal/app.
package views

import (
	"context"
	"time"

	"github.com/kawaiipantsu/aiusagemonitor/internal/config"
	"github.com/kawaiipantsu/aiusagemonitor/internal/engine"
	"github.com/kawaiipantsu/aiusagemonitor/internal/store"
	"github.com/kawaiipantsu/aiusagemonitor/internal/ui"
)

// Ctx bundles everything a view needs to render or react to input. It is
// rebuilt cheaply by the app on every Update/View call.
type Ctx struct {
	Styles ui.Styles
	State  *engine.DashboardState
	Store  *store.Store
	Cfg    *config.Config
	Eng    *engine.Engine
	Width  int
	Height int
	Now    time.Time
}

// DBCtx returns a background context — queries are local SQLite reads and
// expected to be fast, so views don't thread cancellation through.
func (c Ctx) DBCtx() context.Context { return context.Background() }

// Use24h is a shorthand used throughout the views.
func (c Ctx) Use24h() bool { return c.Cfg.UI.Use24Hour }

// ContentHeight returns the height available below the tab bar and above the
// status bar (both are 1 row).
func (c Ctx) ContentHeight() int {
	h := c.Height - 2
	if h < 1 {
		h = 1
	}
	return h
}
