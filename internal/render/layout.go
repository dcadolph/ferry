package render

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dcadolph/ferry/internal/engine"
)

// Layout widths, in columns.
const (
	// DefaultWidth is the width used when the output is not a terminal.
	DefaultWidth = 80
	// MaxWidth caps the width so prose stays readable in a wide terminal.
	MaxWidth = 100
	// minWidth is the narrowest width text is wrapped to, whatever the terminal says.
	minWidth = 40
	// pad is the indent of the items under a heading.
	pad = "  "
	// subPad is the indent of the lines under a subheading.
	subPad = "    "
	// labelWidth fits the longest skeleton label, "Cost of inaction", and a two-space gap.
	labelWidth = 18
	// none is the placeholder for an empty list or field.
	none = "(none)"
)

// Width returns cols clamped to the range ferry wraps text in. A width under 1 means the width
// is unknown and gets DefaultWidth.
func Width(cols int) int {
	switch {
	case cols < 1:
		return DefaultWidth
	case cols < minWidth:
		return minWidth
	case cols > MaxWidth:
		return MaxWidth
	}
	return cols
}

// Hang wraps text to cols with prefix in front of the first line and the later lines indented
// to line up under the first line's text. An empty text returns the prefix with its trailing
// spaces cut.
func Hang(prefix, text string, cols int) string {
	if text == "" {
		return strings.TrimRight(prefix, " ")
	}
	width := utf8.RuneCountInString(prefix)
	room := max(cols-width, minWidth/2)
	lines := strings.Split(wrap(text, room), "\n")
	indent := strings.Repeat(" ", width)
	for i := range lines {
		if i == 0 {
			lines[i] = prefix + lines[i]
			continue
		}
		if lines[i] != "" {
			lines[i] = indent + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

// padRight returns s with spaces after it to make it n runes wide.
func padRight(s string, n int) string {
	if gap := n - utf8.RuneCountInString(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

// layout lays out the plain work section in columns for one width.
type layout struct {
	// cols is the width lines are wrapped to.
	cols int
}

// field returns one skeleton field: an indented label in a fixed column and its text wrapped
// in the column beside it.
func (l layout) field(label, text string) string {
	gap := strings.Repeat(" ", labelWidth-utf8.RuneCountInString(label))
	return Hang(pad+label+gap, text, l.cols)
}

// players returns the players as two columns, names and roles. The name column fits the
// longest name that is no wider than a third of the width. A longer name gets a line of its
// own with its role on the line below, in the role column.
func (l layout) players(ps []engine.Player) string {
	if len(ps) == 0 {
		return subPad + none
	}
	limit, nameCol := l.cols/3, 0
	for _, p := range ps {
		if n := utf8.RuneCountInString(p.Name); n <= limit {
			nameCol = max(nameCol, n)
		}
	}
	width := len(subPad) + nameCol + 2
	lines := make([]string, 0, len(ps))
	for _, p := range ps {
		if utf8.RuneCountInString(p.Name) > nameCol {
			role := Hang(strings.Repeat(" ", width), p.Role, l.cols)
			lines = append(lines, subPad+p.Name, role)
			continue
		}
		lines = append(lines, Hang(subPad+padRight(p.Name, nameCol+2), p.Role, l.cols))
	}
	return strings.Join(lines, "\n")
}

// steps returns the cause and effect steps as a numbered list, each step wrapped under its own
// text.
func (l layout) steps(chain []string) string {
	if len(chain) == 0 {
		return subPad + none
	}
	digits := len(strconv.Itoa(len(chain)))
	lines := make([]string, len(chain))
	for i, step := range chain {
		num := padRight(strconv.Itoa(i+1)+".", digits+1) + " "
		lines[i] = Hang(subPad+num, step, l.cols)
	}
	return strings.Join(lines, "\n")
}

// bullets returns items as a dashed list, each item wrapped under its own text.
func (l layout) bullets(items []string) string {
	lines := make([]string, len(items))
	for i, item := range items {
		lines[i] = Hang(subPad+"- ", item, l.cols)
	}
	return strings.Join(lines, "\n")
}

// table returns one row per candidate with its world, round, verdict, and fit, under a header.
// The round column is left out when no candidate has a round.
func (l layout) table(checked []engine.Checked) string {
	worldCol, rounds := len("World"), false
	for _, c := range checked {
		worldCol = max(worldCol, utf8.RuneCountInString(c.Candidate.World))
		rounds = rounds || c.Round > 0
	}
	const verdictCol = len("dropped")
	header := pad + padRight("World", worldCol) + "  "
	if rounds {
		header += "Round  "
	}
	header += padRight("Verdict", verdictCol) + "  Fit"
	lines := []string{header}
	for _, c := range checked {
		row := pad + padRight(c.Candidate.World, worldCol) + "  "
		if rounds {
			row += padRight(roundText(c.Round), len("Round")) + "  "
		}
		row += padRight(verdictWord(c.Kept), verdictCol) + "  " + fitText(c.Verdict.Fit)
		lines = append(lines, row)
	}
	return strings.Join(lines, "\n")
}

// rule returns a divider line that names a checked candidate: its world, its round when it has
// one, its verdict, and its fit, followed by a line to the width.
func (l layout) rule(c engine.Checked) string {
	parts := []string{c.Candidate.World}
	if c.Round > 0 {
		parts = append(parts, "round "+strconv.Itoa(c.Round))
	}
	parts = append(parts, verdictWord(c.Kept), "fit "+fitText(c.Verdict.Fit))
	label := strings.Join(parts, " · ")
	used := len(pad) + 3 + utf8.RuneCountInString(label) + 1
	tail := max(l.cols-used, 3)
	return pad + "── " + label + " " + strings.Repeat("─", tail)
}

// problems returns a candidate's problems as a subheading and a dashed list, or a line that
// says there are none.
func (l layout) problems(ps []string) string {
	if len(ps) == 0 {
		return subPad + "Problems: none"
	}
	return subPad + "Problems\n" + l.bullets(ps)
}

// spoken returns the spoken text of a candidate indented under its divider.
func (l layout) spoken(s string) string {
	return Hang(subPad, s, l.cols)
}

// verdictWord returns "kept" or "dropped".
func verdictWord(kept bool) string {
	if kept {
		return "kept"
	}
	return "dropped"
}

// roundText returns a round number, or a dash when the round is unknown.
func roundText(n int) string {
	if n < 1 {
		return "-"
	}
	return strconv.Itoa(n)
}

// fitText returns a fit score as "3/5", or a dash when there is none.
func fitText(n int) string {
	if n < 1 {
		return "-"
	}
	return strconv.Itoa(n) + "/5"
}
