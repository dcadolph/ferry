package render

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestStyle checks that each style passes text through when off, and when on renders every
// non-empty line in the palette color for the background.
func TestStyle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Fn         func(Style) func(string) string
		WantResult string
		In         string
		On         bool
		Light      bool
	}{{ // Test 0: Off leaves text alone.
		Fn: func(s Style) func(string) string { return s.Good }, In: "kept",
		WantResult: "kept",
	}, { // Test 1: Good on a dark background.
		Fn: func(s Style) func(string) string { return s.Good }, In: "kept", On: true,
		WantResult: "\x1b[38;2;34;197;94mkept\x1b[m",
	}, { // Test 2: Good on a light background takes the darker shade.
		Fn: func(s Style) func(string) string { return s.Good }, In: "kept", On: true, Light: true,
		WantResult: "\x1b[38;2;21;128;61mkept\x1b[m",
	}, { // Test 3: Each line gets its own codes, and empty lines stay empty.
		Fn: func(s Style) func(string) string { return s.Dim }, In: "a\n\nb", On: true,
		WantResult: "\x1b[38;2;156;163;175ma\x1b[m\n\n\x1b[38;2;156;163;175mb\x1b[m",
	}, { // Test 4: Empty text stays empty.
		Fn: func(s Style) func(string) string { return s.Bad }, In: "", On: true,
		WantResult: "",
	}, { // Test 5: Heading is bold accent2.
		Fn: func(s Style) func(string) string { return s.Heading }, In: "Say it", On: true,
		WantResult: "\x1b[1;38;2;34;211;238mSay it\x1b[m",
	}, { // Test 6: Bold.
		Fn: func(s Style) func(string) string { return s.Bold }, In: "x", On: true,
		WantResult: "\x1b[1mx\x1b[m",
	}, { // Test 7: Warn.
		Fn: func(s Style) func(string) string { return s.Warn }, In: "x", On: true,
		WantResult: "\x1b[38;2;251;191;36mx\x1b[m",
	}, { // Test 8: Bad.
		Fn: func(s Style) func(string) string { return s.Bad }, In: "x", On: true,
		WantResult: "\x1b[38;2;248;113;113mx\x1b[m",
	}, { // Test 9: Accent.
		Fn: func(s Style) func(string) string { return s.Accent }, In: "x", On: true,
		WantResult: "\x1b[38;2;167;139;250mx\x1b[m",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := test.Fn(NewStyle(test.On).OnLight(test.Light))(test.In)
			if diff := cmp.Diff(test.WantResult, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestStylePillMeter checks the pill and the fit meter with color on and off.
func TestStylePillMeter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Got        string
		WantResult string
	}{{ // Test 0: Kept pill, plain.
		Got: NewStyle(false).Pill(true), WantResult: "kept",
	}, { // Test 1: Dropped pill, plain.
		Got: NewStyle(false).Pill(false), WantResult: "dropped",
	}, { // Test 2: Kept pill, styled, padded on both sides.
		Got: testStrip(NewStyle(true).Pill(true)), WantResult: " KEPT ",
	}, { // Test 3: Dropped pill, styled.
		Got: testStrip(NewStyle(true).Pill(false)), WantResult: " DROPPED ",
	}, { // Test 4: Meter, plain.
		Got: NewStyle(false).Meter(4), WantResult: "4/5",
	}, { // Test 5: Meter, styled.
		Got: testStrip(NewStyle(true).Meter(4)), WantResult: "●●●●○ 4/5",
	}, { // Test 6: No score, plain.
		Got: NewStyle(false).Meter(0), WantResult: "",
	}, { // Test 7: No score, styled.
		Got: NewStyle(true).Meter(0), WantResult: "",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(test.WantResult, test.Got); diff != "" {
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
