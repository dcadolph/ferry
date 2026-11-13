// Package render formats an engine result as text for the terminal, styled when wanted.
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
)

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

// textTemplate parses the plain text output templates, wrapping text to cols.
func textTemplate(cols int) (*template.Template, error) {
	l := layout{cols: Width(cols)}
	return template.New("text").Funcs(template.FuncMap{
		"wrap":     func(s string) string { return wrap(s, l.cols) },
		"field":    l.field,
		"players":  l.players,
		"steps":    l.steps,
		"table":    l.table,
		"rule":     l.rule,
		"spoken":   l.spoken,
		"problems": l.problems,
	}).Parse(textSource)
}

// Text writes res to w: the spoken analogy, what stands for what, where it breaks, and its
// world, and with showWork a blank line and the work that Work prints. When st is on, the
// output is the styled layout with panels, tables, and colors for a dark background unless st
// is set for a light one. Otherwise it is plain text. Either way it fits the width that Width
// makes of cols. Nothing is written when a template fails.
func Text(w io.Writer, res engine.Result, showWork bool, st Style, cols int) error {
	if st.On() {
		return fancyOut(w, newFancy(cols, !st.light).textOut(res, showWork))
	}
	t, err := textTemplate(cols)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrRender, err)
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, analogyPart, res); err != nil {
		return fmt.Errorf("%w: %w", ErrRender, err)
	}
	if showWork {
		buf.WriteByte('\n')
		if err := t.ExecuteTemplate(&buf, workPart, res); err != nil {
			return fmt.Errorf("%w: %w", ErrRender, err)
		}
	}
	return flush(w, &buf)
}

// Work writes the work behind res to w: the skeleton, every candidate with its world, round,
// verdict, fit, spoken text, and problems, and the check of the polished analogy when the run
// got that far. A polished text that did not pass is printed there, since the analogy in the
// result is then the checked draft. Work suits a run where no analogy held up, since it leaves
// out the missing winner. It is styled or plain as Text describes, and fits the width that
// Width makes of cols. Nothing is written when the template fails.
func Work(w io.Writer, res engine.Result, st Style, cols int) error {
	if st.On() {
		return fancyOut(w, newFancy(cols, !st.light).work(res))
	}
	t, err := textTemplate(cols)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrRender, err)
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, workPart, res); err != nil {
		return fmt.Errorf("%w: %w", ErrRender, err)
	}
	return flush(w, &buf)
}

// fancyOut writes styled output to w through ColorWriter.
func fancyOut(w io.Writer, s string) error {
	if _, err := io.WriteString(ColorWriter(w), s); err != nil {
		return fmt.Errorf("%w: %w", ErrWrite, err)
	}
	return nil
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
