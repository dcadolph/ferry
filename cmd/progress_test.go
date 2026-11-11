package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/ferry/internal/engine"
	"github.com/dcadolph/ferry/internal/logutil"
)

// TestProgressLine checks the line printed for each kind of engine event.
func TestProgressLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		WantResult string
		In         engine.Event
	}{{ // Test 0: The skeleton step starting.
		In:         engine.Event{Step: engine.StepSkeleton, Status: engine.StatusStarted},
		WantResult: "Reading the situation",
	}, { // Test 1: The skeleton step finishing needs no line.
		In:         engine.Event{Step: engine.StepSkeleton, Status: engine.StatusDone},
		WantResult: "",
	}, { // Test 2: The skeleton step failing.
		In: engine.Event{
			Step: engine.StepSkeleton, Status: engine.StatusFailed, Detail: "bad model reply",
		},
		WantResult: "Reading the situation failed, bad model reply",
	}, { // Test 3: A first-round draft.
		In: engine.Event{
			Step: engine.StepDraft, Status: engine.StatusStarted, World: "cars", Round: 1,
		},
		WantResult: "Drafting from cars",
	}, { // Test 4: A later-round draft names the round.
		In: engine.Event{
			Step: engine.StepDraft, Status: engine.StatusStarted, World: "home plumbing", Round: 2,
		},
		WantResult: "Drafting from home plumbing (round 2)",
	}, { // Test 5: A finished draft needs no line.
		In: engine.Event{
			Step: engine.StepDraft, Status: engine.StatusDone, World: "cars", Round: 1,
		},
		WantResult: "",
	}, { // Test 6: A failed draft.
		In: engine.Event{
			Step: engine.StepDraft, Status: engine.StatusFailed, World: "cars", Detail: "timeout",
		},
		WantResult: "cars: draft failed, timeout",
	}, { // Test 7: A check starting.
		In: engine.Event{
			Step: engine.StepCheck, Status: engine.StatusStarted, World: "cars", Round: 1,
		},
		WantResult: "Checking the cars draft",
	}, { // Test 8: A kept check shows its fit and not the checker's first complaint.
		In: engine.Event{
			Step: engine.StepCheck, Status: engine.StatusKept, World: "cars", Fit: 4,
			Detail: "the hole never grows, but the real gap does",
		},
		WantResult: "cars: kept, fit 4",
	}, { // Test 9: A kept check with no fit.
		In:         engine.Event{Step: engine.StepCheck, Status: engine.StatusKept, World: "cars"},
		WantResult: "cars: kept",
	}, { // Test 10: A dropped check gives its fit and its reason.
		In: engine.Event{
			Step: engine.StepCheck, Status: engine.StatusDropped, World: "kitchens", Fit: 2,
			Detail: "it does not point at the decision",
		},
		WantResult: "kitchens: dropped, fit 2, it does not point at the decision",
	}, { // Test 11: A dropped check with no fit and no reason.
		In: engine.Event{
			Step: engine.StepCheck, Status: engine.StatusDropped, World: "kitchens",
		},
		WantResult: "kitchens: dropped",
	}, { // Test 12: A failed check.
		In: engine.Event{
			Step: engine.StepCheck, Status: engine.StatusFailed, World: "sports",
			Detail: "bad model reply",
		},
		WantResult: "sports: check failed, bad model reply",
	}, { // Test 13: Polishing the winner.
		In: engine.Event{
			Step: engine.StepFinal, Status: engine.StatusStarted, World: "cars",
		},
		WantResult: "Polishing the cars analogy",
	}, { // Test 14: Polishing with no world named.
		In:         engine.Event{Step: engine.StepFinal, Status: engine.StatusStarted},
		WantResult: "Polishing the analogy",
	}, { // Test 15: A clean finish needs no line.
		In:         engine.Event{Step: engine.StepFinal, Status: engine.StatusDone, World: "cars"},
		WantResult: "",
	}, { // Test 16: A finish with a note needs no line, since the note prints with the result.
		In: engine.Event{
			Step: engine.StepFinal, Status: engine.StatusDone, World: "cars",
			Detail: "kept as is: spoken has 4 sentences, and it needs two or three",
		},
		WantResult: "",
	}, { // Test 17: A failed finish.
		In: engine.Event{
			Step: engine.StepFinal, Status: engine.StatusFailed, Detail: "incomplete analogy",
		},
		WantResult: "Polishing failed, incomplete analogy",
	}, { // Test 18: Line breaks and runs of spaces in a detail collapse.
		In: engine.Event{
			Step: engine.StepCheck, Status: engine.StatusDropped, World: "kitchens",
			Detail: "too\nlong   here\n",
		},
		WantResult: "kitchens: dropped, too long here",
	}, { // Test 19: An unknown step needs no line.
		In:         engine.Event{Step: engine.Step("other"), Status: engine.StatusStarted},
		WantResult: "",
	}, { // Test 20: The re-check starting.
		In: engine.Event{
			Step: engine.StepRecheck, Status: engine.StatusStarted, World: "cars", Round: 1,
		},
		WantResult: "Checking the polished analogy",
	}, { // Test 21: A polished analogy that passed its check.
		In: engine.Event{
			Step: engine.StepRecheck, Status: engine.StatusKept, World: "cars", Fit: 5,
		},
		WantResult: "Polished analogy: kept, fit 5",
	}, { // Test 22: A polished analogy that failed its check.
		In: engine.Event{
			Step: engine.StepRecheck, Status: engine.StatusDropped, World: "cars", Fit: 2,
			Detail: "the polish dropped the growing cost",
		},
		WantResult: "Polished analogy: dropped, fit 2, the polish dropped the growing cost",
	}, { // Test 23: A re-check that failed.
		In: engine.Event{
			Step: engine.StepRecheck, Status: engine.StatusFailed, World: "cars",
			Detail: "claude rate limited",
		},
		WantResult: "Polished analogy: check failed, claude rate limited",
	}, { // Test 24: A dropped check with a fit and no reason.
		In: engine.Event{
			Step: engine.StepCheck, Status: engine.StatusDropped, World: "kitchens", Fit: 1,
		},
		WantResult: "kitchens: dropped, fit 1",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(test.WantResult, progressLine(test.In)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestProgressPrinter checks that events printed from many goroutines at once each land as one
// whole line, and that events with no line print nothing.
func TestProgressPrinter(t *testing.T) {
	t.Parallel()
	const goroutines = 64
	var buf bytes.Buffer
	p := newProgressPrinter(&buf)
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
	want := strings.Repeat("cars: kept\n", goroutines)
	if diff := cmp.Diff(want, buf.String()); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
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
		In: failedDraft, Terminal: true, WantPrint: "cars: draft failed, claude rate limited\n",
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
	}, { // Test 7: A started draft at a terminal is printed.
		In: engine.Event{
			Step: engine.StepDraft, Status: engine.StatusStarted, World: "cars", Round: 1,
		},
		Terminal: true, WantPrint: "Drafting from cars\n",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			var printed, logged bytes.Buffer
			logger, err := logutil.New(&logged, "debug")
			if err != nil {
				t.Fatalf("logutil.New: %v", err)
			}
			progressFunc(&printed, test.Terminal, logger)(test.In)
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
			progressFunc(&bytes.Buffer{}, test.Terminal, nil)
		})
	}
}

// TestNewProgressPrinterPanics checks that a nil writer is a developer error.
func TestNewProgressPrinterPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Error("newProgressPrinter(nil) did not panic")
		}
	}()
	newProgressPrinter(nil)
}
