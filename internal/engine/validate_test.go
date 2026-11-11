package engine

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// TestCleanSkeleton checks how a skeleton reply is tidied and judged.
func TestCleanSkeleton(t *testing.T) {
	t.Parallel()
	messy := Skeleton{
		Players: []Player{
			{Name: "  the team ", Role: " patches the pipeline by hand "},
			{Name: "  ", Role: "nobody"},
			{Name: " leadership", Role: " decides where the time goes"},
		},
		Constraint:     " two engineers and a fixed release date ",
		Tradeoff:       " patching keeps releases moving now, and a rebuild costs a sprint ",
		CostOfInaction: " the breaks come more often until a release fails outright ",
		Decision:       " fund a rebuild of the pipeline or keep patching ",
		Point:          " the cheap fix has become the expensive one ",
		Chain: []string{
			" the pipeline breaks", "", "the team patches it ", " ", "the patches pile up",
		},
	}
	oneLink := Skeleton{
		Players:        []Player{{Name: "the team"}},
		CostOfInaction: "the breaks come more often",
		Decision:       "rebuild or keep patching",
		Point:          "the cheap fix is the expensive one",
		Chain:          []string{"the pipeline breaks"},
	}
	tests := []struct {
		In          Skeleton
		WantResult  Skeleton
		WantProblem string
	}{{ // Test 0: A messy but usable skeleton is trimmed, and blank entries are dropped.
		In: messy, WantResult: testSkeleton(), WantProblem: "",
	}, { // Test 1: An empty skeleton names every missing part.
		In:         Skeleton{},
		WantResult: Skeleton{},
		WantProblem: "decision is empty; point is empty; cost_of_inaction is empty; " +
			"players needs at least one entry with a name; " +
			"causal_chain needs at least 2 links, got 0",
	}, { // Test 2: A one-link chain is not enough.
		In:          oneLink,
		WantResult:  oneLink,
		WantProblem: "causal_chain needs at least 2 links, got 1",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := test.In
			problem := cleanSkeleton(&got)
			if diff := cmp.Diff(test.WantProblem, problem); diff != "" {
				t.Errorf("problem mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.WantResult, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("skeleton mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestCleanCandidate checks how a draft reply is tidied and judged.
func TestCleanCandidate(t *testing.T) {
	t.Parallel()
	good := testCandidate()
	good.World = "cars"
	messy := testCandidate()
	messy.Spoken = "  " + messy.Spoken + "\n"
	messy.Mappings = append([]Mapping{{Analogy: "half", Real: " "}}, messy.Mappings...)
	messy.DecisionLine = " fix the leak "
	tests := []struct {
		In          Candidate
		World       string
		WantResult  Candidate
		WantProblem string
	}{{ // Test 0: A usable draft takes the requested world and is trimmed.
		In: messy, World: "cars", WantResult: good, WantProblem: "",
	}, { // Test 1: An empty draft names every missing part.
		In:         Candidate{World: "boats"},
		World:      "cars",
		WantResult: Candidate{World: "cars"},
		WantProblem: "spoken is empty; mappings needs at least 2 complete entries, got 0; " +
			"decision_line is empty; breaks_down is empty",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := test.In
			problem := cleanCandidate(&got, test.World)
			if diff := cmp.Diff(test.WantProblem, problem); diff != "" {
				t.Errorf("problem mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.WantResult, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("candidate mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestCleanVerdict checks that a verdict's fit is clamped and its text trimmed.
func TestCleanVerdict(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         Verdict
		WantResult Verdict
	}{{ // Test 0: A fit above the scale drops to 5.
		In: Verdict{Fit: 9}, WantResult: Verdict{Fit: 5},
	}, { // Test 1: A fit below the scale rises to 1.
		In: Verdict{Fit: -2}, WantResult: Verdict{Fit: 1},
	}, { // Test 2: A missing fit rises to 1.
		In: Verdict{}, WantResult: Verdict{Fit: 1},
	}, { // Test 3: Problems and the fix are trimmed, and blank problems go.
		In:         Verdict{Fit: 3, Problems: []string{" a ", "", "b"}, Fix: " fix it "},
		WantResult: Verdict{Fit: 3, Problems: []string{"a", "b"}, Fix: "fix it"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := test.In
			if problem := cleanVerdict(&got); problem != "" {
				t.Errorf("unexpected problem %q", problem)
			}
			if diff := cmp.Diff(test.WantResult, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestAnalogyProblems checks what the final step treats as missing and as the wrong length.
func TestAnalogyProblems(t *testing.T) {
	t.Parallel()
	mappings := testAnalogy().Mappings
	tests := []struct {
		In          Analogy
		WantMissing []string
		WantLength  string
	}{{ // Test 0: A complete analogy of three sentences.
		In: testAnalogy(), WantMissing: nil, WantLength: "",
	}, { // Test 1: Two sentences are fine.
		In:          Analogy{Spoken: "One. Two.", Mappings: mappings, BreaksDown: "b"},
		WantMissing: nil, WantLength: "",
	}, { // Test 2: One sentence is too short.
		In:          Analogy{Spoken: "Just one.", Mappings: mappings, BreaksDown: "b"},
		WantMissing: nil, WantLength: "spoken has 1 sentence, and it needs two or three",
	}, { // Test 3: Four sentences are too long.
		In:          Analogy{Spoken: "One. Two. Three. Four.", Mappings: mappings, BreaksDown: "b"},
		WantMissing: nil, WantLength: "spoken has 4 sentences, and it needs two or three",
	}, { // Test 4: Missing parts are listed, and an empty spoken line is not a length problem.
		In: Analogy{Mappings: mappings[:1]},
		WantMissing: []string{
			"spoken is empty", "mappings needs at least 2 complete entries, got 1",
			"breaks_down is empty",
		},
		WantLength: "",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			missing, length := analogyProblems(test.In)
			if diff := cmp.Diff(test.WantMissing, missing, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("missing mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.WantLength, length); diff != "" {
				t.Errorf("length mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestCleanAnalogy checks that a final reply takes the winner's world and is trimmed.
func TestCleanAnalogy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         Analogy
		World      string
		WantResult Analogy
	}{{ // Test 0: The world is forced, text is trimmed, and half mappings go.
		In: Analogy{
			World:      "boats",
			Spoken:     " One. Two. ",
			Mappings:   []Mapping{{Analogy: " oil ", Real: " patches "}, {Analogy: "leak"}},
			BreaksDown: " b ",
		},
		World: "cars",
		WantResult: Analogy{
			World:      "cars",
			Spoken:     "One. Two.",
			Mappings:   []Mapping{{Analogy: "oil", Real: "patches"}},
			BreaksDown: "b",
		},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := test.In
			cleanAnalogy(&got, test.World)
			if diff := cmp.Diff(test.WantResult, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
