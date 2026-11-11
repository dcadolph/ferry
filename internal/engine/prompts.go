package engine

import (
	"embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/dcadolph/ferry/internal/util"
)

// systemTemplate names the template for the system prompt every request shares.
const systemTemplate = "system.tmpl"

// promptFS holds the prompt templates.
//
//go:embed prompts/*.tmpl
var promptFS embed.FS

// prompts holds the parsed prompt templates, each named by its file name.
var prompts = template.Must(template.New("prompts").Funcs(template.FuncMap{
	"inc": util.Inc,
}).ParseFS(promptFS, "prompts/*.tmpl"))

// promptData holds what the prompt templates draw on.
type promptData struct {
	// Summary is the speaker's summary of the situation.
	Summary string
	// Audience describes who will hear the analogy.
	Audience string
	// Skeleton is the structure pulled out of the summary.
	Skeleton Skeleton
	// World is the world a draft draws from.
	World string
	// Dropped lists why drafts from earlier rounds did not hold up.
	Dropped []string
	// Candidate is the analogy being checked or finished.
	Candidate Candidate
	// Verdict is the check of the analogy being finished.
	Verdict Verdict
	// Problem says what was wrong with the last reply when a request is retried.
	Problem string
}

// templateFor returns the name of the prompt template for step. The re-check of the polished
// analogy uses the check template, so it is judged by the same rules as the drafts.
func templateFor(step Step) string {
	if step == StepRecheck {
		step = StepCheck
	}
	return string(step) + ".tmpl"
}

// render executes the named prompt template with data and trims the result.
func render(name string, data promptData) (string, error) {
	var b strings.Builder
	if err := prompts.ExecuteTemplate(&b, name, data); err != nil {
		return "", fmt.Errorf("%w: %s: %w", ErrRender, name, err)
	}
	return strings.TrimSpace(b.String()), nil
}
