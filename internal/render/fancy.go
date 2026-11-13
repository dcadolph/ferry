package render

import (
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"

	"github.com/dcadolph/ferry/internal/engine"
)

// Sizes of the styled output, in columns.
const (
	// panelMax caps the width of the headline panel so the analogy reads as a short block.
	panelMax = 84
	// fieldLabel is the width of the right-aligned skeleton labels.
	fieldLabel = 18
)

// fancy renders a result with borders, panels, tables, and truecolor for a terminal.
type fancy struct {
	// p holds the colors.
	p palette
	// cols is the width the output fits in.
	cols int
}

// newFancy returns a fancy renderer for the width that Width makes of cols, colored for a dark
// background when dark is true.
func newFancy(cols int, dark bool) fancy {
	return fancy{p: newPalette(dark), cols: Width(cols)}
}

// textOut returns the full styled output for res: with showWork the work first, then the
// analogy, so the output ends on the text to say.
func (f fancy) textOut(res engine.Result, showWork bool) string {
	out := f.analogy(res.Analogy)
	if showWork {
		out = f.work(res) + "\n" + out
	}
	return out
}

// analogy returns the winning analogy: what stands for what, where it breaks, and its world,
// then last the spoken text in a panel with a blended border.
func (f fancy) analogy(a engine.Analogy) string {
	width := min(f.cols-2, panelMax)
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForegroundBlend(f.p.accent, f.p.accent2).
		Padding(1, 3).
		Width(width).
		Bold(true).
		Foreground(f.p.text).
		Render(a.Spoken)
	parts := []string{
		f.title("What stands for what"),
		f.mappings(a.Mappings),
		"",
		" " + fg(f.p.warn).Bold(true).Render("Where it breaks"),
		f.para(a.BreaksDown, 3),
		"",
		"   " + fg(f.p.muted).Render("World: ") + fg(f.p.accent2).Bold(true).Render(a.World),
		"",
		f.title("Say it like this"),
		indentBlock(panel, 1),
	}
	return strings.Join(parts, "\n") + "\n"
}

// title returns a small section title in bold muted text.
func (f fancy) title(s string) string {
	return " " + fg(f.p.muted).Bold(true).Render(s)
}

// heading returns a section heading in bold accent2 followed by a muted rule to the width.
func (f fancy) heading(s string) string {
	rule := max(f.cols-lipgloss.Width(s)-3, 3)
	return " " + fg(f.p.accent2).Bold(true).Render(s) + " " +
		fg(f.p.muted).Render(strings.Repeat("─", rule))
}

// para returns s wrapped to the width and indented by n spaces, in body text color.
func (f fancy) para(s string, n int) string {
	if s == "" {
		return strings.Repeat(" ", n) + fg(f.p.muted).Render(none)
	}
	return indentBlock(fg(f.p.text).Width(f.cols-n).Render(s), n)
}

// mappings returns what stands for what as two columns joined by an arrow, the analogy side
// padded to the widest entry and the real side wrapped beside it.
func (f fancy) mappings(ms []engine.Mapping) string {
	if len(ms) == 0 {
		return "   " + fg(f.p.muted).Render(none)
	}
	left := 0
	for _, m := range ms {
		left = max(left, lipgloss.Width(m.Analogy))
	}
	left = min(left, f.cols/3)
	arrow := fg(f.p.muted).Render(" → ")
	right := max(f.cols-3-left-3, minWidth/2)
	lines := make([]string, len(ms))
	for i, m := range ms {
		l := fg(f.p.accent).Bold(true).Width(left).Render(m.Analogy)
		r := fg(f.p.text).Width(right).Render(m.Real)
		lines[i] = lipgloss.JoinHorizontal(lipgloss.Top, "   ", l, arrow, r)
	}
	return strings.Join(lines, "\n")
}

// work returns the skeleton, the candidates table, a card for each candidate, and the card of
// the polished analogy when it was checked.
func (f fancy) work(res engine.Result) string {
	s := res.Skeleton
	parts := []string{
		f.heading("Skeleton"),
		"",
		f.players(s.Players),
		"",
		f.field("Constraint", s.Constraint),
		f.field("Tradeoff", s.Tradeoff),
		f.field("Cost of inaction", s.CostOfInaction),
		f.field("Decision", s.Decision),
		f.field("Point", s.Point),
		"",
		f.title("Cause and effect"),
		f.chain(s.Chain),
		"",
		f.heading("Candidates"),
		"",
	}
	order := ranked(res.Checked)
	if len(order) == 0 {
		parts = append(parts, "   "+fg(f.p.muted).Render(none))
	} else {
		parts = append(parts, indentBlock(f.table(order), 1))
		for i, c := range order {
			parts = append(parts, "", f.card(i+1, c))
		}
	}
	if res.Recheck != nil {
		parts = append(parts, "", f.heading("Polished analogy"), "", f.card(0, *res.Recheck))
	}
	return strings.Join(parts, "\n") + "\n"
}

// ranked returns the candidates with the kept ones first, each group by fit from high to low,
// and ties in their original order.
func ranked(checked []engine.Checked) []engine.Checked {
	out := slices.Clone(checked)
	slices.SortStableFunc(out, func(a, b engine.Checked) int {
		switch {
		case a.Kept != b.Kept && a.Kept:
			return -1
		case a.Kept != b.Kept:
			return 1
		}
		return b.Verdict.Fit - a.Verdict.Fit
	})
	return out
}

// players returns the players as a table with rounded borders.
func (f fancy) players(ps []engine.Player) string {
	if len(ps) == 0 {
		return "   " + fg(f.p.muted).Render("Players: "+none)
	}
	rows := make([][]string, len(ps))
	for i, p := range ps {
		rows[i] = []string{p.Name, p.Role}
	}
	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(fg(f.p.muted)).
		Headers("Player", "Role").
		Rows(rows...).
		Width(f.cols - 2).
		Wrap(true).
		StyleFunc(func(row, col int) lipgloss.Style {
			s := lipgloss.NewStyle().Padding(0, 1)
			switch {
			case row == table.HeaderRow:
				return s.Foreground(f.p.accent2).Bold(true)
			case col == 0:
				return s.Foreground(f.p.text).Bold(true)
			}
			return s.Foreground(f.p.text)
		})
	return indentBlock(t.Render(), 1)
}

// field returns one skeleton field: a right-aligned muted label and its text wrapped beside it.
func (f fancy) field(label, text string) string {
	l := fg(f.p.muted).Width(fieldLabel).Align(lipgloss.Right).PaddingRight(2).Render(label)
	if text == "" {
		return l
	}
	r := fg(f.p.text).Width(f.cols - fieldLabel - 1).Render(text)
	return lipgloss.JoinHorizontal(lipgloss.Top, " ", l, r)
}

// chain returns the cause and effect steps, each number in accent with its text wrapped beside
// it.
func (f fancy) chain(steps []string) string {
	if len(steps) == 0 {
		return "   " + fg(f.p.muted).Render(none)
	}
	numWidth := len(strconv.Itoa(len(steps))) + 2
	lines := make([]string, len(steps))
	for i, step := range steps {
		n := fg(f.p.accent).Bold(true).Width(numWidth).Align(lipgloss.Right).
			Render(strconv.Itoa(i+1)) + " "
		r := fg(f.p.text).Width(f.cols - 3 - numWidth - 1).Render(step)
		lines[i] = lipgloss.JoinHorizontal(lipgloss.Top, "   ", n, r)
	}
	return strings.Join(lines, "\n")
}

// table returns the candidates as a table with rounded borders: rank, world, round, status
// pill, and fit meter. Dropped worlds are muted.
func (f fancy) table(order []engine.Checked) string {
	rows := make([][]string, len(order))
	for i, c := range order {
		rows[i] = []string{
			strconv.Itoa(i + 1), c.Candidate.World, roundText(c.Round),
			f.p.pill(c.Kept), f.p.meter(c.Verdict.Fit),
		}
	}
	return table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(fg(f.p.muted)).
		Headers("#", "World", "Round", "Status", "Fit").
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			s := lipgloss.NewStyle().Padding(0, 1)
			switch {
			case row == table.HeaderRow:
				return s.Foreground(f.p.accent2).Bold(true)
			case col == 0 || col == 2:
				return s.Foreground(f.p.muted)
			case col == 1 && !order[row].Kept:
				return s.Foreground(f.p.muted)
			case col == 1:
				return s.Foreground(f.p.text).Bold(true)
			}
			return s
		}).
		Render()
}

// card returns one candidate as a card with a thick left bar, green when kept and red when
// dropped: a header with its rank, world, pill, fit meter, and round, then its spoken text and
// its problems. A rank under 1 is left out.
func (f fancy) card(rank int, c engine.Checked) string {
	bar := f.p.bad
	if c.Kept {
		bar = f.p.good
	}
	width := f.cols - 2
	inner := width - 2
	head := ""
	if rank > 0 {
		head = fg(f.p.muted).Render("#"+strconv.Itoa(rank)) + "  "
	}
	head += fg(f.p.text).Bold(true).Render(c.Candidate.World) + "  " + f.p.pill(c.Kept) + "  " +
		f.p.meter(c.Verdict.Fit)
	if c.Round > 0 {
		head += "  " + fg(f.p.muted).Render("round "+strconv.Itoa(c.Round))
	}
	body := []string{head, ""}
	if c.Candidate.Spoken != "" {
		body = append(body, fg(f.p.text).Italic(true).Width(inner).Render(c.Candidate.Spoken), "")
	}
	body = append(body, f.problems(c.Verdict.Problems, inner))
	return indentBlock(lipgloss.NewStyle().
		Border(lipgloss.ThickBorder(), false, false, false, true).
		BorderForeground(bar).
		PaddingLeft(1).
		Render(strings.Join(body, "\n")), 1)
}

// problems returns a candidate's problems, each behind a warn marker with its text wrapped
// beside it, or a muted line that says there are none.
func (f fancy) problems(ps []string, width int) string {
	if len(ps) == 0 {
		return fg(f.p.muted).Render("No problems found.")
	}
	marker := fg(f.p.warn).Bold(true).Render("▸ ")
	lines := make([]string, len(ps))
	for i, p := range ps {
		text := fg(f.p.text).Width(width - 2).Render(p)
		lines[i] = lipgloss.JoinHorizontal(lipgloss.Top, marker, text)
	}
	return strings.Join(lines, "\n")
}

// indentBlock puts n spaces before every line of s.
func indentBlock(s string, n int) string {
	lead := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = lead + lines[i]
	}
	return strings.Join(lines, "\n")
}
