package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"
)

// TestReminder checks that the privacy reminder makes each promise it has to make, never
// claims to scrub, and fits an 80-column terminal.
func TestReminder(t *testing.T) {
	t.Parallel()
	tests := []struct {
		WantPhrase string
	}{{ // Test 0: It says where the summary goes.
		WantPhrase: "sent to Claude",
	}, { // Test 1: It names what to remove.
		WantPhrase: "Remove names, company names, and other specifics",
	}, { // Test 2: It names the material at risk.
		WantPhrase: "day-job material",
	}, { // Test 3: It says ferry does not scrub.
		WantPhrase: "ferry does not scrub anything itself",
	}}
	flat := strings.Join(strings.Fields(reminder), " ")
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if !strings.Contains(flat, test.WantPhrase) {
				t.Errorf("reminder %q is missing %q", flat, test.WantPhrase)
			}
		})
	}
	for _, line := range strings.Split(reminder, "\n") {
		if n := utf8.RuneCountInString(line); n > 80 {
			t.Errorf("reminder line is %d columns, want 80 or fewer: %q", n, line)
		}
	}
}

// TestPrintReminder checks that the reminder prints as written, ending in a newline, and that
// a failed write comes back to the caller.
func TestPrintReminder(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tests := []struct {
		Want       error
		W          io.Writer
		WantResult string
	}{{ // Test 0: A working writer gets the whole reminder.
		W: &buf, WantResult: reminder + "\n", Want: nil,
	}, { // Test 1: A failing writer returns its error.
		W: testFailWriter{}, WantResult: "", Want: errTestWrite,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if err := printReminder(test.W); !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			if b, ok := test.W.(*bytes.Buffer); ok {
				if diff := cmp.Diff(test.WantResult, b.String()); diff != "" {
					t.Errorf("mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}
