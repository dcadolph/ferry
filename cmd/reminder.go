package cmd

import (
	"fmt"
	"io"
)

// reminder warns that the summary leaves the machine. ferry prints it before every run that
// calls the model and never puts it behind a flag, because ferry cannot scrub the text itself.
const reminder = `Privacy: your summary is sent to Claude. Remove names, company names, and other
specifics from day-job material first. ferry does not scrub anything itself.`

// printReminder writes the privacy reminder to w and returns the write error, so the caller can
// stop before it reads or sends anything when the reminder could not be shown.
func printReminder(w io.Writer) error {
	_, err := fmt.Fprintln(w, reminder)
	return err
}
