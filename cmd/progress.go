package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"go.uber.org/zap"
	"golang.org/x/term"

	"github.com/dcadolph/ferry/internal/engine"
	"github.com/dcadolph/ferry/internal/render"
	"github.com/dcadolph/ferry/internal/util"
)

// Layout of the progress display.
const (
	// worldColumn is the width the world name is padded to on an item line.
	worldColumn = 18
	// minReason is the fewest columns worth spending on a reason. With less room the reason is
	// left off the line.
	minReason = 16
	// tick is how often the live line redraws.
	tick = 100 * time.Millisecond
	// clearLine moves the cursor to the start of the line and erases it.
	clearLine = "\r\x1b[2K"
	// polishedLabel names the polished analogy on its check line.
	polishedLabel = "polished"
)

// spinFrames are the frames of the live line's spinner, in order.
var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"} //nolint:gochecknoglobals // Not modified.

// display describes the stream progress is written to.
type display struct {
	// terminal reports whether the stream is an interactive terminal.
	terminal bool
	// live reports whether a spinner line may redraw in place on the stream.
	live bool
	// style colors the text in ferry's palette, and its On reports whether emoji are used too.
	style render.Style
	// width is the stream's width in columns.
	width int
}

// progress reports engine events while a run works, and closes the report when it ends.
type progress struct {
	// report receives engine events. It is safe for concurrent use.
	report engine.ProgressFunc
	// finish stops the live line and prints the closing line. err is the run's error, or nil.
	// It must be called once, before the result is printed.
	finish func(err error)
}

// progressFunc returns the progress reporting for one run. At a terminal it prints a line for
// each finished step to w, with a live line for the steps still running. Anywhere else it stays
// quiet except for a draft or a check that failed, which it logs at warn level, because that
// world is otherwise just missing from the result. Every other failure already ends in an error
// line or a note.
func progressFunc(w io.Writer, d display, logger *zap.Logger) progress {
	if logger == nil {
		panic("progressFunc: logger required")
	}
	if d.terminal {
		p := newProgressPrinter(w, d, time.Now)
		return progress{report: p.print, finish: p.finish}
	}
	report := func(ev engine.Event) {
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
	return progress{report: report, finish: func(error) {}}
}

// task identifies one model call in flight.
type task struct {
	// step is the engine stage making the call.
	step engine.Step
	// world is the world the call works on, when there is one.
	world string
	// round is the drafting round, or 0 outside drafting and checking.
	round int
}

// progressPrinter writes engine events to a terminal. Each finished step gets a fixed line, and
// the steps still running share one live line at the bottom that redraws in place.
type progressPrinter struct {
	// mu guards every field below it, since the engine reports from several goroutines at once
	// and the live line redraws from its own.
	mu sync.Mutex
	// w receives the output.
	w io.Writer
	// d describes the stream.
	d display
	// now returns the current time.
	now func() time.Time
	// start is when the printer was made.
	start time.Time
	// active maps each call in flight to when it started.
	active map[task]time.Time
	// rounds records the drafting rounds whose heading is printed.
	rounds map[int]bool
	// calls counts the model calls started.
	calls int
	// frame is the spinner frame drawn next.
	frame int
	// drawn reports whether the live line is on screen.
	drawn bool
	// finished reports whether finish ran.
	finished bool
	// stop ends the redraw loop.
	stop chan struct{}
	// stopped is closed when the redraw loop has ended.
	stopped chan struct{}
}

// newProgressPrinter returns a progressPrinter that writes to w. When d.live is set it starts a
// loop that redraws the live line until finish is called.
func newProgressPrinter(w io.Writer, d display, now func() time.Time) *progressPrinter {
	if w == nil {
		panic("newProgressPrinter: writer required")
	}
	if now == nil {
		panic("newProgressPrinter: clock required")
	}
	if d.width < 1 {
		d.width = 80
	}
	p := &progressPrinter{
		w:       w,
		d:       d,
		now:     now,
		start:   now(),
		active:  make(map[task]time.Time),
		rounds:  make(map[int]bool),
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
	if d.live {
		go p.loop()
	} else {
		close(p.stopped)
	}
	return p
}

// loop redraws the live line every tick until stop is closed.
func (p *progressPrinter) loop() {
	defer close(p.stopped)
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-t.C:
			p.mu.Lock()
			p.frame++
			p.redraw()
			p.mu.Unlock()
		}
	}
}

// print records ev and writes the lines it calls for. It is safe for concurrent use.
func (p *progressPrinter) print(ev engine.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.finished {
		return
	}
	k := task{step: ev.Step, world: ev.World, round: ev.Round}
	var lines []string
	if ev.Status == engine.StatusStarted {
		p.calls++
		p.active[k] = p.now()
		lines = p.startLines(ev)
	} else {
		began, ok := p.active[k]
		if !ok {
			began = p.now()
		}
		delete(p.active, k)
		lines = p.endLines(ev, p.now().Sub(began))
	}
	if len(lines) == 0 {
		p.redraw()
		return
	}
	p.write(lines...)
}

// finish stops the live line and prints the closing line for a run that ended in err. A run
// stopped by Ctrl-C gets no closing line. It is safe to call more than once.
func (p *progressPrinter) finish(err error) {
	p.mu.Lock()
	if p.finished {
		p.mu.Unlock()
		return
	}
	p.finished = true
	p.mu.Unlock()
	close(p.stop)
	<-p.stopped
	p.mu.Lock()
	defer p.mu.Unlock()
	p.clear()
	clear(p.active)
	s := p.d.style
	took := fmt.Sprintf(" in %s · %s", duration(p.now().Sub(p.start)), plural(p.calls, "call"))
	switch {
	case err == nil:
		p.write("", s.Good(s.Bold("Done"))+s.Dim(took), "")
	case errors.Is(err, engine.ErrNoFit):
		p.write("", s.Warn(s.Bold("Nothing held up"))+s.Dim(took), "")
	case isQuiet(err):
	default:
		p.write("", s.Bad(s.Bold("Stopped"))+s.Dim(took))
	}
}

// startLines returns the fixed lines for a call that started: a heading when a step or a round
// begins, and nothing for the rest, which the live line covers.
func (p *progressPrinter) startLines(ev engine.Event) []string {
	s := p.d.style
	switch ev.Step {
	case engine.StepDraft:
		if p.rounds[ev.Round] {
			return nil
		}
		p.rounds[ev.Round] = true
		if ev.Round > 1 {
			return p.heading(s.Heading("Round "+strconv.Itoa(ev.Round)) +
				s.Dim("  nothing held up, so trying new worlds"))
		}
		return p.heading(s.Heading("Round 1") + s.Dim("  drafting from several worlds"))
	case engine.StepFinal:
		name := "Polishing the analogy"
		if ev.World != "" {
			name = "Polishing the " + ev.World + " analogy"
		}
		return p.heading(s.Heading(name))
	}
	return nil
}

// heading returns line as a heading, set off from what came before by a blank line.
func (p *progressPrinter) heading(line string) []string {
	return []string{"", line}
}

// endLines returns the fixed line for a call that ended, or nothing when the end needs no line.
func (p *progressPrinter) endLines(ev engine.Event, took time.Duration) []string {
	s := p.d.style
	switch ev.Step {
	case engine.StepSkeleton:
		switch ev.Status {
		case engine.StatusDone:
			return p.heading(p.emoji("🔍 ") + s.Heading("Read the situation") +
				s.Dim("  "+duration(took)))
		case engine.StatusFailed:
			return append(p.heading(p.emoji("🔍 ")+s.Heading("Reading the situation")),
				p.item(s.Bad("✗"), "", s.Bad("failed"), 0, ev.Detail))
		}
	case engine.StepDraft:
		if ev.Status == engine.StatusFailed {
			return []string{p.item(s.Bad("✗"), ev.World, s.Bad("draft failed"), 0, ev.Detail)}
		}
	case engine.StepCheck, engine.StepRecheck:
		name := ev.World
		if ev.Step == engine.StepRecheck {
			name = polishedLabel
		}
		switch ev.Status {
		case engine.StatusKept:
			return []string{p.item(s.Good("✓"), name, s.Pill(true), ev.Fit, "")}
		case engine.StatusDropped:
			return []string{p.item(s.Bad("✗"), name, s.Pill(false), ev.Fit, ev.Detail)}
		case engine.StatusFailed:
			return []string{p.item(s.Warn("!"), name, s.Warn("check failed"), 0, ev.Detail)}
		}
	case engine.StepFinal:
		if ev.Status == engine.StatusFailed {
			return []string{p.item(s.Bad("✗"), "", s.Bad("polish failed"), 0, ev.Detail)}
		}
	}
	return nil
}

// item returns one indented item line: a mark, a padded world name, a status, a fit score, and
// the first sentence of reason, cut to fit the width. A status with a score is padded to the
// width of the dropped pill so the scores line up. Empty parts and a score under 1 are left out.
func (p *progressPrinter) item(mark, world, status string, score int, reason string) string {
	s := p.d.style
	var b strings.Builder
	b.WriteString("  " + mark + " ")
	cols := 4
	if world != "" {
		pad := max(worldColumn-utf8.RuneCountInString(world), 1)
		b.WriteString(world + strings.Repeat(" ", pad))
		cols += utf8.RuneCountInString(world) + pad
	}
	b.WriteString(status)
	cols += lipgloss.Width(status)
	if meter := s.Meter(score); meter != "" {
		pad := max(lipgloss.Width(s.Pill(false))-lipgloss.Width(status), 0)
		b.WriteString(strings.Repeat(" ", pad+2) + meter)
		cols += pad + 2 + lipgloss.Width(meter)
	}
	if reason = firstSentence(reason); reason != "" {
		room := p.d.width - cols - 3
		if room >= minReason {
			b.WriteString("  " + s.Dim(truncate(reason, room)))
		}
	}
	return b.String()
}

// write clears the live line, writes each line, and draws the live line again below them.
func (p *progressPrinter) write(lines ...string) {
	if len(lines) == 0 {
		return
	}
	p.clear()
	for _, line := range lines {
		fmt.Fprintln(p.w, line)
	}
	p.redraw()
}

// clear erases the live line when it is on screen.
func (p *progressPrinter) clear() {
	if p.drawn {
		fmt.Fprint(p.w, clearLine)
		p.drawn = false
	}
}

// redraw draws the live line for the calls in flight, or erases it when none are. It does
// nothing unless the display is live.
func (p *progressPrinter) redraw() {
	if !p.d.live || p.finished {
		return
	}
	label, since := p.liveLabel()
	if label == "" {
		p.clear()
		return
	}
	clock := " " + duration(p.now().Sub(since))
	room := p.d.width - 3 - utf8.RuneCountInString(clock)
	frame := spinFrames[p.frame%len(spinFrames)]
	s := p.d.style
	fmt.Fprint(p.w, clearLine+"  "+s.Accent(frame)+" "+truncate(label, room)+s.Dim(clock))
	p.drawn = true
}

// liveLabel returns the words for the calls in flight, e.g. "drafting cars, sports", and when
// the oldest of them started. It returns an empty label when nothing is in flight.
func (p *progressPrinter) liveLabel() (string, time.Time) {
	if len(p.active) == 0 {
		return "", time.Time{}
	}
	byStep := make(map[engine.Step][]string)
	var since time.Time
	for k, began := range p.active {
		byStep[k.step] = append(byStep[k.step], k.world)
		if since.IsZero() || began.Before(since) {
			since = began
		}
	}
	var parts []string
	for _, step := range []engine.Step{
		engine.StepSkeleton, engine.StepDraft, engine.StepCheck, engine.StepFinal,
		engine.StepRecheck,
	} {
		worlds, ok := byStep[step]
		if !ok {
			continue
		}
		sort.Strings(worlds)
		switch step {
		case engine.StepSkeleton:
			parts = append(parts, "reading the situation")
		case engine.StepDraft:
			parts = append(parts, "drafting "+strings.Join(worlds, ", "))
		case engine.StepCheck:
			parts = append(parts, "checking "+strings.Join(worlds, ", "))
		case engine.StepFinal:
			parts = append(parts, "polishing")
		case engine.StepRecheck:
			parts = append(parts, "checking the polish")
		}
	}
	return strings.Join(parts, " · "), since
}

// emoji returns e when the style is on and empty otherwise, so plain output has no emoji.
func (p *progressPrinter) emoji(e string) string {
	if p.d.style.On() {
		return e
	}
	return ""
}

// firstSentence returns the first sentence of s on one line, or all of s when it has one.
func firstSentence(s string) string {
	s = util.CollapseSpace(s)
	if i := strings.Index(s, ". "); i >= 0 {
		return s[:i+1]
	}
	return s
}

// truncate cuts s to at most n runes, ending in an ellipsis when anything was cut.
func truncate(s string, n int) string {
	if n < 1 {
		return ""
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimRight(string(r[:n-1]), " ,;:") + "…"
}

// duration returns d for a person, e.g. "9s" or "1m 32s".
func duration(d time.Duration) string {
	sec := int(d.Round(time.Second) / time.Second)
	if sec < 60 {
		return strconv.Itoa(sec) + "s"
	}
	return fmt.Sprintf("%dm %02ds", sec/60, sec%60)
}

// plural returns n and noun, with an s on noun unless n is 1.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

// isQuiet reports whether err is a quiet exit, such as a Ctrl-C, that needs no closing line.
func isQuiet(err error) bool {
	var q *QuietExitError
	return errors.As(err, &q)
}

// streamWidth returns the width in columns of v when it is a terminal, and 80 otherwise.
func streamWidth(v any) int {
	f, ok := v.(*os.File)
	if !ok {
		return 80
	}
	w, _, err := term.GetSize(int(f.Fd()))
	if err != nil || w < 1 {
		return 80
	}
	return w
}

// canRedraw reports whether v is a real file, where a live line may redraw in place. Writers
// that are not files, such as test buffers, get fixed lines only.
func canRedraw(v any) bool {
	_, ok := v.(*os.File)
	return ok
}
