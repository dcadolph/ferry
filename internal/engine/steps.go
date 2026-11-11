package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// pullSkeleton runs the skeleton step.
func (r *run) pullSkeleton(ctx context.Context) (Skeleton, error) {
	r.emit(Event{Step: StepSkeleton, Status: StatusStarted})
	sk, err := ask(ctx, r, StepSkeleton, r.data(), cleanSkeleton)
	if err != nil {
		r.emit(Event{Step: StepSkeleton, Status: StatusFailed, Detail: err.Error()})
		return Skeleton{}, err
	}
	r.emit(Event{Step: StepSkeleton, Status: StatusDone})
	return sk, nil
}

// draftAndCheck runs drafting rounds until one keeps a candidate or the rounds or unused
// fallback worlds run out, adding every checked candidate to res.
func (r *run) draftAndCheck(ctx context.Context, res *Result) error {
	cfg := r.e.cfg
	used := map[string]bool{}
	var dropped []string
	worlds := roundOneWorlds(r.preferred, cfg.Worlds, cfg.MaxWorlds)
	for round := 1; round <= cfg.Rounds && len(worlds) > 0; round++ {
		for _, w := range worlds {
			used[worldKey(w)] = true
		}
		res.Rounds = round
		checked, err := r.runRound(ctx, round, worlds, dropped)
		res.Checked = append(res.Checked, checked...)
		if err != nil {
			return err
		}
		if slices.ContainsFunc(checked, func(c Checked) bool { return c.Kept }) {
			return nil
		}
		dropped = append(dropped, droppedReasons(checked, cfg.MinFit)...)
		worlds = nextWorlds(cfg.FallbackWorlds, used, cfg.MaxWorlds)
	}
	return nil
}

// runRound drafts one candidate per world, then checks each draft. It fails when every draft
// or every check in the round fails.
func (r *run) runRound(ctx context.Context, round int, worlds, dropped []string) ([]Checked, error) {
	drafts, err := r.draftAll(ctx, round, worlds, dropped)
	if err != nil {
		return nil, err
	}
	return r.checkAll(ctx, round, drafts)
}

// draftAll drafts one candidate per world at once and returns the drafts that succeeded, in
// world order. A failed draft is skipped, unless every draft fails.
func (r *run) draftAll(ctx context.Context, round int, worlds, dropped []string) ([]Candidate, error) {
	drafts := make([]Candidate, len(worlds))
	errs := make([]error, len(worlds))
	each(ctx, len(worlds), r.e.cfg.Workers, func(i int) {
		drafts[i], errs[i] = r.draft(ctx, round, worlds[i], dropped)
	})
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("draft step: %w", err)
	}
	var ok []Candidate
	var failed []error
	for i, err := range errs {
		if err != nil {
			failed = append(failed, fmt.Errorf("%s: %w", worlds[i], err))
			continue
		}
		ok = append(ok, drafts[i])
	}
	if len(ok) == 0 {
		return nil, fmt.Errorf("every draft in round %d failed: %w", round, errors.Join(failed...))
	}
	return ok, nil
}

// draft runs the draft step for one world.
func (r *run) draft(ctx context.Context, round int, world string, dropped []string) (Candidate, error) {
	r.emit(Event{Step: StepDraft, Status: StatusStarted, World: world, Round: round})
	data := r.data()
	data.World, data.Dropped = world, dropped
	c, err := ask(ctx, r, StepDraft, data, func(c *Candidate) string {
		return cleanCandidate(c, world)
	})
	if err != nil {
		r.emit(Event{
			Step: StepDraft, Status: StatusFailed, World: world, Round: round, Detail: err.Error(),
		})
		return Candidate{}, err
	}
	r.emit(Event{Step: StepDraft, Status: StatusDone, World: world, Round: round})
	return c, nil
}

// checkAll checks every draft at once and returns the results in draft order. A failed check
// leaves its candidate unkept, unless every check fails.
func (r *run) checkAll(ctx context.Context, round int, drafts []Candidate) ([]Checked, error) {
	checked := make([]Checked, len(drafts))
	errs := make([]error, len(drafts))
	each(ctx, len(drafts), r.e.cfg.Workers, func(i int) {
		checked[i], errs[i] = r.check(ctx, StepCheck, round, drafts[i])
	})
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("check step: %w", err)
	}
	if !slices.Contains(errs, nil) {
		failed := make([]error, len(errs))
		for i, err := range errs {
			failed[i] = fmt.Errorf("%s: %w", drafts[i].World, err)
		}
		err := errors.Join(failed...)
		return checked, fmt.Errorf("every check in round %d failed: %w", round, err)
	}
	return checked, nil
}

// check runs a check of one candidate, which the checker sees on its own. Step is StepCheck
// for a draft and StepRecheck for the polished analogy. A check that fails leaves the fit at
// zero and names the failure as the only problem.
func (r *run) check(ctx context.Context, step Step, round int, c Candidate) (Checked, error) {
	out := Checked{Candidate: c, Round: round}
	r.emit(Event{Step: step, Status: StatusStarted, World: c.World, Round: round})
	data := r.data()
	data.Candidate = c
	v, err := ask(ctx, r, step, data, cleanVerdict)
	if err != nil {
		out.Verdict.Problems = []string{"no verdict: " + err.Error()}
		r.emit(Event{
			Step: step, Status: StatusFailed, World: c.World, Round: round, Detail: err.Error(),
		})
		return out, err
	}
	out.Verdict = v
	out.Kept = keeps(v, r.e.cfg.MinFit)
	status := StatusDropped
	if out.Kept {
		status = StatusKept
	}
	r.emit(Event{
		Step: step, Status: status, World: c.World, Round: round, Fit: v.Fit,
		Detail: firstProblem(v, r.e.cfg.MinFit),
	})
	return out, nil
}

// keeps reports whether v passes the keep rule: the same shape, a familiar world, a pointer at
// the decision, and a fit of at least minFit.
func keeps(v Verdict, minFit int) bool {
	return v.SameShape && v.FamiliarWorld && v.PointsAtDecision && v.Fit >= minFit
}

// finish runs the final step on the winning candidate, then checks the polished text the way
// the drafts were checked, and fills in res.Analogy, res.Recheck, and res.Note. The polished
// text is the result only when it passes that check. When the polish fails, the re-check call
// fails, or the polished text does not pass, the result is the winning draft as it was
// checked, and the note says which happened. Only a context that is done ends in an error.
func (r *run) finish(ctx context.Context, win Checked, res *Result) error {
	world := win.Candidate.World
	r.emit(Event{Step: StepFinal, Status: StatusStarted, World: world})
	polished, note, err := r.polish(ctx, win)
	if err != nil {
		r.emit(Event{Step: StepFinal, Status: StatusFailed, World: world, Detail: err.Error()})
		if ctx.Err() != nil {
			return err
		}
		res.Analogy = draftAnalogy(win.Candidate)
		res.Note = "polish failed, this is the checked draft: " + err.Error()
		return nil
	}
	r.emit(Event{Step: StepFinal, Status: StatusDone, World: world, Detail: note})
	recheck, err := r.check(ctx, StepRecheck, win.Round, polishedCandidate(polished, win.Candidate))
	if err != nil && ctx.Err() != nil {
		return err
	}
	res.Recheck = &recheck
	switch {
	case err != nil:
		res.Analogy = draftAnalogy(win.Candidate)
		res.Note = "the check of the polished analogy failed, this is the checked draft: " +
			err.Error()
	case !recheck.Kept:
		res.Analogy = draftAnalogy(win.Candidate)
		res.Note = "the polished analogy did not pass its check, this is the checked draft: " +
			firstProblem(recheck.Verdict, r.e.cfg.MinFit)
	default:
		res.Analogy, res.Note = polished, note
	}
	return nil
}

// draftAnalogy returns the checked draft c as a final analogy, word for word.
func draftAnalogy(c Candidate) Analogy {
	return Analogy{World: c.World, Spoken: c.Spoken, Mappings: c.Mappings, BreaksDown: c.BreaksDown}
}

// polishedCandidate returns the polished analogy a in the shape the check step judges. The
// final step writes no decision line, so the one from the draft it polished stands in.
func polishedCandidate(a Analogy, draft Candidate) Candidate {
	return Candidate{
		World:        a.World,
		Spoken:       a.Spoken,
		DecisionLine: draft.DecisionLine,
		Mappings:     a.Mappings,
		BreaksDown:   a.BreaksDown,
	}
}

// polish asks for the final analogy and validates it, with one corrective retry that names
// any problem. A reply with nothing missing and the wrong sentence count is usable: polish
// asks once more for the right length, and when that retry is no better, or the retry call
// itself fails, it keeps the usable reply and returns a note saying so. A required part still
// missing after the retry is ErrIncomplete, and a reply that never decodes is ErrBadReply.
func (r *run) polish(ctx context.Context, win Checked) (Analogy, string, error) {
	data := r.data()
	data.Candidate, data.Verdict = win.Candidate, win.Verdict
	var (
		usable  Analogy
		note    string
		missing []string
	)
	found, decoded := false, false
	for range attempts {
		var a Analogy
		problem, err := r.send(ctx, StepFinal, data, &a)
		if err != nil {
			if found && ctx.Err() == nil {
				break
			}
			return Analogy{}, "", err
		}
		if problem != "" {
			data.Problem = problem
			continue
		}
		cleanAnalogy(&a, win.Candidate.World)
		var length string
		missing, length = analogyProblems(a)
		if len(missing) == 0 && length == "" {
			return a, "", nil
		}
		decoded = true
		problems := slices.Clone(missing)
		if length != "" {
			problems = append(problems, length)
		}
		if len(missing) == 0 {
			usable, note, found = a, "kept as is: "+length, true
		}
		data.Problem = strings.Join(problems, "; ")
	}
	switch {
	case found:
		return usable, note, nil
	case decoded:
		return Analogy{}, "", fmt.Errorf("%w: final step: %s",
			ErrIncomplete, strings.Join(missing, "; "))
	default:
		return Analogy{}, "", fmt.Errorf("%w: final step: %s", ErrBadReply, data.Problem)
	}
}
