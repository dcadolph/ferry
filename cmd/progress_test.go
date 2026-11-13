package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/ferry/internal/engine"
	"github.com/dcadolph/ferry/internal/logutil"
	"github.com/dcadolph/ferry/internal/render"
)

// testClock is a clock tests move by hand.
type testClock struct {
	// mu guards now.
	mu sync.Mutex
	// now is the current time.
	now time.Time
}

// newTestClock returns a testClock set to a fixed time.
func newTestClock() *testClock {
	return &testClock{now: time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)}
}

// Now returns the clock's time.
func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Add moves the clock forward by d.
func (c *testClock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// testDisplay returns a terminal display with no color and no live line, so output is exact.
func testDisplay(width int) display {
	return display{terminal: true, style: render.NewStyle(false), width: width}
}

// testEvent is one event fed to a printer, after the clock moves by Wait.
type testEvent struct {
	// Ev is the event.
	Ev engine.Event
	// Wait is how far the clock moves before the event.
	Wait time.Duration
}

// testFullRun returns the events of a run that drops both first-round drafts, finds a winner
// in round two, and passes the polish.
func testFullRun() []testEvent {
	long := "The failure comes from the wrong place. In the analogy the engine seizes " +
		"because oil ran low."
	return []testEvent{
		{Ev: engine.Event{Step: engine.StepSkeleton, Status: engine.StatusStarted}},
		{Wait: 9 * time.Second, Ev: engine.Event{Step: engine.StepSkeleton, Status: engine.StatusDone}},
		{Ev: engine.Event{Step: engine.StepDraft, Status: engine.StatusStarted, World: "cars", Round: 1}},
		{Ev: engine.Event{Step: engine.StepDraft, Status: engine.StatusStarted, World: "sports", Round: 1}},
		{Wait: time.Second, Ev: engine.Event{
			Step: engine.StepDraft, Status: engine.StatusDone, World: "cars", Round: 1,
		}},
		{Ev: engine.Event{Step: engine.StepCheck, Status: engine.StatusStarted, World: "cars", Round: 1}},
		{Ev: engine.Event{
			Step: engine.StepDraft, Status: engine.StatusFailed, World: "sports", Round: 1,
			Detail: "claude rate limited",
		}},
		{Ev: engine.Event{
			Step: engine.StepCheck, Status: engine.StatusDropped, World: "cars", Round: 1, Fit: 2,
			Detail: long,
		}},
		{Ev: engine.Event{
			Step: engine.StepDraft, Status: engine.StatusStarted, World: "home plumbing", Round: 2,
		}},
		{Ev: engine.Event{
			Step: engine.StepDraft, Status: engine.StatusDone, World: "home plumbing", Round: 2,
		}},
		{Ev: engine.Event{
			Step: engine.StepCheck, Status: engine.StatusStarted, World: "home plumbing", Round: 2,
		}},
		{Ev: engine.Event{
			Step: engine.StepCheck, Status: engine.StatusKept, World: "home plumbing", Round: 2, Fit: 4,
			Detail: "the pipe never grows",
		}},
		{Ev: engine.Event{Step: engine.StepFinal, Status: engine.StatusStarted, World: "home plumbing"}},
		{Ev: engine.Event{Step: engine.StepFinal, Status: engine.StatusDone, World: "home plumbing"}},
		{Ev: engine.Event{Step: engine.StepRecheck, Status: engine.StatusStarted, World: "home plumbing"}},
		{Wait: 82 * time.Second, Ev: engine.Event{
			Step: engine.StepRecheck, Status: engine.StatusKept, World: "home plumbing", Fit: 5,
		}},
	}
}

// testFullRunText is what a full run prints at 80 columns with no color, before the closing line.
const testFullRunText = `
Read the situation  9s

Round 1  drafting from several worlds
  ✗ sports            draft failed  claude rate limited
  ✗ cars              dropped  2/5  The failure comes from the wrong place.

Round 2  nothing held up, so trying new worlds
  ✓ home plumbing     kept     4/5

Polishing the home plumbing analogy
  ✓ polished          kept     5/5
`

// TestProgressPrinter checks the lines a run prints, and the closing line for each way a run
// can end.
func TestProgressPrinter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Err        error
		WantResult string
		Width      int
	}{{ // Test 0: A clean run.
		Width: 80, WantResult: testFullRunText + "\nDone in 1m 32s · 8 calls\n\n",
	}, { // Test 1: No analogy held up.
		Width: 80, Err: fmt.Errorf("x: %w", engine.ErrNoFit),
		WantResult: testFullRunText + "\nNothing held up in 1m 32s · 8 calls\n\n",
	}, { // Test 2: Ctrl-C gets no closing line.
		Width: 80, Err: &QuietExitError{Code: codeInterrupted}, WantResult: testFullRunText,
	}, { // Test 3: Any other failure says it stopped.
		Width: 80, Err: errors.New("network down"),
		WantResult: testFullRunText + "\nStopped in 1m 32s · 8 calls\n",
	}, { // Test 4: A narrow terminal cuts the reason with an ellipsis.
		Width: 60, WantResult: strings.Replace(testFullRunText,
			"The failure comes from the wrong place.", "The failure comes from…", 1) +
			"\nDone in 1m 32s · 8 calls\n\n",
	}, { // Test 5: A very narrow terminal leaves the reasons off.
		Width: 40, WantResult: strings.NewReplacer(
			"  The failure comes from the wrong place.", "",
			"  claude rate limited", "",
		).Replace(testFullRunText) + "\nDone in 1m 32s · 8 calls\n\n",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			clock := newTestClock()
			p := newProgressPrinter(&buf, testDisplay(test.Width), clock.Now)
			for _, e := range testFullRun() {
				clock.Add(e.Wait)
				p.print(e.Ev)
			}
			p.finish(test.Err)
			p.finish(test.Err)
			p.print(engine.Event{Step: engine.StepSkeleton, Status: engine.StatusDone})
			if diff := cmp.Diff(test.WantResult, buf.String()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestProgressPrinterLines checks the line for each kind of finished event on its own.
func TestProgressPrinterLines(t *testing.T) {
	t.Parallel()
	tests := []struct {
		WantResult string
		In         engine.Event
	}{{ // Test 0: A failed skeleton gets its heading and the reason.
		In:         engine.Event{Step: engine.StepSkeleton, Status: engine.StatusFailed, Detail: "bad reply"},
		WantResult: "\nReading the situation\n  ✗ failed  bad reply\n",
	}, { // Test 1: A failed check.
		In: engine.Event{
			Step: engine.StepCheck, Status: engine.StatusFailed, World: "sports", Detail: "bad reply",
		},
		WantResult: "  ! sports            check failed  bad reply\n",
	}, { // Test 2: A failed polish.
		In:         engine.Event{Step: engine.StepFinal, Status: engine.StatusFailed, Detail: "incomplete"},
		WantResult: "  ✗ polish failed  incomplete\n",
	}, { // Test 3: A dropped polish shows its reason.
		In: engine.Event{
			Step: engine.StepRecheck, Status: engine.StatusDropped, Fit: 2,
			Detail: "the polish dropped the growing cost",
		},
		WantResult: "  ✗ polished          dropped  2/5  the polish dropped the growing cost\n",
	}, { // Test 4: A failed re-check.
		In:         engine.Event{Step: engine.StepRecheck, Status: engine.StatusFailed, Detail: "rate limited"},
		WantResult: "  ! polished          check failed  rate limited\n",
	}, { // Test 5: A kept check with no fit.
		In:         engine.Event{Step: engine.StepCheck, Status: engine.StatusKept, World: "cars"},
		WantResult: "  ✓ cars              kept\n",
	}, { // Test 6: A world name longer than the column still gets a space after it.
		In: engine.Event{
			Step: engine.StepCheck, Status: engine.StatusKept, World: "a very long world name", Fit: 3,
		},
		WantResult: "  ✓ a very long world name kept     3/5\n",
	}, { // Test 7: A polish with no world named.
		In:         engine.Event{Step: engine.StepFinal, Status: engine.StatusStarted},
		WantResult: "\nPolishing the analogy\n",
	}, { // Test 8: A finished draft needs no line.
		In:         engine.Event{Step: engine.StepDraft, Status: engine.StatusDone, World: "cars"},
		WantResult: "",
	}, { // Test 9: An unknown step needs no line.
		In:         engine.Event{Step: engine.Step("other"), Status: engine.StatusDone},
		WantResult: "",
	}, { // Test 10: A started check needs no line, since the live line covers it.
		In:         engine.Event{Step: engine.StepCheck, Status: engine.StatusStarted, World: "cars"},
		WantResult: "",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			p := newProgressPrinter(&buf, testDisplay(80), newTestClock().Now)
			p.print(test.In)
			if diff := cmp.Diff(test.WantResult, buf.String()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestProgressPrinterColor checks that palette colors, pills, meters, and emoji appear when the
// style is on.
func TestProgressPrinterColor(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	d := testDisplay(80)
	d.style = render.NewStyle(true)
	clock := newTestClock()
	p := newProgressPrinter(&buf, d, clock.Now)
	for _, e := range testFullRun() {
		clock.Add(e.Wait)
		p.print(e.Ev)
	}
	p.finish(nil)
	got := buf.String()
	for _, want := range []string{
		"🔍 ", "\x1b[38;2;34;197;94m✓\x1b[m", "\x1b[38;2;248;113;113m✗\x1b[m", "KEPT", "DROPPED",
		"\x1b[1;38;2;34;211;238mRound 1\x1b[m", "●●●●",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
}

// TestProgressPrinterLive checks the live line: it names the calls in flight with their
// elapsed time, redraws in place, and is erased before fixed lines and at the end.
func TestProgressPrinterLive(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	d := testDisplay(80)
	d.live = true
	clock := newTestClock()
	p := newProgressPrinter(&buf, d, clock.Now)
	p.print(engine.Event{Step: engine.StepDraft, Status: engine.StatusStarted, World: "sports", Round: 1})
	p.print(engine.Event{Step: engine.StepDraft, Status: engine.StatusStarted, World: "cars", Round: 1})
	clock.Add(12 * time.Second)
	p.mu.Lock()
	p.redraw()
	p.mu.Unlock()
	p.finish(nil)
	got := buf.String()
	for _, want := range []string{
		clearLine + "  ⠋ drafting cars, sports 0s",
		clearLine + "  ⠋ drafting cars, sports 12s",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%q", want, got)
		}
	}
	if !strings.HasSuffix(got, clearLine+"\nDone in 12s · 2 calls\n\n") {
		t.Errorf("live line not erased before the closing line:\n%q", got)
	}
}

// TestLiveLabel checks the words for the calls in flight.
func TestLiveLabel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		WantResult string
		In         []task
	}{{ // Test 0: Nothing in flight.
		WantResult: "",
	}, { // Test 1: Reading.
		In: []task{{step: engine.StepSkeleton}}, WantResult: "reading the situation",
	}, { // Test 2: Drafts and checks at once, in step order with worlds sorted.
		In: []task{
			{step: engine.StepCheck, world: "sports"}, {step: engine.StepDraft, world: "kitchens"},
			{step: engine.StepDraft, world: "cars"},
		},
		WantResult: "drafting cars, kitchens · checking sports",
	}, { // Test 3: Polishing.
		In: []task{{step: engine.StepFinal, world: "cars"}}, WantResult: "polishing",
	}, { // Test 4: Checking the polish.
		In: []task{{step: engine.StepRecheck, world: "cars"}}, WantResult: "checking the polish",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			clock := newTestClock()
			p := newProgressPrinter(&bytes.Buffer{}, testDisplay(80), clock.Now)
			for _, k := range test.In {
				p.active[k] = clock.Now()
			}
			got, _ := p.liveLabel()
			if diff := cmp.Diff(test.WantResult, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestProgressPrinterConcurrent checks that events printed from many goroutines at once each
// land as one whole line.
func TestProgressPrinterConcurrent(t *testing.T) {
	t.Parallel()
	const goroutines = 64
	var buf bytes.Buffer
	p := newProgressPrinter(&buf, testDisplay(80), newTestClock().Now)
	kept := engine.Event{Step: engine.StepCheck, Status: engine.StatusKept, World: "cars"}
	quiet := engine.Event{Step: engine.StepDraft, Status: engine.StatusDone, World: "cars"}
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.print(kept)
			p.print(quiet)
		}()
	}
	wg.Wait()
	want := strings.Repeat("  ✓ cars              kept\n", goroutines)
	if diff := cmp.Diff(want, buf.String()); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

// TestProgressHelpers checks the small formatting helpers.
func TestProgressHelpers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Got  string
		Want string
	}{
		{Got: firstSentence("One. Two."), Want: "One."},              // Test 0: First of two.
		{Got: firstSentence("one\n  line"), Want: "one line"},        // Test 1: One sentence.
		{Got: truncate("abcdef", 4), Want: "abc…"},                   // Test 2: Cut.
		{Got: truncate("abc", 4), Want: "abc"},                       // Test 3: Fits.
		{Got: truncate("ab, cd", 4), Want: "ab…"},                    // Test 4: Trailing comma.
		{Got: truncate("abc", 0), Want: ""},                          // Test 5: No room.
		{Got: duration(9400 * time.Millisecond), Want: "9s"},         // Test 6: Seconds.
		{Got: duration(92 * time.Second), Want: "1m 32s"},            // Test 7: Minutes.
		{Got: duration(61 * time.Second), Want: "1m 01s"},            // Test 8: Padding.
		{Got: plural(1, "call"), Want: "1 call"},                     // Test 9: One.
		{Got: plural(2, "call"), Want: "2 calls"},                    // Test 10: Many.
		{Got: fmt.Sprint(streamWidth(&bytes.Buffer{})), Want: "80"},  // Test 11: Not a file.
		{Got: fmt.Sprint(canRedraw(&bytes.Buffer{})), Want: "false"}, // Test 12: Not a file.
		{Got: fmt.Sprint(isQuiet(&QuietExitError{})), Want: "true"},  // Test 13: Quiet.
		{Got: fmt.Sprint(isQuiet(errors.New("x"))), Want: "false"},   // Test 14: Not quiet.
	}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(test.Want, test.Got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestProgressFunc checks where each event goes. At a terminal every event that has a line is
// printed and nothing is logged. Anywhere else nothing is printed, and only a failed draft or
// a failed check is logged, at warn level, with its step, world, round, and detail on one line.
func TestProgressFunc(t *testing.T) {
	t.Parallel()
	failedDraft := engine.Event{
		Step: engine.StepDraft, Status: engine.StatusFailed, World: "cars", Round: 2,
		Detail: "claude rate\nlimited",
	}
	tests := []struct {
		In        engine.Event
		WantPrint string
		WantLog   []string
		Terminal  bool
	}{{ // Test 0: A failed draft at a terminal is printed, not logged.
		In: failedDraft, Terminal: true,
		WantPrint: "  ✗ cars              draft failed  claude rate limited\n",
	}, { // Test 1: A failed draft off the terminal is logged.
		In: failedDraft,
		WantLog: []string{
			"warn\tstep failed\t", `"step": "draft"`, `"world": "cars"`, `"round": 2`,
			`"detail": "claude rate limited"`,
		},
	}, { // Test 2: A failed check off the terminal is logged.
		In: engine.Event{
			Step: engine.StepCheck, Status: engine.StatusFailed, World: "sports", Round: 1,
			Detail: "bad model reply",
		},
		WantLog: []string{"warn\tstep failed\t", `"step": "check"`, `"world": "sports"`},
	}, { // Test 3: A kept check off the terminal is silent.
		In: engine.Event{
			Step: engine.StepCheck, Status: engine.StatusKept, World: "cars", Round: 1, Fit: 4,
		},
	}, { // Test 4: A failed skeleton off the terminal is silent, since its error is printed.
		In: engine.Event{Step: engine.StepSkeleton, Status: engine.StatusFailed, Detail: "x"},
	}, { // Test 5: A failed final step off the terminal is silent, since a note is printed.
		In: engine.Event{Step: engine.StepFinal, Status: engine.StatusFailed, Detail: "x"},
	}, { // Test 6: A failed re-check off the terminal is silent, since a note is printed.
		In: engine.Event{Step: engine.StepRecheck, Status: engine.StatusFailed, Detail: "x"},
	}, { // Test 7: A first draft at a terminal prints the round heading.
		In: engine.Event{
			Step: engine.StepDraft, Status: engine.StatusStarted, World: "cars", Round: 1,
		},
		Terminal: true, WantPrint: "\nRound 1  drafting from several worlds\n",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			var printed, logged bytes.Buffer
			logger, err := logutil.New(&logged, "debug")
			if err != nil {
				t.Fatalf("logutil.New: %v", err)
			}
			d := testDisplay(80)
			d.terminal = test.Terminal
			prog := progressFunc(&printed, d, logger)
			prog.report(test.In)
			if diff := cmp.Diff(test.WantPrint, printed.String()); diff != "" {
				t.Errorf("printed mismatch (-want +got):\n%s", diff)
			}
			got := logged.String()
			wantLines := 0
			if len(test.WantLog) > 0 {
				wantLines = 1
			}
			if diff := cmp.Diff(wantLines, strings.Count(got, "\n")); diff != "" {
				t.Errorf("log line count mismatch (-want +got):\n%s\n%s", diff, got)
			}
			for _, want := range test.WantLog {
				if !strings.Contains(got, want) {
					t.Errorf("log lacks %q:\n%s", want, got)
				}
			}
			if !test.Terminal {
				prog.finish(nil)
				if printed.Len() != 0 {
					t.Errorf("finish off the terminal printed %q", printed.String())
				}
			}
		})
	}
}

// TestProgressFuncPanics checks that a missing logger is a developer error.
func TestProgressFuncPanics(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Terminal bool
	}{{ // Test 0: Off the terminal.
		Terminal: false,
	}, { // Test 1: At a terminal.
		Terminal: true,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			defer func() {
				if recover() == nil {
					t.Error("progressFunc with no logger did not panic")
				}
			}()
			d := testDisplay(80)
			d.terminal = test.Terminal
			progressFunc(&bytes.Buffer{}, d, nil)
		})
	}
}

// TestNewProgressPrinterPanics checks that a nil writer or a nil clock is a developer error, and
// that a width below one falls back to 80 columns.
func TestNewProgressPrinterPanics(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Now  func() time.Time
		Name string
	}{{ // Test 0: No writer.
		Name: "writer", Now: time.Now,
	}, { // Test 1: No clock.
		Name: "clock",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			defer func() {
				if recover() == nil {
					t.Errorf("newProgressPrinter with no %s did not panic", test.Name)
				}
			}()
			if test.Name == "writer" {
				newProgressPrinter(nil, testDisplay(80), test.Now)
				return
			}
			newProgressPrinter(&bytes.Buffer{}, testDisplay(80), nil)
		})
	}
	if got := newProgressPrinter(&bytes.Buffer{}, testDisplay(0), time.Now).d.width; got != 80 {
		t.Errorf("width = %d, want 80", got)
	}
}
