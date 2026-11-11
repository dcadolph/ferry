package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/ferry/internal/claude"
	"github.com/dcadolph/ferry/internal/engine"
)

// TestExitCode checks the mapping from each kind of error to its exit code.
func TestExitCode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In       error
		WantCode int
	}{{ // Test 0: Success.
		In: nil, WantCode: 0,
	}, { // Test 1: An unclassified error is a general failure.
		In: errors.New("network down"), WantCode: 1,
	}, { // Test 2: A usage error.
		In: fmt.Errorf("%w: unknown flag: --bogus", ErrUsage), WantCode: 2,
	}, { // Test 3: An empty summary is a usage error.
		In: fmt.Errorf("%w: pass it as arguments", engine.ErrNoSummary), WantCode: 2,
	}, { // Test 4: A summary over the limit is a usage error.
		In: fmt.Errorf("%w: it must be 20000 bytes or less", ErrInputTooLong), WantCode: 2,
	}, { // Test 5: No analogy held up.
		In: fmt.Errorf("%w: closest was cars at fit 2", engine.ErrNoFit), WantCode: 3,
	}, { // Test 6: Claude refused, wrapped the way the engine wraps model errors.
		In:       fmt.Errorf("%w: skeleton step: %w", engine.ErrModelCall, claude.ErrRefused),
		WantCode: 4,
	}, { // Test 7: Claude rejected the credentials.
		In:       fmt.Errorf("%w: skeleton step: %w", engine.ErrModelCall, claude.ErrAuth),
		WantCode: 5,
	}, { // Test 8: A Ctrl-C.
		In: &QuietExitError{Code: codeInterrupted}, WantCode: 130,
	}, { // Test 9: A wrapped quiet exit keeps its code.
		In: fmt.Errorf("draft step: %w", &QuietExitError{Code: codeInterrupted}), WantCode: 130,
	}, { // Test 10: Credentials win over no fit when an error carries both.
		In: errors.Join(engine.ErrNoFit, claude.ErrAuth), WantCode: 5,
	}, { // Test 11: A refusal wins over no fit when an error carries both.
		In: errors.Join(engine.ErrNoFit, claude.ErrRefused), WantCode: 4,
	}, { // Test 12: A client that cannot be set up is a general failure.
		In: fmt.Errorf("%w: bad options", ErrClient), WantCode: 1,
	}, { // Test 13: An effort the client rejects is a usage error.
		In: fmt.Errorf("%w: %w", ErrClient, claude.ErrEffort), WantCode: 2,
	}, { // Test 14: A timeout the client rejects is a usage error.
		In: fmt.Errorf("%w: %w", ErrClient, claude.ErrTimeout), WantCode: 2,
	}, { // Test 15: Credentials win over a refusal when an error carries both.
		In: errors.Join(claude.ErrRefused, claude.ErrAuth), WantCode: 5,
	}, { // Test 16: A summary that holds one of ferry's own tags is a usage error.
		In: fmt.Errorf("%w: the summary holds a tag", engine.ErrReservedTag), WantCode: 2,
	}, { // Test 17: A failed model call with no better cause is a general failure.
		In: fmt.Errorf("%w: draft step: %w", engine.ErrModelCall, claude.ErrRateLimit), WantCode: 1,
	}, { // Test 18: A call that ran out of time is a general failure, not a usage error.
		In:       fmt.Errorf("%w: draft step: %w", engine.ErrModelCall, context.DeadlineExceeded),
		WantCode: 1,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(test.WantCode, exitCode(test.In)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestReport checks the printed message and the returned code for each kind of error.
func TestReport(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         error
		WantResult string
		WantCode   int
	}{{ // Test 0: Success prints nothing.
		In: nil, WantResult: "", WantCode: 0,
	}, { // Test 1: A usage error prints with the ferry prefix.
		In:         fmt.Errorf("%w: give the summary once", ErrUsage),
		WantResult: "ferry: usage: give the summary once\n", WantCode: 2,
	}, { // Test 2: A quiet exit prints nothing.
		In: &QuietExitError{Code: codeInterrupted}, WantResult: "", WantCode: 130,
	}, { // Test 3: No fit prints the engine's message.
		In:         fmt.Errorf("%w: closest was cars at fit 2", engine.ErrNoFit),
		WantResult: "ferry: no analogy fit: closest was cars at fit 2\n", WantCode: 3,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			code := report(&buf, test.In)
			if diff := cmp.Diff(test.WantResult, buf.String()); diff != "" {
				t.Errorf("message mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.WantCode, code); diff != "" {
				t.Errorf("code mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
