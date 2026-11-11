package engine

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// testChecked returns a checked candidate from world in round, with a passing verdict at fit.
func testChecked(world string, round, fit int, kept bool) Checked {
	return Checked{
		Candidate: Candidate{World: world},
		Verdict:   Verdict{SameShape: true, FamiliarWorld: true, PointsAtDecision: true, Fit: fit},
		Kept:      kept,
		Round:     round,
	}
}

// TestPick checks that the winner is chosen by preference, fit, round, and draft order.
func TestPick(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         []Checked
		Preferred  []string
		WantResult int
	}{{ // Test 0: Nothing checked.
		In: nil, WantResult: -1,
	}, { // Test 1: Nothing kept.
		In:         []Checked{testChecked("cars", 1, 5, false)},
		WantResult: -1,
	}, { // Test 2: The higher fit wins.
		In:         []Checked{testChecked("cars", 1, 3, true), testChecked("sports", 1, 5, true)},
		WantResult: 1,
	}, { // Test 3: A preferred world beats a higher fit.
		In: []Checked{
			testChecked("cars", 1, 5, true), testChecked("gardening", 1, 3, true),
		},
		Preferred:  []string{"gardening"},
		WantResult: 1,
	}, { // Test 4: The earlier preferred world wins, whatever the fit.
		In: []Checked{
			testChecked("boats", 1, 5, true), testChecked("trains", 1, 3, true),
		},
		Preferred:  []string{"trains", "boats"},
		WantResult: 1,
	}, { // Test 5: Preferred worlds match without regard to case.
		In:         []Checked{testChecked("cars", 1, 5, true), testChecked("Boats", 1, 3, true)},
		Preferred:  []string{"boats"},
		WantResult: 1,
	}, { // Test 6: An unkept preferred world does not win.
		In:         []Checked{testChecked("boats", 1, 5, false), testChecked("cars", 1, 3, true)},
		Preferred:  []string{"boats"},
		WantResult: 1,
	}, { // Test 7: A fit tie goes to the earlier round.
		In:         []Checked{testChecked("cars", 2, 4, true), testChecked("sports", 1, 4, true)},
		WantResult: 1,
	}, { // Test 8: A full tie goes to the earlier draft.
		In:         []Checked{testChecked("cars", 1, 4, true), testChecked("sports", 1, 4, true)},
		WantResult: 0,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(test.WantResult, pick(test.In, test.Preferred)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestClosest checks which dropped candidate counts as nearest to being kept.
func TestClosest(t *testing.T) {
	t.Parallel()
	shapeOnly := Checked{Verdict: Verdict{SameShape: true, Fit: 2}}
	twoChecks := Checked{Verdict: Verdict{SameShape: true, FamiliarWorld: true, Fit: 1}}
	unchecked := Checked{Verdict: Verdict{Problems: []string{"no verdict: boom"}}}
	tests := []struct {
		In         []Checked
		WantResult int
	}{{ // Test 0: Nothing checked.
		In: nil, WantResult: -1,
	}, { // Test 1: Failed checks never count.
		In: []Checked{unchecked, unchecked}, WantResult: -1,
	}, { // Test 2: More checks passed beats a higher fit.
		In: []Checked{shapeOnly, twoChecks}, WantResult: 1,
	}, { // Test 3: With equal checks passed, the higher fit wins.
		In: []Checked{
			{Verdict: Verdict{SameShape: true, Fit: 1}}, shapeOnly,
		},
		WantResult: 1,
	}, { // Test 4: A full tie goes to the earlier draft, and failed checks are skipped.
		In: []Checked{unchecked, shapeOnly, shapeOnly}, WantResult: 1,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(test.WantResult, closest(test.In, 3)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestFirstProblem checks the problem named for a verdict.
func TestFirstProblem(t *testing.T) {
	t.Parallel()
	pass := Verdict{SameShape: true, FamiliarWorld: true, PointsAtDecision: true, Fit: 4}
	tests := []struct {
		In         Verdict
		WantResult string
	}{{ // Test 0: The checker's first problem comes first.
		In:         Verdict{Fit: 1, Problems: []string{"the leak is visible", "too long"}},
		WantResult: "the leak is visible",
	}, { // Test 1: A shape failure with no listed problem.
		In:         Verdict{FamiliarWorld: true, PointsAtDecision: true, Fit: 4},
		WantResult: "the cause and effect do not match the real situation",
	}, { // Test 2: An unfamiliar world with no listed problem.
		In:         Verdict{SameShape: true, PointsAtDecision: true, Fit: 4},
		WantResult: "the audience may not know this world",
	}, { // Test 3: No pointer at the decision with no listed problem.
		In:         Verdict{SameShape: true, FamiliarWorld: true, Fit: 4},
		WantResult: "it does not point at the decision",
	}, { // Test 4: A low fit with no listed problem.
		In:         Verdict{SameShape: true, FamiliarWorld: true, PointsAtDecision: true, Fit: 2},
		WantResult: "fit 2 is below the minimum of 3",
	}, { // Test 5: Nothing wrong.
		In: pass, WantResult: "",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(test.WantResult, firstProblem(test.In, 3)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestDroppedReasons checks the lines that tell later rounds why drafts were dropped.
func TestDroppedReasons(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         []Checked
		WantResult []string
	}{{ // Test 0: Nothing checked.
		In: nil, WantResult: nil,
	}, { // Test 1: Kept and unchecked candidates give no reason.
		In: []Checked{
			testChecked("cars", 1, 4, true),
			{Candidate: Candidate{World: "sports"}, Verdict: Verdict{Problems: []string{"x"}}},
		},
		WantResult: nil,
	}, { // Test 2: Each dropped candidate gives its world, what it said, and its first problem.
		In: []Checked{
			{
				Candidate: Candidate{World: "cars", Spoken: "We top off the oil. It leaks."},
				Verdict:   testDropped(),
			},
			testChecked("kitchens", 1, 2, false),
		},
		WantResult: []string{
			`cars: "We top off the oil. It leaks." was dropped because the chain runs backward`,
			`kitchens: "" was dropped because fit 2 is below the minimum of 3`,
		},
	}, { // Test 3: Line breaks in the spoken text collapse, so each reason stays on one line.
		In: []Checked{{
			Candidate: Candidate{World: "sports", Spoken: " We drop\nthe baton.  Every lap. "},
			Verdict:   testDropped(),
		}},
		WantResult: []string{
			`sports: "We drop the baton. Every lap." was dropped because the chain runs backward`,
		},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := droppedReasons(test.In, 3)
			if diff := cmp.Diff(test.WantResult, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestNoFit checks the ErrNoFit message.
func TestNoFit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In          []Checked
		WantMessage string
		Want        error
	}{{ // Test 0: No verdict at all.
		In:          nil,
		WantMessage: "no analogy fit: no check returned a verdict",
		Want:        ErrNoFit,
	}, { // Test 1: The closest candidate is named with its first problem.
		In: []Checked{
			{Candidate: Candidate{World: "cars"}, Verdict: testDropped()},
			{
				Candidate: Candidate{World: "kitchens"},
				Verdict:   Verdict{SameShape: true, FamiliarWorld: true, Fit: 2},
			},
		},
		WantMessage: "no analogy fit: closest was kitchens at fit 2: " +
			"it does not point at the decision",
		Want: ErrNoFit,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			err := noFit(test.In, 3)
			if !errors.Is(err, test.Want) {
				t.Fatalf("got error %v, want %v", err, test.Want)
			}
			if diff := cmp.Diff(test.WantMessage, err.Error()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
