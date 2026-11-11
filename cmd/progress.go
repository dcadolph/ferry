package cmd

import (
	"fmt"
	"io"
	"strconv"
	"sync"

	"go.uber.org/zap"

	"github.com/dcadolph/ferry/internal/engine"
	"github.com/dcadolph/ferry/internal/util"
)

// polished names the analogy in the lines about its re-check.
const polished = "Polished analogy"

// progressFunc returns the engine's progress function for one run. At a terminal it prints a
// line for each event to w. Anywhere else it stays quiet except for a draft or a check that
// failed, which it logs at warn level, because that world is otherwise just missing from the
// result. Every other failure already ends in an error line or a note.
func progressFunc(w io.Writer, terminal bool, logger *zap.Logger) engine.ProgressFunc {
	if logger == nil {
		panic("progressFunc: logger required")
	}
	if terminal {
		return newProgressPrinter(w).print
	}
	return func(ev engine.Event) {
		item := ev.Step == engine.StepDraft || ev.Step == engine.StepCheck
		if !item || ev.Status != engine.StatusFailed {
			return
		}
		logger.Warn("step failed",
			zap.String("step", string(ev.Step)),
			zap.String("world", ev.World),
			zap.Int("round", ev.Round),
			zap.String("detail", util.CollapseSpace(ev.Detail)),
		)
	}
}

// progressPrinter writes engine events to a terminal as plain lines, one per event that has
// something to report.
type progressPrinter struct {
	// mu serializes writes, since the engine reports from several goroutines at once.
	mu sync.Mutex
	// w receives the lines.
	w io.Writer
}

// newProgressPrinter returns a progressPrinter that writes to w.
func newProgressPrinter(w io.Writer) *progressPrinter {
	if w == nil {
		panic("newProgressPrinter: writer required")
	}
	return &progressPrinter{w: w}
}

// print writes the line for ev, if the event has one. It is safe for concurrent use.
func (p *progressPrinter) print(ev engine.Event) {
	line := progressLine(ev)
	if line == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintln(p.w, line)
}

// progressLine returns the line for ev, or empty when the event needs no line of its own. A
// kept check shows its fit, e.g. "cars: kept, fit 4", and a dropped one shows its fit and the
// first problem, e.g. "kitchens: dropped, fit 2, the chain runs backward". A finished polish
// has no line, since the note the engine may attach to it is printed with the result.
func progressLine(ev engine.Event) string {
	switch ev.Step {
	case engine.StepSkeleton:
		switch ev.Status {
		case engine.StatusStarted:
			return "Reading the situation"
		case engine.StatusFailed:
			return withDetail("Reading the situation failed", ev.Detail)
		}
	case engine.StepDraft:
		switch ev.Status {
		case engine.StatusStarted:
			if ev.Round > 1 {
				return fmt.Sprintf("Drafting from %s (round %d)", ev.World, ev.Round)
			}
			return "Drafting from " + ev.World
		case engine.StatusFailed:
			return withDetail(ev.World+": draft failed", ev.Detail)
		}
	case engine.StepCheck:
		switch ev.Status {
		case engine.StatusStarted:
			return "Checking the " + ev.World + " draft"
		case engine.StatusKept:
			return withFit(ev.World+": kept", ev.Fit)
		case engine.StatusDropped:
			return withDetail(withFit(ev.World+": dropped", ev.Fit), ev.Detail)
		case engine.StatusFailed:
			return withDetail(ev.World+": check failed", ev.Detail)
		}
	case engine.StepFinal:
		switch ev.Status {
		case engine.StatusStarted:
			if ev.World == "" {
				return "Polishing the analogy"
			}
			return "Polishing the " + ev.World + " analogy"
		case engine.StatusFailed:
			return withDetail("Polishing failed", ev.Detail)
		}
	case engine.StepRecheck:
		switch ev.Status {
		case engine.StatusStarted:
			return "Checking the polished analogy"
		case engine.StatusKept:
			return withFit(polished+": kept", ev.Fit)
		case engine.StatusDropped:
			return withDetail(withFit(polished+": dropped", ev.Fit), ev.Detail)
		case engine.StatusFailed:
			return withDetail(polished+": check failed", ev.Detail)
		}
	}
	return ""
}

// withFit appends the fit score to line after a comma, when the event carries one.
func withFit(line string, fit int) string {
	if fit < 1 {
		return line
	}
	return line + ", fit " + strconv.Itoa(fit)
}

// withDetail appends detail on one line to line after a comma, when there is one.
func withDetail(line, detail string) string {
	if detail = util.CollapseSpace(detail); detail == "" {
		return line
	}
	return line + ", " + detail
}
