package render

import (
	"image/color"
	"io"
	"os"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

// meterDots is the number of dots in a fit meter, one per point of fit.
const meterDots = 5

// palette holds ferry's colors, picked for a dark or a light background.
type palette struct {
	// accent colors mapping sides, step numbers, the spinner, and the start of the panel border.
	accent color.Color
	// accent2 colors headings, table headers, and the end of the panel border.
	accent2 color.Color
	// text colors body text.
	text color.Color
	// muted colors labels, borders, and secondary text.
	muted color.Color
	// keptBG is the fill of a kept pill.
	keptBG color.Color
	// keptFG is the text of a kept pill.
	keptFG color.Color
	// dropBG is the fill of a dropped pill.
	dropBG color.Color
	// dropFG is the text of a dropped pill.
	dropFG color.Color
	// good colors a high fit, a kept mark, and the bar of a kept card.
	good color.Color
	// warn colors a middling fit, problem markers, and the breakdown label.
	warn color.Color
	// bad colors a low fit, a dropped or failed mark, and the bar of a dropped card.
	bad color.Color
	// empty colors the unfilled dots of a fit meter.
	empty color.Color
}

// newPalette returns the palette for a dark background when dark is true, and for a light one
// otherwise.
func newPalette(dark bool) palette {
	ld := lipgloss.LightDark(dark)
	pick := func(light, dark string) color.Color {
		return ld(lipgloss.Color(light), lipgloss.Color(dark))
	}
	return palette{
		accent:  pick("#6D28D9", "#A78BFA"),
		accent2: pick("#0E7490", "#22D3EE"),
		text:    pick("#1F2937", "#E5E7EB"),
		muted:   pick("#6B7280", "#9CA3AF"),
		keptBG:  pick("#15803D", "#22C55E"),
		keptFG:  pick("#FFFFFF", "#052E16"),
		dropBG:  pick("#B91C1C", "#F87171"),
		dropFG:  pick("#FFFFFF", "#450A0A"),
		good:    pick("#15803D", "#22C55E"),
		warn:    pick("#B45309", "#FBBF24"),
		bad:     pick("#B91C1C", "#F87171"),
		empty:   pick("#D1D5DB", "#4B5563"),
	}
}

// fg returns a style with foreground c.
func fg(c color.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(c)
}

// pill returns KEPT or DROPPED as bold text on a filled background.
func (p palette) pill(kept bool) string {
	if kept {
		return lipgloss.NewStyle().Bold(true).Padding(0, 1).
			Background(p.keptBG).Foreground(p.keptFG).Render("KEPT")
	}
	return lipgloss.NewStyle().Bold(true).Padding(0, 1).
		Background(p.dropBG).Foreground(p.dropFG).Render("DROPPED")
}

// meter returns a fit score as five dots, filled up to the score in a color that rates it,
// followed by the score as a number so it never rests on color alone.
func (p palette) meter(n int) string {
	if n < 1 {
		return fg(p.muted).Render("-")
	}
	n = min(n, meterDots)
	c := p.bad
	switch {
	case n >= 4:
		c = p.good
	case n == 3:
		c = p.warn
	}
	dots := fg(c).Render(strings.Repeat("●", n))
	if n < meterDots {
		dots += fg(p.empty).Render(strings.Repeat("○", meterDots-n))
	}
	return dots + " " + fg(p.text).Render(strconv.Itoa(n)+"/5")
}

// Style colors text in ferry's palette when color is on and returns it unchanged when it is off.
// Colors are written as truecolor, so text styled by it should reach the terminal through
// ColorWriter.
type Style struct {
	// on reports whether color codes are written.
	on bool
	// light reports whether the terminal has a light background.
	light bool
	// p holds the colors for the background.
	p palette
}

// NewStyle returns a Style that writes color codes only when on is true. It assumes a dark
// background until OnLight says otherwise.
func NewStyle(on bool) Style {
	return Style{on: on, p: newPalette(true)}
}

// OnLight returns a copy of s for a terminal with a light background when light is true.
func (s Style) OnLight(light bool) Style {
	s.light = light
	s.p = newPalette(!light)
	return s
}

// ColorWanted reports whether output to a stream should be colored: the stream is a terminal, NO_COLOR
// is unset or empty, and TERM is not "dumb".
func ColorWanted(terminal bool) bool {
	return terminal && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
}

// ColorWriter returns w wrapped so the colors written through it are cut down to what the
// terminal behind w shows. When w is not a terminal, color codes are removed.
func ColorWriter(w io.Writer) io.Writer {
	return colorprofile.NewWriter(w, os.Environ())
}

// On reports whether s writes color codes.
func (s Style) On() bool {
	return s.on
}

// Bold returns text in bold.
func (s Style) Bold(text string) string { return s.wrap(lipgloss.NewStyle().Bold(true), text) }

// Dim returns text in the muted color.
func (s Style) Dim(text string) string { return s.wrap(fg(s.p.muted), text) }

// Heading returns text in bold accent2, the color of the result's headings.
func (s Style) Heading(text string) string { return s.wrap(fg(s.p.accent2).Bold(true), text) }

// Accent returns text in the accent color.
func (s Style) Accent(text string) string { return s.wrap(fg(s.p.accent), text) }

// Good returns text in the color of a kept candidate.
func (s Style) Good(text string) string { return s.wrap(fg(s.p.good), text) }

// Warn returns text in the warning color.
func (s Style) Warn(text string) string { return s.wrap(fg(s.p.warn), text) }

// Bad returns text in the color of a dropped candidate or a failure.
func (s Style) Bad(text string) string { return s.wrap(fg(s.p.bad), text) }

// Pill returns the KEPT or DROPPED pill of the result's candidates table, or "kept" or
// "dropped" when color is off.
func (s Style) Pill(kept bool) string {
	switch {
	case s.on:
		return s.p.pill(kept)
	case kept:
		return "kept"
	}
	return "dropped"
}

// Meter returns the fit meter of the result's candidates table, or the score as "3/5" when
// color is off. A score under 1 gives an empty string.
func (s Style) Meter(n int) string {
	switch {
	case n < 1:
		return ""
	case s.on:
		return s.p.meter(n)
	}
	return strconv.Itoa(n) + "/5"
}

// wrap renders each non-empty line of text with st, so a style never bleeds into the next line
// or into text a terminal wraps.
func (s Style) wrap(st lipgloss.Style, text string) string {
	if !s.on || text == "" {
		return text
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = st.Render(line)
		}
	}
	return strings.Join(lines, "\n")
}
