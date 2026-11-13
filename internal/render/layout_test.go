package render

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/ferry/internal/engine"
)

// TestWidth checks that a width is clamped to the range ferry wraps text in.
func TestWidth(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         int
		WantResult int
	}{{ // Test 0: An unknown width gets the default.
		In: 0, WantResult: DefaultWidth,
	}, { // Test 1: A negative width gets the default.
		In: -5, WantResult: DefaultWidth,
	}, { // Test 2: A narrow terminal is raised to the minimum.
		In: 20, WantResult: minWidth,
	}, { // Test 3: A width in range is kept.
		In: 72, WantResult: 72,
	}, { // Test 4: A wide terminal is capped.
		In: 240, WantResult: MaxWidth,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(test.WantResult, Width(test.In)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestHang checks the hanging indent: later lines line up under the first line's text.
func TestHang(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Prefix     string
		Text       string
		WantResult string
		Cols       int
	}{{ // Test 0: Short text stays on the prefix line.
		Prefix: "- ", Text: "one two", Cols: 40, WantResult: "- one two",
	}, { // Test 1: Long text wraps under the text, not under the prefix.
		Prefix: "ferry: ", Text: "aaaa bbbb cccc dddd eeee ffff", Cols: 30,
		WantResult: "ferry: aaaa bbbb cccc dddd\n       eeee ffff",
	}, { // Test 2: Empty text returns the prefix without trailing spaces.
		Prefix: "  Point   ", Text: "", Cols: 40, WantResult: "  Point",
	}, { // Test 3: A prefix wider than the width still leaves room for the text.
		Prefix: strings.Repeat("x", 38) + " ", Text: "a b", Cols: 40,
		WantResult: strings.Repeat("x", 38) + " a b",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(test.WantResult, Hang(test.Prefix, test.Text, test.Cols)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestLayoutPlayers checks the player columns, including a name too long for its column.
func TestLayoutPlayers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		WantResult string
		In         []engine.Player
		Cols       int
	}{{ // Test 0: No players prints a placeholder.
		In: nil, Cols: 80, WantResult: "    (none)",
	}, { // Test 1: Names are padded to the longest one and roles wrap in their column.
		In: []engine.Player{
			{Name: "ops", Role: "restarts the service every single morning"},
			{Name: "leadership", Role: "decides"},
		},
		Cols: 40,
		WantResult: "    ops         restarts the service\n" +
			"                every single morning\n" +
			"    leadership  decides",
	}, { // Test 2: A name past a third of the width gets its own line and does not widen the
		// column for the others.
		In: []engine.Player{
			{Name: "ops", Role: "restarts"},
			{Name: "the test suite nobody owns", Role: "fails at random"},
		},
		Cols: 60,
		WantResult: "    ops  restarts\n" +
			"    the test suite nobody owns\n" +
			"         fails at random",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			l := layout{cols: test.Cols}
			if diff := cmp.Diff(test.WantResult, l.players(test.In)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestLayoutSteps checks that step numbers line up once the list reaches ten.
func TestLayoutSteps(t *testing.T) {
	t.Parallel()
	chain := make([]string, 10)
	for i := range chain {
		chain[i] = "step " + strconv.Itoa(i+1)
	}
	tests := []struct {
		WantResult string
		In         []string
	}{{ // Test 0: No steps prints a placeholder.
		In: nil, WantResult: "    (none)",
	}, { // Test 1: Under ten steps, numbers take one digit.
		In: []string{"a", "b"}, WantResult: "    1. a\n    2. b",
	}, { // Test 2: At ten steps, one-digit numbers are padded to line up.
		In: chain,
		WantResult: "    1.  step 1\n    2.  step 2\n    3.  step 3\n    4.  step 4\n" +
			"    5.  step 5\n    6.  step 6\n    7.  step 7\n    8.  step 8\n" +
			"    9.  step 9\n    10. step 10",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			l := layout{cols: 80}
			if diff := cmp.Diff(test.WantResult, l.steps(test.In)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestLayoutProblems checks that long problems wrap under their own text.
func TestLayoutProblems(t *testing.T) {
	t.Parallel()
	tests := []struct {
		WantResult string
		In         []string
	}{{ // Test 0: No problems says so on one line.
		In: nil, WantResult: "    Problems: none",
	}, { // Test 1: A long problem wraps with a hanging indent.
		In: []string{"short", "a long problem that runs past the width of the column"},
		WantResult: "    Problems\n    - short\n" +
			"    - a long problem that runs past the\n      width of the column",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			l := layout{cols: 42}
			if diff := cmp.Diff(test.WantResult, l.problems(test.In)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
