package engine

import (
	"fmt"
	"strings"
)

// Bounds on what a usable reply holds.
const (
	// lowestFit and highestFit bound the checker's fit score.
	lowestFit, highestFit = 1, 5
	// fewestSentences and mostSentences bound the final spoken analogy.
	fewestSentences, mostSentences = 2, 3
	// fewestMappings is the shortest usable mapping list.
	fewestMappings = 2
	// fewestLinks is the shortest usable causal chain.
	fewestLinks = 2
)

// cleanSkeleton trims s, drops blank entries, and returns what is wrong with it, or "" when it
// is usable.
func cleanSkeleton(s *Skeleton) string {
	s.Constraint = strings.TrimSpace(s.Constraint)
	s.Tradeoff = strings.TrimSpace(s.Tradeoff)
	s.CostOfInaction = strings.TrimSpace(s.CostOfInaction)
	s.Decision = strings.TrimSpace(s.Decision)
	s.Point = strings.TrimSpace(s.Point)
	s.Chain = trimAll(s.Chain)
	players := make([]Player, 0, len(s.Players))
	for _, p := range s.Players {
		p.Name, p.Role = strings.TrimSpace(p.Name), strings.TrimSpace(p.Role)
		if p.Name != "" {
			players = append(players, p)
		}
	}
	s.Players = players

	var problems []string
	if s.Decision == "" {
		problems = append(problems, "decision is empty")
	}
	if s.Point == "" {
		problems = append(problems, "point is empty")
	}
	if s.CostOfInaction == "" {
		problems = append(problems, "cost_of_inaction is empty")
	}
	if len(s.Players) == 0 {
		problems = append(problems, "players needs at least one entry with a name")
	}
	if len(s.Chain) < fewestLinks {
		problems = append(problems,
			fmt.Sprintf("causal_chain needs at least %d links, got %d", fewestLinks, len(s.Chain)))
	}
	return strings.Join(problems, "; ")
}

// cleanCandidate trims c, sets its world to world, and returns what is wrong with it, or ""
// when it is usable.
func cleanCandidate(c *Candidate, world string) string {
	c.World = world
	c.Spoken = strings.TrimSpace(c.Spoken)
	c.DecisionLine = strings.TrimSpace(c.DecisionLine)
	c.BreaksDown = strings.TrimSpace(c.BreaksDown)
	c.Mappings = cleanMappings(c.Mappings)

	var problems []string
	if c.Spoken == "" {
		problems = append(problems, "spoken is empty")
	}
	if len(c.Mappings) < fewestMappings {
		problems = append(problems, mappingsProblem(len(c.Mappings)))
	}
	if c.DecisionLine == "" {
		problems = append(problems, "decision_line is empty")
	}
	if c.BreaksDown == "" {
		problems = append(problems, "breaks_down is empty")
	}
	return strings.Join(problems, "; ")
}

// cleanVerdict trims v and clamps its fit to the 1 to 5 scale. A verdict is always usable, so
// it returns "".
func cleanVerdict(v *Verdict) string {
	v.Fit = min(max(v.Fit, lowestFit), highestFit)
	v.Problems = trimAll(v.Problems)
	v.Fix = strings.TrimSpace(v.Fix)
	return ""
}

// cleanAnalogy trims a and sets its world to world.
func cleanAnalogy(a *Analogy, world string) {
	a.World = world
	a.Spoken = strings.TrimSpace(a.Spoken)
	a.BreaksDown = strings.TrimSpace(a.BreaksDown)
	a.Mappings = cleanMappings(a.Mappings)
}

// analogyProblems returns the required parts a lacks and, separately, a note when its spoken
// line does not run two or three sentences.
func analogyProblems(a Analogy) (missing []string, length string) {
	if a.Spoken == "" {
		missing = append(missing, "spoken is empty")
	}
	if len(a.Mappings) < fewestMappings {
		missing = append(missing, mappingsProblem(len(a.Mappings)))
	}
	if a.BreaksDown == "" {
		missing = append(missing, "breaks_down is empty")
	}
	if a.Spoken == "" {
		return missing, ""
	}
	n := sentenceCount(a.Spoken)
	if n >= fewestSentences && n <= mostSentences {
		return missing, ""
	}
	if n == 1 {
		return missing, "spoken has 1 sentence, and it needs two or three"
	}
	return missing, fmt.Sprintf("spoken has %d sentences, and it needs two or three", n)
}

// mappingsProblem describes a mapping list with only n complete entries.
func mappingsProblem(n int) string {
	return fmt.Sprintf("mappings needs at least %d complete entries, got %d", fewestMappings, n)
}

// cleanMappings returns ms trimmed, without any entry missing either side.
func cleanMappings(ms []Mapping) []Mapping {
	out := make([]Mapping, 0, len(ms))
	for _, m := range ms {
		m.Analogy, m.Real = strings.TrimSpace(m.Analogy), strings.TrimSpace(m.Real)
		if m.Analogy != "" && m.Real != "" {
			out = append(out, m)
		}
	}
	return out
}

// trimAll returns xs trimmed, without blank entries.
func trimAll(xs []string) []string {
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}
