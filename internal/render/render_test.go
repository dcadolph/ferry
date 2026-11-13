package render

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/ferry/internal/engine"
)

// errTestWrite is returned by testFailWriter.
var errTestWrite = errors.New("disk full")

// testFailWriter is an io.Writer that always fails.
type testFailWriter struct{}

// Write returns errTestWrite.
func (testFailWriter) Write([]byte) (int, error) { return 0, errTestWrite }

// testResult returns a filled result with one kept and one dropped candidate.
func testResult() engine.Result {
	return engine.Result{
		Analogy: engine.Analogy{
			World: "cars",
			Spoken: "We keep topping off the oil every morning instead of fixing the leak. " +
				"The engine still runs, but each quart costs us a little more, and one day " +
				"the leak wins. Let's put the car on the lift this week.",
			Mappings: []engine.Mapping{
				{Analogy: "topping off the oil", Real: "restarting the service each morning"},
				{Analogy: "the leak", Real: "the memory bug"},
				{Analogy: "putting the car on the lift", Real: "a week to fix the bug"},
			},
			BreaksDown: "A leak drips at a steady rate, but our crashes come in bursts after " +
				"big releases.",
		},
		Skeleton: engine.Skeleton{
			Players: []engine.Player{
				{Name: "the ops team", Role: "restarts the service"},
				{Name: "leadership", Role: "decides what gets fixed"},
			},
			Constraint:     "two engineers this quarter",
			Tradeoff:       "a week on the fix delays the next feature",
			CostOfInaction: "outages grow until a restart no longer helps",
			Decision:       "spend a week on the fix now",
			Point:          "the restarts cost more than the fix",
			Chain: []string{
				"A memory bug leaks memory.",
				"The service crashes overnight.",
				"Someone restarts it each morning.",
			},
		},
		Checked: []engine.Checked{{
			Candidate: engine.Candidate{
				World:  "cars",
				Spoken: "We keep topping off the oil instead of fixing the leak.",
			},
			Verdict: engine.Verdict{
				SameShape:        true,
				FamiliarWorld:    true,
				PointsAtDecision: true,
				Fit:              4,
			},
			Kept:  true,
			Round: 1,
		}, {
			Candidate: engine.Candidate{
				World:  "kitchens",
				Spoken: "We keep mopping the floor while the sink runs.",
			},
			Verdict: engine.Verdict{
				FamiliarWorld: true,
				Fit:           2,
				Problems: []string{
					"A running sink is obvious, the bug is hidden.",
					"Mopping is cheap, restarts are not.",
				},
			},
			Kept:  false,
			Round: 1,
		}},
		Rounds: 1,
	}
}

// testTextPlain is the text output for testResult without the work.
const testTextPlain = `Say it like this:
We keep topping off the oil every morning instead of fixing the leak. The engine
still runs, but each quart costs us a little more, and one day the leak wins.
Let's put the car on the lift this week.

What stands for what:
- topping off the oil -> restarting the service each morning
- the leak -> the memory bug
- putting the car on the lift -> a week to fix the bug

Where it breaks:
A leak drips at a steady rate, but our crashes come in bursts after big
releases.

World: cars
`

// testTextSkeleton is the skeleton part of the work section for testResult, through the
// candidates header.
const testTextSkeleton = `
Skeleton:
Players:
- the ops team: restarts the service
- leadership: decides what gets fixed
Constraint: two engineers this quarter
Tradeoff: a week on the fix delays the next feature
Cost of inaction: outages grow until a restart no longer helps
Decision: spend a week on the fix now
Point: the restarts cost more than the fix
Cause and effect:
1. A memory bug leaks memory.
2. The service crashes overnight.
3. Someone restarts it each morning.

Candidates:
`

// testTextCandidates is the candidates list of the work section for testResult.
const testTextCandidates = `cars (round 1): kept, fit 4
We keep topping off the oil instead of fixing the leak.
Problems: none

kitchens (round 1): dropped, fit 2
We keep mopping the floor while the sink runs.
Problems:
- A running sink is obvious, the bug is hidden.
- Mopping is cheap, restarts are not.
`

// testTextEmpty is the text output for a zero result with the work shown.
const testTextEmpty = `Say it like this:


What stands for what:
(none)

Where it breaks:


World:

` + testTextEmptyWork

// testTextEmptyWork is the work output for a zero result.
const testTextEmptyWork = `Skeleton:
Players:
(none)
Constraint:
Tradeoff:
Cost of inaction:
Decision:
Point:
Cause and effect:
(none)

Candidates:
(none)
`

// testRecheck returns a check of a polished version of the analogy, kept or dropped.
func testRecheck(kept bool) *engine.Checked {
	c := &engine.Checked{
		Candidate: engine.Candidate{
			World:  "cars",
			Spoken: "We top off the oil each morning. One day the leak wins, so we fix it now.",
		},
		Verdict: engine.Verdict{
			SameShape: true, FamiliarWorld: true, PointsAtDecision: true, Fit: 4,
		},
		Kept:  kept,
		Round: 1,
	}
	if !kept {
		c.Verdict.SameShape, c.Verdict.Fit = false, 2
		c.Verdict.Problems = []string{"The polish dropped the growing cost."}
	}
	return c
}

// testTextRecheckKept is the work output for a polished analogy that passed its check.
const testTextRecheckKept = `
Polished analogy: kept, fit 4
Problems: none
`

// testTextRecheckDropped is the work output for a polished analogy that failed its check. It
// shows the polished text, since the analogy in the result is then the draft.
const testTextRecheckDropped = `
Polished analogy: dropped, fit 2
We top off the oil each morning. One day the leak wins, so we fix it now.
Problems:
- The polish dropped the growing cost.
`

// TestText checks the full text output with and without the work section.
func TestText(t *testing.T) {
	t.Parallel()
	roundless := testResult()
	roundless.Checked = roundless.Checked[:1]
	roundless.Checked[0].Round = 0
	rechecked := testResult()
	rechecked.Recheck = testRecheck(true)
	noted := testResult()
	noted.Note = "kept as is: spoken has 4 sentences, and it needs two or three"
	tests := []struct {
		In         engine.Result
		WantResult string
		ShowWork   bool
	}{{ // Test 0: The default output has the four required parts and nothing else.
		In: testResult(), ShowWork: false, WantResult: testTextPlain,
	}, { // Test 1: Show work appends the skeleton and every candidate.
		In: testResult(), ShowWork: true,
		WantResult: testTextPlain + testTextSkeleton + testTextCandidates,
	}, { // Test 2: Empty lists print a placeholder instead of nothing.
		In: engine.Result{}, ShowWork: true, WantResult: testTextEmpty,
	}, { // Test 3: A candidate with no round number leaves the round out.
		In: roundless, ShowWork: true,
		WantResult: testTextPlain + testTextSkeleton +
			"cars: kept, fit 4\n" +
			"We keep topping off the oil instead of fixing the leak.\n" +
			"Problems: none\n",
	}, { // Test 4: Show work adds the check of the polished analogy after the candidates.
		In: rechecked, ShowWork: true,
		WantResult: testTextPlain + testTextSkeleton + testTextCandidates + testTextRecheckKept,
	}, { // Test 5: Without show work the check of the polished analogy stays out.
		In: rechecked, ShowWork: false, WantResult: testTextPlain,
	}, { // Test 6: The note is not part of the text, since the command prints it on stderr.
		In: noted, ShowWork: true,
		WantResult: testTextPlain + testTextSkeleton + testTextCandidates,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			if err := Text(&buf, test.In, test.ShowWork, NewStyle(false)); err != nil {
				t.Fatalf("Text: %v", err)
			}
			if diff := cmp.Diff(test.WantResult, buf.String()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestWork checks the work section on its own, as printed when no analogy held up.
func TestWork(t *testing.T) {
	t.Parallel()
	noWinner := testResult()
	noWinner.Analogy = engine.Analogy{}
	polishKept := testResult()
	polishKept.Recheck = testRecheck(true)
	polishDropped := testResult()
	polishDropped.Recheck = testRecheck(false)
	tests := []struct {
		In         engine.Result
		WantResult string
	}{{ // Test 0: The work prints without a leading blank line or the analogy.
		In: noWinner, WantResult: testTextSkeleton[1:] + testTextCandidates,
	}, { // Test 1: An empty result prints placeholders.
		In: engine.Result{}, WantResult: testTextEmptyWork,
	}, { // Test 2: A polished analogy that passed its check is listed with its fit.
		In:         polishKept,
		WantResult: testTextSkeleton[1:] + testTextCandidates + testTextRecheckKept,
	}, { // Test 3: A polished analogy that failed its check is printed with its problems.
		In:         polishDropped,
		WantResult: testTextSkeleton[1:] + testTextCandidates + testTextRecheckDropped,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			if err := Work(&buf, test.In, NewStyle(false)); err != nil {
				t.Fatalf("Work: %v", err)
			}
			if diff := cmp.Diff(test.WantResult, buf.String()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestWriteError checks that a failing writer surfaces ErrWrite from Text and Work.
func TestWriteError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Write func() error
		Want  error
	}{{ // Test 0: Text.
		Write: func() error { return Text(testFailWriter{}, testResult(), true, NewStyle(false)) }, Want: ErrWrite,
	}, { // Test 1: Work.
		Write: func() error { return Work(testFailWriter{}, testResult(), NewStyle(false)) }, Want: ErrWrite,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			err := test.Write()
			if !errors.Is(err, test.Want) {
				t.Errorf("error = %v, want %v", err, test.Want)
			}
			if !errors.Is(err, errTestWrite) {
				t.Errorf("error = %v, want it to wrap %v", err, errTestWrite)
			}
		})
	}
}

// TestWrap checks line filling, long words, kept line breaks, and rune counting.
func TestWrap(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         string
		WantResult string
		Cols       int
	}{{ // Test 0: Empty input stays empty.
		In: "", Cols: 80, WantResult: "",
	}, { // Test 1: Short text is unchanged.
		In: "short line", Cols: 80, WantResult: "short line",
	}, { // Test 2: Text that exactly fills the width stays on one line.
		In: "aaaa bbbb", Cols: 9, WantResult: "aaaa bbbb",
	}, { // Test 3: One rune over the width moves the last word down.
		In: "aaaa bbbbb", Cols: 9, WantResult: "aaaa\nbbbbb",
	}, { // Test 4: A word longer than the width gets its own line.
		In: "a verylongword b", Cols: 5, WantResult: "a\nverylongword\nb",
	}, { // Test 5: Runs of spaces collapse to one.
		In: "a    b", Cols: 80, WantResult: "a b",
	}, { // Test 6: Existing line breaks are kept.
		In: "a b\nc d", Cols: 80, WantResult: "a b\nc d",
	}, { // Test 7: Width counts runes, not bytes.
		In: "ééé ééé", Cols: 7, WantResult: "ééé ééé",
	}, { // Test 8: Blank lines between paragraphs survive.
		In: "one\n\ntwo", Cols: 80, WantResult: "one\n\ntwo",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(test.WantResult, wrap(test.In, test.Cols)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
