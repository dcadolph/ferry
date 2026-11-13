package render

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestStyle checks that each style wraps every non-empty line when on and passes text through
// when off.
func TestStyle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Fn         func(Style) func(string) string
		WantResult string
		In         string
		On         bool
	}{{ // Test 0: Off leaves text alone.
		Fn: func(s Style) func(string) string { return s.Green }, In: "kept",
		WantResult: "kept",
	}, { // Test 1: Green on one line.
		Fn: func(s Style) func(string) string { return s.Green }, In: "kept", On: true,
		WantResult: "\x1b[32mkept\x1b[0m",
	}, { // Test 2: Each line gets its own codes, and empty lines stay empty.
		Fn: func(s Style) func(string) string { return s.Dim }, In: "a\n\nb", On: true,
		WantResult: "\x1b[2ma\x1b[0m\n\n\x1b[2mb\x1b[0m",
	}, { // Test 3: Empty text stays empty.
		Fn: func(s Style) func(string) string { return s.Red }, In: "", On: true,
		WantResult: "",
	}, { // Test 4: Heading is bold cyan.
		Fn: func(s Style) func(string) string { return s.Heading }, In: "Say it", On: true,
		WantResult: "\x1b[1m\x1b[36mSay it\x1b[0m",
	}, { // Test 5: Bold.
		Fn: func(s Style) func(string) string { return s.Bold }, In: "x", On: true,
		WantResult: "\x1b[1mx\x1b[0m",
	}, { // Test 6: Yellow.
		Fn: func(s Style) func(string) string { return s.Yellow }, In: "x", On: true,
		WantResult: "\x1b[33mx\x1b[0m",
	}, { // Test 7: Cyan.
		Fn: func(s Style) func(string) string { return s.Cyan }, In: "x", On: true,
		WantResult: "\x1b[36mx\x1b[0m",
	}, { // Test 8: Red.
		Fn: func(s Style) func(string) string { return s.Red }, In: "x", On: true,
		WantResult: "\x1b[31mx\x1b[0m",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := test.Fn(NewStyle(test.On))(test.In)
			if diff := cmp.Diff(test.WantResult, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestColorWanted checks the terminal, NO_COLOR, and TERM rules. It sets the environment, so it
// does not run in parallel.
func TestColorWanted(t *testing.T) {
	tests := []struct {
		NoColor  string
		Term     string
		Terminal bool
		Want     bool
	}{{ // Test 0: A plain terminal gets color.
		Terminal: true, Term: "xterm-256color", Want: true,
	}, { // Test 1: Not a terminal.
		Terminal: false, Term: "xterm-256color", Want: false,
	}, { // Test 2: NO_COLOR set.
		Terminal: true, Term: "xterm-256color", NoColor: "1", Want: false,
	}, { // Test 3: A dumb terminal.
		Terminal: true, Term: "dumb", Want: false,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Setenv("NO_COLOR", test.NoColor)
			t.Setenv("TERM", test.Term)
			if diff := cmp.Diff(test.Want, ColorWanted(test.Terminal)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.Want, NewStyle(test.Want).On()); diff != "" {
				t.Errorf("On mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
