// Package render formats an engine result as plain text for the terminal.
package render

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"strings"
	"text/template"
	"unicode/utf8"

	"github.com/dcadolph/ferry/internal/engine"
	"github.com/dcadolph/ferry/internal/util"
)

// width is the column at which prose is wrapped.
const width = 80

// Names of the templates in text.tmpl.
const (
	// analogyPart prints the winning analogy.
	analogyPart = "analogy"
	// workPart prints the skeleton and every candidate with its verdict.
	workPart = "work"
)

// textSource holds the text output templates.
//
//go:embed text.tmpl
var textSource string

// textTemplate is the parsed set of text output templates.
var textTemplate = template.Must(template.New("text").Funcs(template.FuncMap{
	"wrap": func(s string) string { return wrap(s, width) },
	"inc":  util.Inc,
}).Parse(textSource))

// Text writes res to w as plain text: the spoken analogy, what stands for what, where it
// breaks, and its world. With showWork it adds a blank line and the work that Work prints.
// Nothing is written when a template fails.
func Text(w io.Writer, res engine.Result, showWork bool) error {
	var buf bytes.Buffer
	if err := textTemplate.ExecuteTemplate(&buf, analogyPart, res); err != nil {
		return fmt.Errorf("%w: %w", ErrRender, err)
	}
	if showWork {
		buf.WriteByte('\n')
		if err := textTemplate.ExecuteTemplate(&buf, workPart, res); err != nil {
			return fmt.Errorf("%w: %w", ErrRender, err)
		}
	}
	return flush(w, &buf)
}

// Work writes the work behind res to w as plain text: the skeleton, every candidate with its
// world, round, verdict, fit, spoken text, and problems, and the check of the polished analogy
// when the run got that far. A polished text that did not pass is printed there, since the
// analogy in the result is then the checked draft. Work suits a run where no analogy held up,
// since it leaves out the missing winner. Nothing is written when the template fails.
func Work(w io.Writer, res engine.Result) error {
	var buf bytes.Buffer
	if err := textTemplate.ExecuteTemplate(&buf, workPart, res); err != nil {
		return fmt.Errorf("%w: %w", ErrRender, err)
	}
	return flush(w, &buf)
}

// flush writes the rendered text in buf to w.
func flush(w io.Writer, buf *bytes.Buffer) error {
	if _, err := w.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("%w: %w", ErrWrite, err)
	}
	return nil
}

// wrap breaks each line of s at spaces so no line runs past cols runes. A word longer than
// cols gets a line of its own. Existing line breaks are kept and runs of spaces collapse to one.
func wrap(s string, cols int) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = wrapLine(line, cols)
	}
	return strings.Join(lines, "\n")
}

// wrapLine wraps one line that holds no line breaks.
func wrapLine(line string, cols int) string {
	var b strings.Builder
	col := 0
	for _, word := range strings.Fields(line) {
		n := utf8.RuneCountInString(word)
		switch {
		case col == 0:
		case col+1+n > cols:
			b.WriteByte('\n')
			col = 0
		default:
			b.WriteByte(' ')
			col++
		}
		b.WriteString(word)
		col += n
	}
	return b.String()
}
