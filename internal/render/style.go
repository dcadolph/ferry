package render

import (
	"os"
	"strings"
)

// ANSI escape codes for the styles ferry uses.
const (
	reset  = "\x1b[0m"
	bold   = "\x1b[1m"
	dim    = "\x1b[2m"
	red    = "\x1b[31m"
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	cyan   = "\x1b[36m"
)

// Style colors text when color is on and returns it unchanged when it is off.
type Style struct {
	// on reports whether color codes are written.
	on bool
}

// NewStyle returns a Style that writes color codes only when on is true.
func NewStyle(on bool) Style {
	return Style{on: on}
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
