// Package collector gathers usage data from a source and streams it as
// model.Event / model.Limit values. Collectors are deliberately dumb: they do
// no aggregation and no persistence, they just observe and emit.
package collector

import (
	"context"

	"github.com/kawaiipantsu/aiusagemonitor/internal/model"
)

// Emission is one unit of output from a collector. Either Event or Limit (or
// both) may be set; Err carries a non-fatal problem the UI can surface.
type Emission struct {
	Event   *model.Event
	Limit   *model.Limit
	Account *model.AccountStatus // vendor CLI login/plan state (Claude Code, ...)
	Err     error
	Source  string
	Note    string // optional human status ("watching 3 files", "polled ok")
}

// Collector is a long-running observer.
type Collector interface {
	// Name is a stable identifier ("claude-code", "proxy", "poll-openai").
	Name() string
	// Provider is the vendor this collector reports for. A collector that spans
	// multiple providers (the proxy) returns "".
	Provider() model.Provider
	// Run blocks until ctx is cancelled, sending Emissions on out. It must not
	// close out. Returning an error means the collector stopped abnormally.
	Run(ctx context.Context, out chan<- Emission) error
}

// emit is a helper that respects context cancellation.
func emit(ctx context.Context, out chan<- Emission, e Emission) {
	select {
	case out <- e:
	case <-ctx.Done():
	}
}
