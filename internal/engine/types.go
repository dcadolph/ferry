package engine

import (
	"context"
	"encoding/json"
)

// Step names one stage of the engine.
type Step string

// Steps of the engine, in the order they run. StepRecheck is the check step run once more, on
// the polished analogy.
const (
	StepSkeleton Step = "skeleton"
	StepDraft    Step = "draft"
	StepCheck    Step = "check"
	StepFinal    Step = "final"
	StepRecheck  Step = "recheck"
)

// Status says what happened in a progress event.
type Status string

// Statuses reported in progress events.
const (
	StatusStarted Status = "started"
	StatusDone    Status = "done"
	StatusKept    Status = "kept"
	StatusDropped Status = "dropped"
	StatusFailed  Status = "failed"
)

// Input holds one request to the engine.
type Input struct {
	// Summary is the situation in the speaker's own words.
	Summary string
	// Audience describes who will hear the analogy.
	Audience string
	// Worlds lists the speaker's preferred source worlds, most preferred first.
	Worlds []string
}

// Player is one party in the situation and the part it plays.
type Player struct {
	// Name is how the summary refers to the party.
	Name string `json:"name" desc:"How the summary refers to this party, kept generic."`
	// Role is what the party wants, does, or controls in the situation.
	Role string `json:"role" desc:"What this party wants, does, or controls."`
}

// Skeleton holds the structure pulled out of the summary.
type Skeleton struct {
	// Players lists the parties that act or are acted on.
	Players []Player `json:"players" desc:"The parties that act or are acted on."`
	// Constraint is the limit that makes the situation hard.
	Constraint string `json:"constraint" desc:"The limit that makes the situation hard."`
	// Tradeoff is what is gained and what is given up by each path.
	Tradeoff string `json:"tradeoff" desc:"What each path gains and what it gives up."`
	// CostOfInaction is what happens if nobody acts.
	CostOfInaction string `json:"cost_of_inaction" desc:"What happens if nobody acts."`
	// Decision is the choice the speaker wants the listener to make or see.
	Decision string `json:"decision" desc:"The choice the listener should make or see."`
	// Point is the one idea the speaker wants to land.
	Point string `json:"point" desc:"The one idea the speaker wants to land."`
	// Chain lists the cause-and-effect steps of the situation, in order.
	Chain []string `json:"causal_chain" desc:"The cause-and-effect steps, in order."`
}

// Mapping pairs one element of the analogy with the real thing it stands for.
type Mapping struct {
	// Analogy is the element as it appears in the analogy.
	Analogy string `json:"analogy" desc:"The element as it appears in the analogy."`
	// Real is the part of the real situation the element stands for.
	Real string `json:"real" desc:"The part of the real situation it stands for."`
}

// Candidate is one drafted analogy from a single world. The model writes the fields of a reply
// in the order they are declared, so the spoken text comes before the parts drawn from it.
type Candidate struct {
	// World is the familiar world the analogy draws from.
	World string `json:"world" desc:"The familiar world the analogy draws from."`
	// Spoken is the analogy as two or three sentences said out loud.
	Spoken string `json:"spoken" desc:"Two or three sentences a person can say out loud."`
	// DecisionLine is the part of the analogy that points at the decision.
	DecisionLine string `json:"decision_line" desc:"The words that point at the decision."`
	// Mappings lists what stands for what.
	Mappings []Mapping `json:"mappings" desc:"What in the analogy stands for what in reality."`
	// BreaksDown is where the analogy stops matching the real situation.
	BreaksDown string `json:"breaks_down" desc:"Where the analogy stops matching reality."`
}

// Verdict is the check of one candidate against the skeleton. The model writes the fields of a
// reply in the order they are declared, so the problems come first and the score and the fix,
// which depend on them, come last.
type Verdict struct {
	// Problems lists each way the analogy fails or misleads.
	Problems []string `json:"problems" desc:"Each way the analogy fails or misleads."`
	// SameShape reports whether the analogy has the real cause-and-effect shape.
	SameShape bool `json:"same_shape" desc:"True only if the cause-and-effect shape matches."`
	// FamiliarWorld reports whether the audience already knows the world.
	FamiliarWorld bool `json:"familiar_world" desc:"True only if the audience knows this world."`
	// PointsAtDecision reports whether the analogy points at the decision.
	PointsAtDecision bool `json:"points_at_decision" desc:"True only if it points at the decision."`
	// Fit is the fit score from 1, would mislead, to 5, could not be broken.
	Fit int `json:"fit" desc:"Fit from 1 (would mislead) to 5 (could not be broken)."`
	// Fix is the single change that would most improve the analogy.
	Fix string `json:"fix" desc:"The single change that would most improve it."`
}

// Checked pairs a candidate with its verdict and whether it was kept.
type Checked struct {
	// Candidate is the drafted analogy.
	Candidate Candidate `json:"candidate"`
	// Verdict is the check of the candidate against the skeleton.
	Verdict Verdict `json:"verdict"`
	// Kept reports whether the candidate passed every required check.
	Kept bool `json:"kept"`
	// Round is the drafting round the candidate came from, starting at 1.
	Round int `json:"round"`
}

// Analogy is the final, polished result.
type Analogy struct {
	// World is the familiar world the analogy draws from.
	World string `json:"world" desc:"The familiar world the analogy draws from."`
	// Spoken is the analogy as two or three sentences said out loud.
	Spoken string `json:"spoken" desc:"Two or three sentences a person can say out loud."`
	// Mappings lists what stands for what.
	Mappings []Mapping `json:"mappings" desc:"What in the analogy stands for what in reality."`
	// BreaksDown is where the analogy stops matching the real situation.
	BreaksDown string `json:"breaks_down" desc:"Where the analogy stops matching reality."`
}

// Result holds the winning analogy and the work behind it.
type Result struct {
	// Analogy is the winning analogy. It is the polished text when that passed its own check,
	// and the checked draft otherwise.
	Analogy Analogy `json:"analogy"`
	// Note says, in words for the user, when the analogy is not a polished one that met every
	// rule, e.g. because the final step failed and the checked draft stands in. It is empty
	// for a clean run.
	Note string `json:"note,omitempty"`
	// Skeleton is the structure pulled out of the summary.
	Skeleton Skeleton `json:"skeleton"`
	// Checked lists every candidate drafted, with its verdict.
	Checked []Checked `json:"checked"`
	// Recheck is the check of the polished analogy, or nil when the run never got that far.
	// Its candidate holds the polished text with the winner's decision line. When it was not
	// kept, Analogy is the checked draft.
	Recheck *Checked `json:"recheck,omitempty"`
	// Rounds is the number of drafting rounds that ran.
	Rounds int `json:"rounds"`
}

// Request is one structured call to the model.
type Request struct {
	// Step is the engine stage making the call.
	Step Step
	// System holds the standing instructions for the call.
	System string
	// Prompt holds the task for the call.
	Prompt string
	// Schema is the JSON schema the reply must match, with its properties in the order the
	// model should write them.
	Schema json.RawMessage
}

// Completer sends one request to a language model and returns the raw JSON text of its reply.
type Completer interface {
	// Complete sends req and returns the reply text, which must match req.Schema.
	Complete(ctx context.Context, req Request) (string, error)
}

// CompleterFunc adapts a function to the Completer interface.
type CompleterFunc func(ctx context.Context, req Request) (string, error)

// Complete calls f(ctx, req).
func (f CompleterFunc) Complete(ctx context.Context, req Request) (string, error) {
	return f(ctx, req)
}

// Event reports one unit of progress from the engine.
type Event struct {
	// Step is the engine stage the event belongs to.
	Step Step
	// Status says what happened.
	Status Status
	// World names the world a draft or check belongs to, when there is one.
	World string
	// Round is the drafting round, starting at 1, or 0 outside drafting and checking.
	Round int
	// Fit is the checker's fit score on a kept or dropped event, and 0 on every other event.
	Fit int
	// Detail carries a short note, such as the reason a draft was dropped.
	Detail string
}

// ProgressFunc receives engine events. It must be safe for concurrent use.
type ProgressFunc func(Event)

// Config holds the engine's tunable settings.
type Config struct {
	// Worlds lists the default worlds drafted on the first round.
	Worlds []string
	// FallbackWorlds lists the worlds drafted when no earlier draft holds up.
	FallbackWorlds []string
	// MaxWorlds caps how many worlds one round drafts. Preferred worlds fill the round first,
	// and the defaults take what room is left.
	MaxWorlds int
	// Rounds caps the number of drafting rounds.
	Rounds int
	// Workers caps how many model calls run at once.
	Workers int
	// MinFit is the lowest fit score a kept candidate may have.
	MinFit int
	// Progress receives progress events. Nil means no reporting.
	Progress ProgressFunc
}
