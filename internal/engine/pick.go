package engine

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/dcadolph/ferry/internal/util"
)

// pick returns the index of the winning kept candidate in checked, or -1 when none was kept.
// A preferred world beats any other, earlier preferred worlds first. After that the higher fit
// wins, then the earlier round, then the earlier draft.
func pick(checked []Checked, preferred []string) int {
	rank := make(map[string]int, len(preferred))
	for i, w := range preferred {
		rank[worldKey(w)] = i
	}
	preference := func(c Checked) int {
		if i, ok := rank[worldKey(c.Candidate.World)]; ok {
			return i
		}
		return len(preferred)
	}
	var kept []int
	for i, c := range checked {
		if c.Kept {
			kept = append(kept, i)
		}
	}
	if len(kept) == 0 {
		return -1
	}
	return slices.MinFunc(kept, func(a, b int) int {
		ca, cb := checked[a], checked[b]
		return cmp.Or(
			cmp.Compare(preference(ca), preference(cb)),
			cmp.Compare(cb.Verdict.Fit, ca.Verdict.Fit),
			cmp.Compare(ca.Round, cb.Round),
			cmp.Compare(a, b),
		)
	})
}

// closest returns the index of the checked candidate that came nearest to being kept, or -1
// when no check returned a verdict. It counts the checks each candidate passed, then compares
// fit, then draft order. A check that failed leaves the fit at zero and never counts.
func closest(checked []Checked, minFit int) int {
	var rated []int
	for i, c := range checked {
		if c.Verdict.Fit >= lowestFit {
			rated = append(rated, i)
		}
	}
	if len(rated) == 0 {
		return -1
	}
	return slices.MinFunc(rated, func(a, b int) int {
		va, vb := checked[a].Verdict, checked[b].Verdict
		return cmp.Or(
			cmp.Compare(passed(vb, minFit), passed(va, minFit)),
			cmp.Compare(vb.Fit, va.Fit),
			cmp.Compare(a, b),
		)
	})
}

// passed counts the checks v passes.
func passed(v Verdict, minFit int) int {
	n := 0
	for _, ok := range []bool{v.SameShape, v.FamiliarWorld, v.PointsAtDecision, v.Fit >= minFit} {
		if ok {
			n++
		}
	}
	return n
}

// firstProblem returns the first problem the checker named or, when it named none, the first
// check v fails. It returns "" for a verdict with nothing wrong.
func firstProblem(v Verdict, minFit int) string {
	switch {
	case len(v.Problems) > 0:
		return v.Problems[0]
	case !v.SameShape:
		return "the cause and effect do not match the real situation"
	case !v.FamiliarWorld:
		return "the audience may not know this world"
	case !v.PointsAtDecision:
		return "it does not point at the decision"
	case v.Fit < minFit:
		return fmt.Sprintf("fit %d is below the minimum of %d", v.Fit, minFit)
	default:
		return ""
	}
}

// droppedReasons returns one line for each candidate in checked that got a verdict and was
// dropped: its world, its spoken text on one line, and its first problem. The spoken text is
// there because the problem names parts of an analogy a later draft has never seen.
func droppedReasons(checked []Checked, minFit int) []string {
	var reasons []string
	for _, c := range checked {
		if c.Kept || c.Verdict.Fit < lowestFit {
			continue
		}
		reasons = append(reasons, fmt.Sprintf(`%s: "%s" was dropped because %s`,
			c.Candidate.World, util.CollapseSpace(c.Candidate.Spoken),
			firstProblem(c.Verdict, minFit)))
	}
	return reasons
}

// noFit returns ErrNoFit naming the candidate that came closest and what held it back.
func noFit(checked []Checked, minFit int) error {
	i := closest(checked, minFit)
	if i < 0 {
		return fmt.Errorf("%w: no check returned a verdict", ErrNoFit)
	}
	c := checked[i]
	return fmt.Errorf("%w: closest was %s at fit %d: %s",
		ErrNoFit, c.Candidate.World, c.Verdict.Fit, firstProblem(c.Verdict, minFit))
}
