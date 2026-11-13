package render

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/ferry/internal/engine"
)

// testANSI matches the escape codes the styled output writes.
var testANSI = regexp.MustCompile("\x1b\\[[0-9;:]*m") //nolint:gochecknoglobals // Not modified, simplifies testing.

// testStrip returns s without escape codes.
func testStrip(s string) string {
	return testANSI.ReplaceAllString(s, "")
}

// TestFancyText checks that the styled output has its panel, tables, pills, meters, and cards,
// lists the kept candidate first, and keeps every line inside the width.
func TestFancyText(t *testing.T) {
	t.Parallel()
	res := testResult()
	res.Recheck = testRecheck(false)
	tests := []struct {
		WantParts []string
		Cols      int
		Dark      bool
		ShowWork  bool
	}{{ // Test 0: Without the work, only the analogy and its parts.
		Cols: 80, Dark: true,
		WantParts: []string{"╭", "Say it like this", "topping off the oil", " → ", "Where it breaks", "World: cars"},
	}, { // Test 1: With the work, the skeleton, the candidates, and the polished check.
		Cols: 100, Dark: true, ShowWork: true,
		WantParts: []string{
			"Skeleton", "│ Player", "the ops team", "Cost of inaction", "Cause and effect",
			"Candidates", "│ # │", " KEPT ", " DROPPED ", "●●●●○ 4/5", "●●○○○ 2/5",
			"▸ Mopping is cheap, restarts are not.", "No problems found.", "Polished analogy",
		},
	}, { // Test 2: A light background renders the same layout.
		Cols: 60, Dark: false, ShowWork: true,
		WantParts: []string{"╭", " KEPT ", "▸ "},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			raw := newFancy(test.Cols, test.Dark).textOut(res, test.ShowWork)
			got := testStrip(raw)
			for _, want := range test.WantParts {
				if !strings.Contains(got, want) {
					t.Errorf("output lacks %q:\n%s", want, got)
				}
			}
			for i, line := range strings.Split(raw, "\n") {
				if w := lipgloss.Width(line); w > test.Cols {
					t.Errorf("line %d is %d wide, over %d: %q", i, w, test.Cols, testStrip(line))
				}
			}
			if test.ShowWork && strings.Index(got, "#1  cars") > strings.Index(got, "#2  kitchens") {
				t.Errorf("kept candidate is not first:\n%s", got)
			}
		})
	}
}

// TestRanked checks that kept candidates come first, then the rest by fit from high to low.
func TestRanked(t *testing.T) {
	t.Parallel()
	c := func(world string, kept bool, fit int) engine.Checked {
		return engine.Checked{Candidate: engine.Candidate{World: world}, Kept: kept, Verdict: engine.Verdict{Fit: fit}}
	}
	in := []engine.Checked{c("a", false, 2), c("b", true, 4), c("c", false, 3), c("d", false, 3), c("e", true, 5)}
	var got []string
	for _, r := range ranked(in) {
		got = append(got, r.Candidate.World)
	}
	if diff := cmp.Diff([]string{"e", "b", "c", "d", "a"}, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff("a", in[0].Candidate.World); diff != "" {
		t.Errorf("input was reordered (-want +got):\n%s", diff)
	}
}

// TestMeter checks the dots and the number of a fit meter.
func TestMeter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		WantResult string
		In         int
	}{{ // Test 0: No score is a dash.
		In: 0, WantResult: "-",
	}, { // Test 1: A middling score.
		In: 3, WantResult: "●●●○○ 3/5",
	}, { // Test 2: A full score.
		In: 5, WantResult: "●●●●● 5/5",
	}, { // Test 3: A score past the top is capped.
		In: 9, WantResult: "●●●●● 5/5",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := testStrip(newFancy(80, true).meter(test.In))
			if diff := cmp.Diff(test.WantResult, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestTextStyled checks that Text and Work take the styled path when color is on, cut the
// colors for a writer that is not a terminal, and surface a write failure.
func TestTextStyled(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Write    func(w *bytes.Buffer) error
		WantPart string
	}{{ // Test 0: Text.
		Write:    func(w *bytes.Buffer) error { return Text(w, testResult(), true, NewStyle(true), 80) },
		WantPart: "╭",
	}, { // Test 1: Work on a light background.
		Write:    func(w *bytes.Buffer) error { return Work(w, testResult(), NewStyle(true).OnLight(true), 80) },
		WantPart: " DROPPED ",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			if err := test.Write(&buf); err != nil {
				t.Fatalf("write: %v", err)
			}
			if !strings.Contains(buf.String(), test.WantPart) {
				t.Errorf("output lacks %q:\n%s", test.WantPart, buf.String())
			}
			if strings.Contains(buf.String(), "\x1b[") {
				t.Errorf("output to a buffer kept escape codes:\n%q", buf.String())
			}
		})
	}
	err := Text(testFailWriter{}, testResult(), true, NewStyle(true), 80)
	if !errors.Is(err, ErrWrite) {
		t.Errorf("error = %v, want %v", err, ErrWrite)
	}
}
