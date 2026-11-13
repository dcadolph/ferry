package render

import (
	"os"
	"strings"
)

// ANSI escape codes for the styles ferry uses.
const (
	reset   = "\x1b[0m"
	bold    = "\x1b[1m"
	dim     = "\x1b[2m"
	italic  = "\x1b[3m"
	red     = "\x1b[31m"
	green   = "\x1b[32m"
	yellow  = "\x1b[33m"
	magenta = "\x1b[35m"
	cyan    = "\x1b[36m"
)

// Style colors text when color is on and returns it unchanged when it is off.
type Style struct {
	// on reports whether color codes are written.
	on bool
	// light reports whether the terminal has a light background.
	light bool
}

// NewStyle returns a Style that writes color codes only when on is true. It assumes a dark
// background until OnLight says otherwise.
func NewStyle(on bool) Style {
	return Style{on: on}
}

// OnLight returns a copy of s for a terminal with a light background when light is true.
func (s Style) OnLight(light bool) Style {
	s.light = light
	return s
}

// ColorWanted reports whether output to a stream should be colored: the stream is a terminal, NO_COLOR
// is unset or empty, and TERM is not "dumb".
func ColorWanted(terminal bool) bool {
	return terminal && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
}

// On reports whether s writes color codes.
func (s Style) On() bool {
	return s.on
}

// Bold returns text in bold.
func (s Style) Bold(text string) string { return s.wrap(bold, text) }

// Dim returns text in a faint weight.
func (s Style) Dim(text string) string { return s.wrap(dim, text) }

// Red returns text in red.
func (s Style) Red(text string) string { return s.wrap(red, text) }

// Green returns text in green.
func (s Style) Green(text string) string { return s.wrap(green, text) }

// Yellow returns text in yellow.
func (s Style) Yellow(text string) string { return s.wrap(yellow, text) }

// Cyan returns text in cyan.
func (s Style) Cyan(text string) string { return s.wrap(cyan, text) }

// Italic returns text in italics.
func (s Style) Italic(text string) string { return s.wrap(italic, text) }

// Label returns text in bold magenta, for the labels inside a section.
func (s Style) Label(text string) string { return s.wrap(bold+magenta, text) }

// Fit returns text colored by a fit score: green from 4 up, yellow at 3, and red below.
func (s Style) Fit(n int, text string) string {
	switch {
	case n >= 4:
		return s.Green(text)
	case n == 3:
		return s.Yellow(text)
	}
	return s.Red(text)
}

// Heading returns text in bold cyan.
func (s Style) Heading(text string) string { return s.wrap(bold+cyan, text) }

// wrap puts code before each line of text and a reset after it, so a style never bleeds into
// the next line or into text a terminal wraps.
func (s Style) wrap(code, text string) string {
	if !s.on || text == "" {
		return text
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = code + line + reset
		}
	}
	return strings.Join(lines, "\n")
}
