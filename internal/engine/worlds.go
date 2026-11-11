package engine

import (
	"maps"
	"strings"

	"github.com/dcadolph/ferry/internal/util"
)

// worldKey returns the form of w used to compare worlds without regard to case or spacing.
func worldKey(w string) string {
	return strings.ToLower(util.CollapseSpace(w))
}

// uniqueWorlds returns worlds trimmed with the spaces inside them collapsed, without blanks,
// and without any world that is already in seen or earlier in the list, compared without regard
// to case. It adds each world it keeps to seen.
func uniqueWorlds(worlds []string, seen map[string]bool) []string {
	var out []string
	for _, w := range worlds {
		w = util.CollapseSpace(w)
		key := worldKey(w)
		if w == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, w)
	}
	return out
}

// DistinctWorlds returns worlds cleaned, in order, without blanks or repeats. Worlds that differ
// only in case or spacing count as one.
func DistinctWorlds(worlds []string) []string {
	return uniqueWorlds(worlds, map[string]bool{})
}

// roundOneWorlds returns the worlds for the first round: the preferred worlds in order, then the
// defaults, up to limit worlds in all. Preferred worlds past limit are left out, since every
// world costs a draft call and a check call.
func roundOneWorlds(preferred, defaults []string, limit int) []string {
	seen := map[string]bool{}
	worlds := uniqueWorlds(preferred, seen)
	worlds = worlds[:min(len(worlds), limit)]
	for _, w := range uniqueWorlds(defaults, seen) {
		if len(worlds) >= limit {
			break
		}
		worlds = append(worlds, w)
	}
	return worlds
}

// nextWorlds returns up to limit fallback worlds that no earlier round used.
func nextWorlds(fallbacks []string, used map[string]bool, limit int) []string {
	worlds := uniqueWorlds(fallbacks, maps.Clone(used))
	return worlds[:min(len(worlds), limit)]
}
