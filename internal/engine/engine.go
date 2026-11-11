// Package engine turns a situation summary into a checked analogy that can be said out loud.
package engine

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// DefaultAudience is the audience assumed when the input names none.
const DefaultAudience = "smart listeners who do not know the details"

// reservedTag matches the opening or closing form of a tag the prompts use to mark where the
// speaker's text and the work of earlier steps begin and end.
var reservedTag = regexp.MustCompile(
	`(?i)<\s*/?\s*(summary|audience|world|skeleton|analogy|check|dropped)\s*>`)

// Engine runs the four steps, skeleton, draft, check, and final, then checks the final text
// once more.
type Engine struct {
	// completer sends each step's request to the model.
	completer Completer
	// cfg holds the engine's settings.
	cfg Config
}

// DefaultConfig returns the engine's default settings.
func DefaultConfig() Config {
	return Config{
		Worlds:         []string{"cars", "kitchens", "construction", "sports"},
		FallbackWorlds: []string{"home plumbing", "gardening", "air travel", "household budgets"},
		MaxWorlds:      6,
		Rounds:         2,
		Workers:        4,
		MinFit:         3,
	}
}

// New returns an Engine with the default settings. It panics when c is nil.
func New(c Completer) *Engine {
	return NewWithConfig(c, DefaultConfig())
}

// NewWithConfig returns an Engine with the given settings. Empty world lists and numbers below
// one take their values from DefaultConfig. It panics when c is nil.
func NewWithConfig(c Completer, cfg Config) *Engine {
	if f, ok := c.(CompleterFunc); c == nil || (ok && f == nil) {
		panic("engine.NewWithConfig: Completer required")
	}
	def := DefaultConfig()
	if len(cfg.Worlds) == 0 {
		cfg.Worlds = def.Worlds
	}
	if len(cfg.FallbackWorlds) == 0 {
		cfg.FallbackWorlds = def.FallbackWorlds
	}
	cfg.MaxWorlds = orDefault(cfg.MaxWorlds, def.MaxWorlds)
	cfg.Rounds = orDefault(cfg.Rounds, def.Rounds)
	cfg.Workers = orDefault(cfg.Workers, def.Workers)
	cfg.MinFit = orDefault(cfg.MinFit, def.MinFit)
	return &Engine{completer: c, cfg: cfg}
}

// orDefault returns n, or def when n is below one.
func orDefault(n, def int) int {
	if n < 1 {
		return def
	}
	return n
}

// Run turns in into a checked analogy. It pulls the skeleton out of the summary, drafts
// candidates from several worlds at once, checks each one against the skeleton on its own,
// picks the winner in code, polishes the winner for speaking, and checks the polished text.
// Every analogy Run returns has passed the check: when the polish fails, or the polished text
// does not pass, the result is the winning draft as it was checked, and Result.Note says so.
// Once the skeleton step has passed, Run returns the work done so far along with any error, so
// a caller can still show the candidates behind ErrNoFit.
func (e *Engine) Run(ctx context.Context, in Input) (Result, error) {
	r, err := e.start(in)
	if err != nil {
		return Result{}, err
	}
	sk, err := r.pullSkeleton(ctx)
	if err != nil {
		return Result{}, err
	}
	r.skeleton = sk
	res := Result{Skeleton: sk}
	if err = r.draftAndCheck(ctx, &res); err != nil {
		return res, err
	}
	win := pick(res.Checked, r.preferred)
	if win < 0 {
		return res, noFit(res.Checked, e.cfg.MinFit)
	}
	if err = r.finish(ctx, res.Checked[win], &res); err != nil {
		return res, err
	}
	return res, nil
}

// start validates in and returns the state for one run. It rejects a summary, audience, or
// world that holds one of the prompts' own tags, because that text could close its tag early
// and sit where the model reads instructions. The text is rejected, never rewritten.
func (e *Engine) start(in Input) (*run, error) {
	summary := strings.TrimSpace(in.Summary)
	if summary == "" {
		return nil, ErrNoSummary
	}
	audience := strings.TrimSpace(in.Audience)
	if err := checkReserved("summary", summary); err != nil {
		return nil, err
	}
	if err := checkReserved("audience", audience); err != nil {
		return nil, err
	}
	for _, w := range in.Worlds {
		if err := checkReserved("world", w); err != nil {
			return nil, err
		}
	}
	if audience == "" {
		audience = DefaultAudience
	}
	system, err := render(systemTemplate, promptData{})
	if err != nil {
		return nil, err
	}
	return &run{
		e:         e,
		system:    system,
		summary:   summary,
		audience:  audience,
		preferred: DistinctWorlds(in.Worlds),
	}, nil
}

// checkReserved returns ErrReservedTag when text holds a tag the prompts use. Part names the
// input for the message: summary, audience, or world. The message names the tag and never
// quotes text.
func checkReserved(part, text string) error {
	m := reservedTag.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	return fmt.Errorf("%w: the %s holds the tag %q, which ferry uses to mark its own text: "+
		"remove it and run again", ErrReservedTag, part, strings.ToLower(m[1]))
}

// run holds the state of one call to Run.
type run struct {
	// e is the engine doing the run.
	e *Engine
	// system is the rendered system prompt that every request carries.
	system string
	// summary is the trimmed situation summary.
	summary string
	// audience describes who will hear the analogy.
	audience string
	// preferred lists the speaker's preferred worlds, cleaned, in the order given.
	preferred []string
	// skeleton is the structure pulled from the summary, set once the skeleton step passes.
	skeleton Skeleton
}

// emit sends ev to the progress function, when there is one.
func (r *run) emit(ev Event) {
	if r.e.cfg.Progress != nil {
		r.e.cfg.Progress(ev)
	}
}

// data returns the prompt data that every step shares.
func (r *run) data() promptData {
	return promptData{Summary: r.summary, Audience: r.audience, Skeleton: r.skeleton}
}
