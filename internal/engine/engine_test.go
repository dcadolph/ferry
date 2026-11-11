package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/dcadolph/ferry/internal/schema"
)

// errTestModel is the error the scripted model returns when told to fail.
var errTestModel = errors.New("test model failure")

// testSummary is the summary the Run tests send.
const testSummary = "Our deploy pipeline breaks every week, and the team patches it by hand."

// testReply is one scripted answer from testModel.
type testReply struct {
	// Text is the reply text, returned when Err is nil and Cancel is false.
	Text string
	// Err is returned in place of a reply when set.
	Err error
	// Cancel cancels the context of the run under test and returns the context's error.
	Cancel bool
}

// promptWant names a prompt a test expects, or does not expect, the model to receive.
type promptWant struct {
	// Step is the step of the request.
	Step Step
	// World is the world named in the prompt, or empty to match any world.
	World string
	// Text is the text the prompt contains.
	Text string
}

// testModel is a scripted Completer. It answers by key: the step name for the skeleton, final,
// and recheck steps, and the step and world for drafts and checks, such as "draft/cars", with
// "draft/*" as the fallback. Once a key's replies run out, the last one repeats.
type testModel struct {
	// replies maps a key to its replies in call order.
	replies map[string][]testReply
	// cancel cancels the context of the run under test.
	cancel context.CancelFunc
	// mu guards the fields below.
	mu sync.Mutex
	// calls counts the calls made for each key.
	calls map[string]int
	// requests records every request in arrival order.
	requests []Request
	// running counts the calls in progress.
	running int
	// peak is the most calls seen in progress at once.
	peak int
}

// newTestModel returns a testModel that answers from replies.
func newTestModel(replies map[string][]testReply) *testModel {
	return &testModel{replies: replies, calls: map[string]int{}}
}

// Complete answers req from the script.
func (m *testModel) Complete(ctx context.Context, req Request) (string, error) {
	key := string(req.Step)
	if req.Step == StepDraft || req.Step == StepCheck {
		key += "/" + promptWorld(req.Prompt)
	}
	m.mu.Lock()
	m.requests = append(m.requests, req)
	m.running++
	m.peak = max(m.peak, m.running)
	list, ok := m.replies[key]
	if !ok {
		list = m.replies[string(req.Step)+"/*"]
	}
	n := m.calls[key]
	m.calls[key]++
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.running--
		m.mu.Unlock()
	}()
	runtime.Gosched()
	if len(list) == 0 {
		return "", fmt.Errorf("no reply scripted for %s", key)
	}
	reply := list[min(n, len(list)-1)]
	if reply.Cancel {
		m.cancel()
		return "", ctx.Err()
	}
	return reply.Text, reply.Err
}

// callCount returns how many calls the model received.
func (m *testModel) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.requests)
}

// prompted reports whether any request the model received matches want.
func (m *testModel) prompted(want promptWant) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, req := range m.requests {
		if req.Step != want.Step || (want.World != "" && promptWorld(req.Prompt) != want.World) {
			continue
		}
		if strings.Contains(req.Prompt, want.Text) {
			return true
		}
	}
	return false
}

// testEvents collects progress events, which arrive from several goroutines.
type testEvents struct {
	// mu guards list.
	mu sync.Mutex
	// list holds the events in arrival order.
	list []Event
}

// add records ev.
func (e *testEvents) add(ev Event) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.list = append(e.list, ev)
}

// promptWorld returns the text inside the first world tag of prompt.
func promptWorld(prompt string) string {
	_, rest, ok := strings.Cut(prompt, "<world>")
	if !ok {
		return ""
	}
	world, _, _ := strings.Cut(rest, "</world>")
	return world
}

// jsonReply returns a reply holding v as JSON.
func jsonReply(t *testing.T, v any) testReply {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal reply: %v", err)
	}
	return testReply{Text: string(b)}
}

// testScript returns replies that let every step succeed, with every check and the re-check
// kept at fit 4, and then applies overrides.
func testScript(t *testing.T, overrides map[string][]testReply) map[string][]testReply {
	t.Helper()
	replies := map[string][]testReply{
		"skeleton": {jsonReply(t, testSkeleton())},
		"draft/*":  {jsonReply(t, testCandidate())},
		"check/*":  {jsonReply(t, testVerdict(4))},
		"final":    {jsonReply(t, testAnalogy())},
		"recheck":  {jsonReply(t, testVerdict(4))},
	}
	maps.Copy(replies, overrides)
	return replies
}

// testSkeleton returns a usable skeleton.
func testSkeleton() Skeleton {
	return Skeleton{
		Players: []Player{
			{Name: "the team", Role: "patches the pipeline by hand"},
			{Name: "leadership", Role: "decides where the time goes"},
		},
		Constraint:     "two engineers and a fixed release date",
		Tradeoff:       "patching keeps releases moving now, and a rebuild costs a sprint",
		CostOfInaction: "the breaks come more often until a release fails outright",
		Decision:       "fund a rebuild of the pipeline or keep patching",
		Point:          "the cheap fix has become the expensive one",
		Chain: []string{
			"the pipeline breaks", "the team patches it", "the patches pile up",
		},
	}
}

// testCandidate returns a usable draft. Its world is wrong on purpose, since Run sets it.
func testCandidate() Candidate {
	return Candidate{
		World:  "boats",
		Spoken: "We keep topping off the oil instead of fixing the leak. Each top-off buys a week.",
		Mappings: []Mapping{
			{Analogy: "topping off the oil", Real: "patching the pipeline"},
			{Analogy: "the leak", Real: "the broken pipeline"},
		},
		DecisionLine: "fix the leak",
		BreaksDown:   "a mechanic can see a leak, and nobody can see the pipeline wear out",
	}
}

// testVerdict returns a verdict that passes every check at fit.
func testVerdict(fit int) Verdict {
	return Verdict{
		SameShape: true, FamiliarWorld: true, PointsAtDecision: true,
		Fit: fit, Fix: "name the leak",
	}
}

// testProblem is the problem every dropped test verdict names.
const testProblem = "the chain runs backward"

// testDropped returns a verdict that fails the shape check and names testProblem.
func testDropped() Verdict {
	return Verdict{FamiliarWorld: true, Fit: 2, Problems: []string{testProblem}, Fix: "start over"}
}

// testAnalogy returns a usable final analogy. Its world is wrong on purpose, since Run sets it.
func testAnalogy() Analogy {
	return Analogy{
		World: "boats",
		Spoken: "We keep topping off the oil instead of fixing the leak. " +
			"Every top-off hides more damage underneath. It is time to put the car in the shop.",
		Mappings: []Mapping{
			{Analogy: "topping off the oil", Real: "patching the pipeline"},
			{Analogy: "the leak", Real: "the broken pipeline"},
			{Analogy: "the shop", Real: "a funded rebuild"},
		},
		BreaksDown: "a mechanic can see a leak, and nobody can see the pipeline wear out",
	}
}

// testFinal returns testAnalogy with its world set to world, as Run returns it.
func testFinal(world string) Analogy {
	a := testAnalogy()
	a.World = world
	return a
}

// testDraftAnalogy returns testCandidate as a final analogy from world, word for word, as Run
// returns it when the checked draft stands in for the polished text.
func testDraftAnalogy(world string) Analogy {
	c := testCandidate()
	return Analogy{World: world, Spoken: c.Spoken, Mappings: c.Mappings, BreaksDown: c.BreaksDown}
}

// recheckLine describes the check of the polished analogy as its outcome, or "" when there
// was none.
func recheckLine(c *Checked) string {
	switch {
	case c == nil:
		return ""
	case c.Kept:
		return "kept"
	case c.Verdict.Fit == 0:
		return "unchecked"
	default:
		return "dropped"
	}
}

// checkedLines describes each checked candidate as its round, world, and outcome.
func checkedLines(checked []Checked) []string {
	lines := make([]string, 0, len(checked))
	for _, c := range checked {
		outcome := "dropped"
		switch {
		case c.Kept:
			outcome = "kept"
		case c.Verdict.Fit == 0:
			outcome = "unchecked"
		}
		lines = append(lines, fmt.Sprintf("%d %s %s", c.Round, c.Candidate.World, outcome))
	}
	return lines
}

// TestDefaultConfig checks the default settings.
func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	ignoreProgress := cmpopts.IgnoreFields(Config{}, "Progress")
	tests := []struct {
		WantResult Config
	}{{ // Test 0: The documented defaults.
		WantResult: Config{
			Worlds: []string{"cars", "kitchens", "construction", "sports"},
			FallbackWorlds: []string{
				"home plumbing", "gardening", "air travel", "household budgets",
			},
			MaxWorlds: 6,
			Rounds:    2,
			Workers:   4,
			MinFit:    3,
		},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := DefaultConfig()
			if diff := cmp.Diff(test.WantResult, got, ignoreProgress); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestNewWithConfig checks that unset settings take their defaults and set ones stay.
func TestNewWithConfig(t *testing.T) {
	t.Parallel()
	def := DefaultConfig()
	ignoreProgress := cmpopts.IgnoreFields(Config{}, "Progress")
	tests := []struct {
		In           Config
		WantResult   Config
		WantProgress bool
	}{{ // Test 0: A zero config takes every default.
		In: Config{}, WantResult: def,
	}, { // Test 1: Set fields stay, and the rest take defaults.
		In: Config{Worlds: []string{"boats"}, Workers: 2, MinFit: 4},
		WantResult: Config{
			Worlds:         []string{"boats"},
			FallbackWorlds: def.FallbackWorlds,
			MaxWorlds:      def.MaxWorlds,
			Rounds:         def.Rounds,
			Workers:        2,
			MinFit:         4,
		},
	}, { // Test 2: Numbers below one take defaults.
		In:         Config{MaxWorlds: -1, Rounds: -3, Workers: -2, MinFit: -4},
		WantResult: def,
	}, { // Test 3: A progress function stays.
		In: Config{Progress: func(Event) {}}, WantResult: def, WantProgress: true,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := NewWithConfig(newTestModel(nil), test.In).cfg
			if diff := cmp.Diff(test.WantResult, got, ignoreProgress); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.WantProgress, got.Progress != nil); diff != "" {
				t.Errorf("progress mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestNewPanics checks that New panics without a completer.
func TestNewPanics(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In          Completer
		WantMessage string
	}{{ // Test 0: A nil completer.
		In: nil, WantMessage: "Completer required",
	}, { // Test 1: A nil CompleterFunc.
		In: CompleterFunc(nil), WantMessage: "Completer required",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("New did not panic")
				}
				if msg := fmt.Sprint(r); !strings.Contains(msg, test.WantMessage) {
					t.Errorf("panic %q does not mention %q", msg, test.WantMessage)
				}
			}()
			New(test.In)
		})
	}
}

// TestRun checks Run end to end against a scripted model.
func TestRun(t *testing.T) {
	t.Parallel()
	in := Input{Summary: testSummary}
	noDecision := testSkeleton()
	noDecision.Decision = ""
	oneLink := testSkeleton()
	oneLink.Chain = oneLink.Chain[:1]
	oneMapping := testCandidate()
	oneMapping.Mappings = oneMapping.Mappings[:1]
	noBreak := testAnalogy()
	noBreak.BreaksDown = ""
	long := testAnalogy()
	long.Spoken += " We should book it this week."
	longFinal := long
	longFinal.World = "cars"
	nearMiss := Verdict{
		SameShape: true, FamiliarWorld: true, Fit: 2,
		Problems: []string{"it never reaches the decision"},
	}
	picky := testVerdict(5)
	picky.Problems = []string{"the crew is never named"}
	bad := testReply{Text: "{not json"}
	var scratch Analogy
	notJSON := "the reply is not valid JSON for the schema: " +
		json.Unmarshal([]byte(bad.Text), &scratch).Error()
	fail := testReply{Err: errTestModel}
	dropped := []testReply{jsonReply(t, testDropped())}
	allKept := []string{"1 cars kept", "1 kitchens kept", "1 construction kept", "1 sports kept"}
	allDropped := []string{
		"1 cars dropped", "1 kitchens dropped", "1 construction dropped", "1 sports dropped",
	}
	droppedLine := func(world string) string {
		return "- " + world + `: "` + testCandidate().Spoken + `" was dropped because ` +
			testProblem + "\n"
	}
	const (
		keptAsIs     = "kept as is: spoken has 4 sentences, and it needs two or three"
		polishFailed = "polish failed, this is the checked draft: "
		recheckNo    = "the polished analogy did not pass its check, this is the checked draft: "
		recheckError = "the check of the polished analogy failed, this is the checked draft: "
	)

	tests := []struct {
		Replies      map[string][]testReply
		Input        Input
		Config       Config
		WantAnalogy  Analogy
		WantSkeleton Skeleton
		WantChecked  []string
		WantPrompts  []promptWant
		WantAbsent   []promptWant
		WantRecheck  string
		WantNote     string
		WantRounds   int
		WantCalls    int
		WantErrText  string
		Want         error
	}{{ // Test 0: Every step succeeds, the best fit wins, the final prompt carries the winner's
		// fit and first problem, and the re-check sees the polished text with the draft's
		// decision line.
		Replies: testScript(t, map[string][]testReply{
			"check/construction": {jsonReply(t, picky)},
		}),
		Input:        in,
		WantAnalogy:  testFinal("construction"),
		WantSkeleton: testSkeleton(),
		WantChecked:  allKept,
		WantPrompts: []promptWant{{
			Step: StepFinal, World: "construction",
			Text: "<check>\nFit: 5 of 5\nProblems:\n- the crew is never named\n" +
				"Suggested fix: name the leak\n</check>",
		}, {
			Step: StepRecheck, World: "construction",
			Text: "Spoken: " + testAnalogy().Spoken + "\n",
		}, {
			Step: StepRecheck, World: "construction", Text: "Decision line: fix the leak\n",
		}},
		WantRecheck: "kept",
		WantRounds:  1,
		WantCalls:   11,
	}, { // Test 1: A preferred world beats a higher fit.
		Replies: testScript(t, map[string][]testReply{
			"check/*":         {jsonReply(t, testVerdict(5))},
			"check/Gardening": {jsonReply(t, testVerdict(3))},
		}),
		Input:        Input{Summary: testSummary, Worlds: []string{" Gardening "}},
		WantAnalogy:  testFinal("Gardening"),
		WantSkeleton: testSkeleton(),
		WantChecked:  append([]string{"1 Gardening kept"}, allKept...),
		WantRecheck:  "kept",
		WantRounds:   1,
		WantCalls:    13,
	}, { // Test 2: Round two runs after every draft is dropped, and its prompts show each
		// dropped draft with the reason.
		Replies: testScript(t, map[string][]testReply{
			"check/cars":         dropped,
			"check/kitchens":     dropped,
			"check/construction": dropped,
			"check/sports":       dropped,
		}),
		Input:        in,
		WantAnalogy:  testFinal("home plumbing"),
		WantSkeleton: testSkeleton(),
		WantChecked: append(slices.Clone(allDropped),
			"2 home plumbing kept", "2 gardening kept", "2 air travel kept",
			"2 household budgets kept"),
		WantPrompts: []promptWant{{
			Step:  StepDraft,
			World: "gardening",
			Text: "<dropped>\n" + droppedLine("cars") + droppedLine("kitchens") +
				droppedLine("construction") + droppedLine("sports") + "</dropped>",
		}},
		WantAbsent:  []promptWant{{Step: StepDraft, World: "cars", Text: "<dropped>"}},
		WantRecheck: "kept",
		WantRounds:  2,
		WantCalls:   19,
	}, { // Test 3: Nothing holds up in either round, and the error names the closest draft.
		Replies: testScript(t, map[string][]testReply{
			"check/*":        dropped,
			"check/kitchens": {jsonReply(t, nearMiss)},
		}),
		Input:        in,
		WantSkeleton: testSkeleton(),
		WantChecked: append(slices.Clone(allDropped),
			"2 home plumbing dropped", "2 gardening dropped", "2 air travel dropped",
			"2 household budgets dropped"),
		WantRounds:  2,
		WantCalls:   17,
		WantErrText: "closest was kitchens at fit 2: it never reaches the decision",
		Want:        ErrNoFit,
	}, { // Test 4: A skeleton missing its decision gets one retry that names the problem.
		Replies: testScript(t, map[string][]testReply{
			"skeleton": {jsonReply(t, noDecision), jsonReply(t, testSkeleton())},
		}),
		Input:        in,
		WantAnalogy:  testFinal("cars"),
		WantSkeleton: testSkeleton(),
		WantChecked:  allKept,
		WantPrompts: []promptWant{{
			Step: StepSkeleton,
			Text: "An earlier reply to this request could not be used: decision is empty.",
		}},
		WantRecheck: "kept",
		WantRounds:  1,
		WantCalls:   12,
	}, { // Test 5: A skeleton that is bad twice gives ErrBadReply.
		Replies: testScript(t, map[string][]testReply{
			"skeleton": {jsonReply(t, oneLink)},
		}),
		Input:       in,
		WantCalls:   2,
		WantErrText: "causal_chain needs at least 2 links, got 1",
		Want:        ErrBadReply,
	}, { // Test 6: A skeleton reply that is not JSON twice gives ErrBadReply.
		Replies:     testScript(t, map[string][]testReply{"skeleton": {bad}}),
		Input:       in,
		WantCalls:   2,
		WantErrText: "not valid JSON",
		Want:        ErrBadReply,
	}, { // Test 7: A failed skeleton call stops the run with the call's own error.
		Replies:   testScript(t, map[string][]testReply{"skeleton": {fail}}),
		Input:     in,
		WantCalls: 1,
		Want:      errTestModel,
	}, { // Test 8: One failed draft does not sink the round.
		Replies:      testScript(t, map[string][]testReply{"draft/kitchens": {bad}}),
		Input:        in,
		WantAnalogy:  testFinal("cars"),
		WantSkeleton: testSkeleton(),
		WantChecked:  []string{"1 cars kept", "1 construction kept", "1 sports kept"},
		WantRecheck:  "kept",
		WantRounds:   1,
		WantCalls:    11,
	}, { // Test 9: A draft with one mapping gets one retry that names the problem.
		Replies: testScript(t, map[string][]testReply{
			"draft/cars": {jsonReply(t, oneMapping), jsonReply(t, testCandidate())},
		}),
		Input:        in,
		WantAnalogy:  testFinal("cars"),
		WantSkeleton: testSkeleton(),
		WantChecked:  allKept,
		WantPrompts: []promptWant{{
			Step:  StepDraft,
			World: "cars",
			Text:  "mappings needs at least 2 complete entries, got 1",
		}},
		WantRecheck: "kept",
		WantRounds:  1,
		WantCalls:   12,
	}, { // Test 10: Every draft failing to decode returns ErrBadReply.
		Replies:      testScript(t, map[string][]testReply{"draft/*": {bad}}),
		Input:        in,
		WantSkeleton: testSkeleton(),
		WantRounds:   1,
		WantCalls:    9,
		WantErrText:  "every draft in round 1 failed",
		Want:         ErrBadReply,
	}, { // Test 11: Every draft call failing returns the call's error, naming each world.
		Replies:      testScript(t, map[string][]testReply{"draft/*": {fail}}),
		Input:        in,
		WantSkeleton: testSkeleton(),
		WantRounds:   1,
		WantCalls:    5,
		WantErrText: "every draft in round 1 failed: " +
			"cars: model call: draft step: test model failure\n" +
			"kitchens: model call: draft step: test model failure",
		Want: errTestModel,
	}, { // Test 12: A failed check leaves its candidate unkept, and the rest go on.
		Replies:      testScript(t, map[string][]testReply{"check/cars": {fail}}),
		Input:        in,
		WantAnalogy:  testFinal("kitchens"),
		WantSkeleton: testSkeleton(),
		WantChecked: []string{
			"1 cars unchecked", "1 kitchens kept", "1 construction kept", "1 sports kept",
		},
		WantRecheck: "kept",
		WantRounds:  1,
		WantCalls:   11,
	}, { // Test 13: Every check failing returns the call's error.
		Replies:      testScript(t, map[string][]testReply{"check/*": {fail}}),
		Input:        in,
		WantSkeleton: testSkeleton(),
		WantChecked: []string{
			"1 cars unchecked", "1 kitchens unchecked", "1 construction unchecked",
			"1 sports unchecked",
		},
		WantRounds:  1,
		WantCalls:   9,
		WantErrText: "every check in round 1 failed",
		Want:        errTestModel,
	}, { // Test 14: A final missing breaks_down twice falls back to the checked draft.
		Replies:      testScript(t, map[string][]testReply{"final": {jsonReply(t, noBreak)}}),
		Input:        in,
		WantAnalogy:  testDraftAnalogy("cars"),
		WantSkeleton: testSkeleton(),
		WantChecked:  allKept,
		WantPrompts: []promptWant{{
			Step: StepFinal, Text: "could not be used: breaks_down is empty.",
		}},
		WantNote:   polishFailed + "incomplete analogy: final step: breaks_down is empty",
		WantRounds: 1,
		WantCalls:  11,
	}, { // Test 15: A final missing breaks_down once is fixed by the retry.
		Replies: testScript(t, map[string][]testReply{
			"final": {jsonReply(t, noBreak), jsonReply(t, testAnalogy())},
		}),
		Input:        in,
		WantAnalogy:  testFinal("cars"),
		WantSkeleton: testSkeleton(),
		WantChecked:  allKept,
		WantRecheck:  "kept",
		WantRounds:   1,
		WantCalls:    12,
	}, { // Test 16: A final that runs long twice is kept as it is, with a note.
		Replies:      testScript(t, map[string][]testReply{"final": {jsonReply(t, long)}}),
		Input:        in,
		WantAnalogy:  longFinal,
		WantSkeleton: testSkeleton(),
		WantChecked:  allKept,
		WantPrompts: []promptWant{{
			Step: StepFinal, Text: "spoken has 4 sentences, and it needs two or three",
		}, {
			Step: StepRecheck, Text: "Spoken: " + long.Spoken + "\n",
		}},
		WantRecheck: "kept",
		WantNote:    keptAsIs,
		WantRounds:  1,
		WantCalls:   12,
	}, { // Test 17: A final reply that is not JSON twice falls back to the checked draft.
		Replies:      testScript(t, map[string][]testReply{"final": {bad}}),
		Input:        in,
		WantAnalogy:  testDraftAnalogy("cars"),
		WantSkeleton: testSkeleton(),
		WantChecked:  allKept,
		WantNote:     polishFailed + "bad model reply: final step: " + notJSON,
		WantRounds:   1,
		WantCalls:    11,
	}, { // Test 18: A blank summary gives ErrNoSummary without a call.
		Replies: testScript(t, nil),
		Input:   Input{Summary: " \n\t "},
		Want:    ErrNoSummary,
	}, { // Test 19: One round only, so nothing kept means ErrNoFit after it.
		Replies:      testScript(t, map[string][]testReply{"check/*": dropped}),
		Input:        in,
		Config:       Config{Rounds: 1},
		WantSkeleton: testSkeleton(),
		WantChecked:  allDropped,
		WantRounds:   1,
		WantCalls:    9,
		WantErrText:  "closest was cars at fit 2: the chain runs backward",
		Want:         ErrNoFit,
	}, { // Test 20: Configured worlds are capped at MaxWorlds.
		Replies:      testScript(t, nil),
		Input:        in,
		Config:       Config{Worlds: []string{"boats", "trains", "planes"}, MaxWorlds: 2},
		WantAnalogy:  testFinal("boats"),
		WantSkeleton: testSkeleton(),
		WantChecked:  []string{"1 boats kept", "1 trains kept"},
		WantRecheck:  "kept",
		WantRounds:   1,
		WantCalls:    7,
	}, { // Test 21: No unused fallback world means no second round.
		Replies:      testScript(t, map[string][]testReply{"check/*": dropped}),
		Input:        in,
		Config:       Config{FallbackWorlds: []string{"Cars", "sports"}},
		WantSkeleton: testSkeleton(),
		WantChecked:  allDropped,
		WantRounds:   1,
		WantCalls:    9,
		Want:         ErrNoFit,
	}, { // Test 22: A long final whose retry call fails is still kept, not thrown away.
		Replies: testScript(t, map[string][]testReply{
			"final": {jsonReply(t, long), fail},
		}),
		Input:        in,
		WantAnalogy:  longFinal,
		WantSkeleton: testSkeleton(),
		WantChecked:  allKept,
		WantRecheck:  "kept",
		WantNote:     keptAsIs,
		WantRounds:   1,
		WantCalls:    12,
	}, { // Test 23: A long final whose retry comes back with a part missing is still kept.
		Replies: testScript(t, map[string][]testReply{
			"final": {jsonReply(t, long), jsonReply(t, noBreak)},
		}),
		Input:        in,
		WantAnalogy:  longFinal,
		WantSkeleton: testSkeleton(),
		WantChecked:  allKept,
		WantRecheck:  "kept",
		WantNote:     keptAsIs,
		WantRounds:   1,
		WantCalls:    12,
	}, { // Test 24: A failed final call falls back to the checked draft, word for word.
		Replies:      testScript(t, map[string][]testReply{"final": {fail}}),
		Input:        in,
		WantAnalogy:  testDraftAnalogy("cars"),
		WantSkeleton: testSkeleton(),
		WantChecked:  allKept,
		WantNote:     polishFailed + "model call: final step: test model failure",
		WantRounds:   1,
		WantCalls:    10,
	}, { // Test 25: A polished text that fails the re-check gives way to the checked draft.
		Replies:      testScript(t, map[string][]testReply{"recheck": dropped}),
		Input:        in,
		Config:       Config{Worlds: []string{"boats", "trains"}},
		WantAnalogy:  testDraftAnalogy("boats"),
		WantSkeleton: testSkeleton(),
		WantChecked:  []string{"1 boats kept", "1 trains kept"},
		WantRecheck:  "dropped",
		WantNote:     recheckNo + testProblem,
		WantRounds:   1,
		WantCalls:    7,
	}, { // Test 26: A re-check call that fails gives way to the checked draft.
		Replies:      testScript(t, map[string][]testReply{"recheck": {fail}}),
		Input:        in,
		WantAnalogy:  testDraftAnalogy("cars"),
		WantSkeleton: testSkeleton(),
		WantChecked:  allKept,
		WantRecheck:  "unchecked",
		WantNote:     recheckError + "model call: recheck step: test model failure",
		WantRounds:   1,
		WantCalls:    11,
	}, { // Test 27: A re-check reply that is not JSON twice gives way to the checked draft.
		Replies:      testScript(t, map[string][]testReply{"recheck": {bad}}),
		Input:        in,
		WantAnalogy:  testDraftAnalogy("cars"),
		WantSkeleton: testSkeleton(),
		WantChecked:  allKept,
		WantRecheck:  "unchecked",
		WantNote:     recheckError + "bad model reply: recheck step: " + notJSON,
		WantRounds:   1,
		WantCalls:    12,
	}, { // Test 28: A re-check that names a problem and still passes keeps the polished text.
		Replies:      testScript(t, map[string][]testReply{"recheck": {jsonReply(t, picky)}}),
		Input:        in,
		WantAnalogy:  testFinal("cars"),
		WantSkeleton: testSkeleton(),
		WantChecked:  allKept,
		WantRecheck:  "kept",
		WantRounds:   1,
		WantCalls:    11,
	}, { // Test 29: A failed model call carries ErrModelCall and names its step.
		Replies:     testScript(t, map[string][]testReply{"skeleton": {fail}}),
		Input:       in,
		WantCalls:   1,
		WantErrText: "model call: skeleton step: test model failure",
		Want:        ErrModelCall,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			model := newTestModel(test.Replies)
			res, err := NewWithConfig(model, test.Config).Run(context.Background(), test.Input)
			if !errors.Is(err, test.Want) {
				t.Fatalf("got error %v, want %v", err, test.Want)
			}
			if err != nil && !strings.Contains(err.Error(), test.WantErrText) {
				t.Errorf("error %q does not contain %q", err, test.WantErrText)
			}
			empty := cmpopts.EquateEmpty()
			if diff := cmp.Diff(test.WantAnalogy, res.Analogy, empty); diff != "" {
				t.Errorf("analogy mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.WantSkeleton, res.Skeleton, empty); diff != "" {
				t.Errorf("skeleton mismatch (-want +got):\n%s", diff)
			}
			got := checkedLines(res.Checked)
			if diff := cmp.Diff(test.WantChecked, got, empty); diff != "" {
				t.Errorf("checked mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.WantRecheck, recheckLine(res.Recheck)); diff != "" {
				t.Errorf("recheck mismatch (-want +got):\n%s", diff)
			}
			if res.Recheck != nil {
				want := testCandidate().DecisionLine
				if diff := cmp.Diff(want, res.Recheck.Candidate.DecisionLine); diff != "" {
					t.Errorf("recheck decision line mismatch (-want +got):\n%s", diff)
				}
			}
			if diff := cmp.Diff(test.WantNote, res.Note); diff != "" {
				t.Errorf("note mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.WantRounds, res.Rounds); diff != "" {
				t.Errorf("rounds mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.WantCalls, model.callCount()); diff != "" {
				t.Errorf("calls mismatch (-want +got):\n%s", diff)
			}
			for _, want := range test.WantPrompts {
				if !model.prompted(want) {
					t.Errorf("no %s prompt for %q contains %q", want.Step, want.World, want.Text)
				}
			}
			for _, absent := range test.WantAbsent {
				if model.prompted(absent) {
					t.Errorf("a %s prompt for %q contains %q",
						absent.Step, absent.World, absent.Text)
				}
			}
		})
	}
}

// TestRunKeepRule checks each condition of the keep rule on its own: a verdict that fails
// exactly one of them is dropped, and the fit floor sits exactly at MinFit.
func TestRunKeepRule(t *testing.T) {
	t.Parallel()
	pass := testVerdict(5)
	noShape, strange, noDecision := pass, pass, pass
	noShape.SameShape = false
	strange.FamiliarWorld = false
	noDecision.PointsAtDecision = false
	tests := []struct {
		Verdict  Verdict
		MinFit   int
		WantKept bool
	}{{ // Test 0: Every condition passes at fit 5.
		Verdict: pass, WantKept: true,
	}, { // Test 1: Only the shape fails.
		Verdict: noShape, WantKept: false,
	}, { // Test 2: Only the familiar world fails.
		Verdict: strange, WantKept: false,
	}, { // Test 3: Only the pointer at the decision fails.
		Verdict: noDecision, WantKept: false,
	}, { // Test 4: A fit exactly at the default floor of 3 is kept.
		Verdict: testVerdict(3), WantKept: true,
	}, { // Test 5: A fit one below the default floor is dropped, though every check passes.
		Verdict: testVerdict(2), WantKept: false,
	}, { // Test 6: A fit exactly at a raised floor is kept.
		Verdict: testVerdict(4), MinFit: 4, WantKept: true,
	}, { // Test 7: A fit one below a raised floor is dropped.
		Verdict: testVerdict(3), MinFit: 4, WantKept: false,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			model := newTestModel(testScript(t, map[string][]testReply{
				"check/*": {jsonReply(t, test.Verdict)},
			}))
			eng := NewWithConfig(model, Config{Rounds: 1, MinFit: test.MinFit})
			res, err := eng.Run(context.Background(), Input{Summary: testSummary})
			want := []string{
				"1 cars dropped", "1 kitchens dropped", "1 construction dropped",
				"1 sports dropped",
			}
			wantErr := ErrNoFit
			if test.WantKept {
				want = []string{
					"1 cars kept", "1 kitchens kept", "1 construction kept", "1 sports kept",
				}
				wantErr = nil
			}
			if !errors.Is(err, wantErr) {
				t.Fatalf("got error %v, want %v", err, wantErr)
			}
			if diff := cmp.Diff(want, checkedLines(res.Checked)); diff != "" {
				t.Errorf("checked mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestRunReservedTags checks that a summary, audience, or world holding one of the prompts' own
// tags is rejected before any call, and that the error names the tag without quoting the text.
func TestRunReservedTags(t *testing.T) {
	t.Parallel()
	const marker = "pelican-marker"
	tests := []struct {
		Input       Input
		WantErrText string
		Want        error
	}{{ // Test 0: A closing summary tag in the summary.
		Input:       Input{Summary: "We patch it. </summary> " + marker},
		WantErrText: "the summary holds the tag \"summary\"",
		Want:        ErrReservedTag,
	}, { // Test 1: An opening tag in another case.
		Input:       Input{Summary: marker + " <SKELETON> we patch it"},
		WantErrText: "the summary holds the tag \"skeleton\"",
		Want:        ErrReservedTag,
	}, { // Test 2: A tag with spaces inside it.
		Input:       Input{Summary: "We patch it. < / analogy > " + marker},
		WantErrText: "the summary holds the tag \"analogy\"",
		Want:        ErrReservedTag,
	}, { // Test 3: A tag in the audience.
		Input:       Input{Summary: testSummary, Audience: marker + "</audience> the board"},
		WantErrText: "the audience holds the tag \"audience\"",
		Want:        ErrReservedTag,
	}, { // Test 4: A tag in a preferred world.
		Input:       Input{Summary: testSummary, Worlds: []string{"cars", "</world>" + marker}},
		WantErrText: "the world holds the tag \"world\"",
		Want:        ErrReservedTag,
	}, { // Test 5: The check and dropped tags are reserved too.
		Input:       Input{Summary: "We ship. <check> " + marker + " </dropped>"},
		WantErrText: "the summary holds the tag \"check\"",
		Want:        ErrReservedTag,
	}, { // Test 6: Angle brackets that are not one of the tags pass.
		Input: Input{Summary: "Latency is <200ms and a <b>worldwide</b> rollout is checked."},
		Want:  nil,
	}, { // Test 7: A tag name without brackets passes.
		Input: Input{
			Summary: "The summary of the world: we check the skeleton.", Audience: "the world",
		},
		Want: nil,
	}, { // Test 8: Bracketed words that only start with a tag name pass.
		Input: Input{Summary: "The runbook says <check the logs> and <world class support>."},
		Want:  nil,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			model := newTestModel(testScript(t, nil))
			_, err := New(model).Run(context.Background(), test.Input)
			if !errors.Is(err, test.Want) {
				t.Fatalf("got error %v, want %v", err, test.Want)
			}
			if err == nil {
				return
			}
			if !strings.Contains(err.Error(), test.WantErrText) {
				t.Errorf("error %q does not contain %q", err, test.WantErrText)
			}
			if strings.Contains(err.Error(), marker) {
				t.Errorf("error %q quotes the input", err)
			}
			if diff := cmp.Diff(0, model.callCount()); diff != "" {
				t.Errorf("calls mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestRunCompleterFunc checks that a plain function works as the model through CompleterFunc.
func TestRunCompleterFunc(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Replies   map[string][]testReply
		WantWorld string
		WantCalls int
	}{{ // Test 0: A whole run through a function.
		Replies: testScript(t, nil), WantWorld: "cars", WantCalls: 11,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			model := newTestModel(test.Replies)
			res, err := New(CompleterFunc(model.Complete)).Run(
				context.Background(), Input{Summary: testSummary})
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if diff := cmp.Diff(test.WantWorld, res.Analogy.World); diff != "" {
				t.Errorf("world mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.WantCalls, model.callCount()); diff != "" {
				t.Errorf("calls mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestRunEvents checks the progress events Run reports for each step and item, with the fit on
// every kept and dropped event and nowhere else.
func TestRunEvents(t *testing.T) {
	t.Parallel()
	long := testAnalogy()
	long.Spoken += " We should book it this week."
	noBreak := testAnalogy()
	noBreak.BreaksDown = ""
	bad := testReply{Text: "{not json"}
	const keptAsIs = "kept as is: spoken has 4 sentences, and it needs two or three"
	tests := []struct {
		Replies     map[string][]testReply
		WantCounts  map[string]int
		WantDropped []string
		WantDetail  string
		WantNote    string
		Want        error
	}{{ // Test 0: Every item succeeds.
		Replies: testScript(t, nil),
		WantCounts: map[string]int{
			"skeleton started": 1, "skeleton done": 1,
			"draft started": 4, "draft done": 4,
			"check started": 4, "check kept": 4,
			"final started": 1, "final done": 1,
			"recheck started": 1, "recheck kept": 1,
		},
	}, { // Test 1: A failed draft, a dropped check, and a long final that is kept.
		Replies: testScript(t, map[string][]testReply{
			"draft/kitchens": {bad},
			"check/cars":     {jsonReply(t, testDropped())},
			"final":          {jsonReply(t, long)},
		}),
		WantCounts: map[string]int{
			"skeleton started": 1, "skeleton done": 1,
			"draft started": 4, "draft done": 3, "draft failed": 1,
			"check started": 3, "check kept": 2, "check dropped": 1,
			"final started": 1, "final done": 1,
			"recheck started": 1, "recheck kept": 1,
		},
		WantDropped: []string{"the chain runs backward"},
		WantDetail:  keptAsIs,
		WantNote:    keptAsIs,
	}, { // Test 2: A failed skeleton.
		Replies:    testScript(t, map[string][]testReply{"skeleton": {bad}}),
		WantCounts: map[string]int{"skeleton started": 1, "skeleton failed": 1},
		Want:       ErrBadReply,
	}, { // Test 3: A failed final reports the failure and skips the re-check.
		Replies: testScript(t, map[string][]testReply{"final": {jsonReply(t, noBreak)}}),
		WantCounts: map[string]int{
			"skeleton started": 1, "skeleton done": 1,
			"draft started": 4, "draft done": 4,
			"check started": 4, "check kept": 4,
			"final started": 1, "final failed": 1,
		},
		WantNote: "polish failed, this is the checked draft: " +
			"incomplete analogy: final step: breaks_down is empty",
	}, { // Test 4: A polished text that fails the re-check is reported as dropped.
		Replies: testScript(t, map[string][]testReply{
			"recheck": {jsonReply(t, testDropped())},
		}),
		WantCounts: map[string]int{
			"skeleton started": 1, "skeleton done": 1,
			"draft started": 4, "draft done": 4,
			"check started": 4, "check kept": 4,
			"final started": 1, "final done": 1,
			"recheck started": 1, "recheck dropped": 1,
		},
		WantDropped: []string{"the chain runs backward"},
		WantNote: "the polished analogy did not pass its check, this is the checked draft: " +
			"the chain runs backward",
	}, { // Test 5: A re-check call that fails is reported as failed.
		Replies: testScript(t, map[string][]testReply{"recheck": {{Err: errTestModel}}}),
		WantCounts: map[string]int{
			"skeleton started": 1, "skeleton done": 1,
			"draft started": 4, "draft done": 4,
			"check started": 4, "check kept": 4,
			"final started": 1, "final done": 1,
			"recheck started": 1, "recheck failed": 1,
		},
		WantNote: "the check of the polished analogy failed, this is the checked draft: " +
			"model call: recheck step: test model failure",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			events := &testEvents{}
			eng := NewWithConfig(newTestModel(test.Replies), Config{Progress: events.add})
			res, err := eng.Run(context.Background(), Input{Summary: testSummary})
			if !errors.Is(err, test.Want) {
				t.Fatalf("got error %v, want %v", err, test.Want)
			}
			counts := map[string]int{}
			var droppedDetails []string
			detail := ""
			for _, ev := range events.list {
				counts[string(ev.Step)+" "+string(ev.Status)]++
				item := ev.Step == StepDraft || ev.Step == StepCheck || ev.Step == StepRecheck
				if item && (ev.Round < 1 || ev.World == "") {
					t.Errorf("%s event lacks a round or world: %+v", ev.Step, ev)
				}
				if !item && ev.Round != 0 {
					t.Errorf("%s event has round %d, want 0", ev.Step, ev.Round)
				}
				wantFit := 0
				switch ev.Status {
				case StatusKept:
					wantFit = 4
				case StatusDropped:
					wantFit = 2
					droppedDetails = append(droppedDetails, ev.Detail)
				}
				if diff := cmp.Diff(wantFit, ev.Fit); diff != "" {
					t.Errorf("%s %s event fit mismatch (-want +got):\n%s", ev.Step, ev.Status, diff)
				}
				if ev.Step == StepFinal && ev.Status == StatusDone {
					detail = ev.Detail
				}
			}
			if diff := cmp.Diff(test.WantCounts, counts); diff != "" {
				t.Errorf("counts mismatch (-want +got):\n%s", diff)
			}
			empty := cmpopts.EquateEmpty()
			if diff := cmp.Diff(test.WantDropped, droppedDetails, empty); diff != "" {
				t.Errorf("dropped details mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.WantDetail, detail); diff != "" {
				t.Errorf("final detail mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.WantNote, res.Note); diff != "" {
				t.Errorf("note mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestRunRequests checks that every request carries the system prompt, the schema of its
// step's reply type, the summary inside its tags, and the audience inside its tags, and that
// every step after the first carries the skeleton the first step pulled.
func TestRunRequests(t *testing.T) {
	t.Parallel()
	system, err := render(systemTemplate, promptData{})
	if err != nil {
		t.Fatalf("render system prompt: %v", err)
	}
	schemas := map[Step]string{
		StepSkeleton: string(schema.For(Skeleton{})),
		StepDraft:    string(schema.For(Candidate{})),
		StepCheck:    string(schema.For(Verdict{})),
		StepFinal:    string(schema.For(Analogy{})),
		StepRecheck:  string(schema.For(Verdict{})),
	}
	skeletonLines := []string{
		"<skeleton>\nPlayers:\n- the team: patches the pipeline by hand\n",
		"Decision: fund a rebuild of the pipeline or keep patching\n",
		"Causal chain:\n1. the pipeline breaks\n2. the team patches it\n3. the patches pile up\n",
	}
	wantSteps := map[Step]int{
		StepSkeleton: 1, StepDraft: 4, StepCheck: 4, StepFinal: 1, StepRecheck: 1,
	}
	tests := []struct {
		Input        Input
		WantAudience string
		WantSteps    map[Step]int
	}{{ // Test 0: The audience given is used.
		Input:        Input{Summary: testSummary, Audience: "the board"},
		WantAudience: "the board",
		WantSteps:    wantSteps,
	}, { // Test 1: A blank audience falls back to the default, and the summary is trimmed.
		Input:        Input{Summary: "\n  " + testSummary + "  \n", Audience: "  "},
		WantAudience: DefaultAudience,
		WantSteps:    wantSteps,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			model := newTestModel(testScript(t, nil))
			if _, err := New(model).Run(context.Background(), test.Input); err != nil {
				t.Fatalf("run: %v", err)
			}
			wantSummary := "<summary>\n" + testSummary + "\n</summary>"
			wantAudience := "<audience>\n" + test.WantAudience + "\n</audience>"
			steps := map[Step]int{}
			for _, req := range model.requests {
				steps[req.Step]++
				if req.System == "" {
					t.Errorf("%s request has no system prompt", req.Step)
				}
				if diff := cmp.Diff(system, req.System); diff != "" {
					t.Errorf("%s system mismatch (-want +got):\n%s", req.Step, diff)
				}
				if diff := cmp.Diff(schemas[req.Step], string(req.Schema)); diff != "" {
					t.Errorf("%s schema mismatch (-want +got):\n%s", req.Step, diff)
				}
				for _, want := range []string{wantSummary, wantAudience} {
					if !strings.Contains(req.Prompt, want) {
						t.Errorf("%s prompt lacks %q", req.Step, want)
					}
				}
				if req.Step == StepSkeleton {
					if strings.Contains(req.Prompt, "<skeleton>") {
						t.Errorf("skeleton prompt holds a skeleton block:\n%s", req.Prompt)
					}
					continue
				}
				for _, want := range skeletonLines {
					if !strings.Contains(req.Prompt, want) {
						t.Errorf("%s prompt lacks the skeleton line %q:\n%s",
							req.Step, want, req.Prompt)
					}
				}
			}
			if diff := cmp.Diff(test.WantSteps, steps); diff != "" {
				t.Errorf("steps mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// testWorldDraft returns a usable draft whose spoken line names world, so a prompt that holds
// it can be told apart from a prompt that holds another world's draft.
func testWorldDraft(world string) Candidate {
	c := testCandidate()
	c.Spoken = "The " + world + " draft says this and nothing else. It stops here."
	return c
}

// TestRunCheckIsolation checks that each check sees exactly one candidate: its prompt holds one
// analogy block and one world tag, the spoken line of its own draft, and no other draft's
// line. The re-check sees only the polished text.
func TestRunCheckIsolation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Worlds []string
	}{{ // Test 0: The default worlds.
		Worlds: []string{"cars", "kitchens", "construction", "sports"},
	}, { // Test 1: Two worlds.
		Worlds: []string{"boats", "trains"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			script := map[string][]testReply{}
			for _, w := range test.Worlds {
				script["draft/"+w] = []testReply{jsonReply(t, testWorldDraft(w))}
			}
			model := newTestModel(testScript(t, script))
			eng := NewWithConfig(model, Config{Worlds: test.Worlds})
			if _, err := eng.Run(context.Background(), Input{Summary: testSummary}); err != nil {
				t.Fatalf("run: %v", err)
			}
			checks := 0
			for _, req := range model.requests {
				if req.Step != StepCheck && req.Step != StepRecheck {
					continue
				}
				if diff := cmp.Diff(1, strings.Count(req.Prompt, "<analogy>")); diff != "" {
					t.Errorf("%s analogy block count mismatch (-want +got):\n%s", req.Step, diff)
				}
				if diff := cmp.Diff(1, strings.Count(req.Prompt, "<world>")); diff != "" {
					t.Errorf("%s world tag count mismatch (-want +got):\n%s", req.Step, diff)
				}
				own := promptWorld(req.Prompt)
				for _, w := range test.Worlds {
					line := testWorldDraft(w).Spoken
					has := strings.Contains(req.Prompt, line)
					want := req.Step == StepCheck && w == own
					if has != want {
						t.Errorf("%s prompt for %s holds the %s draft: %t, want %t",
							req.Step, own, w, has, want)
					}
				}
				if req.Step == StepCheck {
					checks++
					continue
				}
				if !strings.Contains(req.Prompt, "Spoken: "+testAnalogy().Spoken+"\n") {
					t.Errorf("recheck prompt lacks the polished text:\n%s", req.Prompt)
				}
			}
			if diff := cmp.Diff(len(test.Worlds), checks); diff != "" {
				t.Errorf("check count mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestRunCanceled checks that Run stops and returns the context's error when it is canceled,
// at every step. A cancel during the final step or the re-check is still a cancel, never a
// fallback to the draft.
func TestRunCanceled(t *testing.T) {
	t.Parallel()
	stop := []testReply{{Cancel: true}}
	tests := []struct {
		Replies   map[string][]testReply
		Before    bool
		WantCalls int
	}{{ // Test 0: Canceled before Run starts.
		Replies: testScript(t, nil), Before: true, WantCalls: 0,
	}, { // Test 1: Canceled during the skeleton step.
		Replies: testScript(t, map[string][]testReply{"skeleton": stop}), WantCalls: 1,
	}, { // Test 2: Canceled during the first draft.
		Replies: testScript(t, map[string][]testReply{"draft/cars": stop}), WantCalls: 2,
	}, { // Test 3: Canceled during the second check.
		Replies: testScript(t, map[string][]testReply{"check/kitchens": stop}), WantCalls: 7,
	}, { // Test 4: Canceled during the final step.
		Replies: testScript(t, map[string][]testReply{"final": stop}), WantCalls: 10,
	}, { // Test 5: Canceled during the re-check.
		Replies: testScript(t, map[string][]testReply{"recheck": stop}), WantCalls: 11,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			model := newTestModel(test.Replies)
			model.cancel = cancel
			if test.Before {
				cancel()
			}
			eng := NewWithConfig(model, Config{Workers: 1})
			res, err := eng.Run(ctx, Input{Summary: testSummary})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("got error %v, want %v", err, context.Canceled)
			}
			if diff := cmp.Diff(Analogy{}, res.Analogy, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("analogy mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff("", res.Note); diff != "" {
				t.Errorf("note mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.WantCalls, model.callCount()); diff != "" {
				t.Errorf("calls mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestRunWorkers checks that Run never has more model calls in flight than Workers allows.
func TestRunWorkers(t *testing.T) {
	t.Parallel()
	worlds := []string{"cars", "kitchens", "construction", "sports", "boats", "trains"}
	tests := []struct {
		Workers int
		WantMax int
	}{{ // Test 0: One call at a time.
		Workers: 1, WantMax: 1,
	}, { // Test 1: Two calls at a time.
		Workers: 2, WantMax: 2,
	}, { // Test 2: Three calls at a time.
		Workers: 3, WantMax: 3,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			model := newTestModel(testScript(t, nil))
			eng := NewWithConfig(model, Config{Worlds: worlds, Workers: test.Workers})
			if _, err := eng.Run(context.Background(), Input{Summary: testSummary}); err != nil {
				t.Fatalf("run: %v", err)
			}
			if model.peak < 1 || model.peak > test.WantMax {
				t.Errorf("peak of %d calls at once, want 1 to %d", model.peak, test.WantMax)
			}
		})
	}
}
