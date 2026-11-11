package logutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// TestNew checks that New honors the level and rejects names zap does not know.
func TestNew(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Want       error
		In         string
		WantLevel  zapcore.Level
		WantLogger bool
	}{{ // Test 0: Warn is the CLI default.
		In: "warn", WantLevel: zapcore.WarnLevel, WantLogger: true, Want: nil,
	}, { // Test 1: Debug enables everything.
		In: "debug", WantLevel: zapcore.DebugLevel, WantLogger: true, Want: nil,
	}, { // Test 2: Upper case names parse too.
		In: "ERROR", WantLevel: zapcore.ErrorLevel, WantLogger: true, Want: nil,
	}, { // Test 3: An unknown name is rejected.
		In: "loud", WantLogger: false, Want: ErrLevel,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			log, err := New(&bytes.Buffer{}, test.In)
			if !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			if diff := cmp.Diff(test.WantLogger, log != nil); diff != "" {
				t.Fatalf("logger presence mismatch (-want +got):\n%s", diff)
			}
			if log == nil {
				return
			}
			if diff := cmp.Diff(test.WantLevel, log.Level()); diff != "" {
				t.Errorf("level mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestNewWriter checks that the logger writes whole lines to the writer it is given, at its
// level and above, with the message and the fields on the line.
func TestNewWriter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Level     string
		WantLines []string
	}{{ // Test 0: Debug writes every line.
		Level: "debug", WantLines: []string{"debug\tcall done", "warn\tstep failed"},
	}, { // Test 1: Warn leaves the debug line out.
		Level: "warn", WantLines: []string{"warn\tstep failed"},
	}, { // Test 2: Error writes neither.
		Level: "error", WantLines: nil,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			log, err := New(&buf, test.Level)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			log.Debug("call done", zap.String("step", "draft"))
			log.Warn("step failed", zap.String("step", "check"))
			if err := log.Sync(); err != nil {
				t.Fatalf("sync: %v", err)
			}
			got := buf.String()
			if diff := cmp.Diff(len(test.WantLines), strings.Count(got, "\n")); diff != "" {
				t.Errorf("line count mismatch (-want +got):\n%s\n%s", diff, got)
			}
			for _, want := range test.WantLines {
				if !strings.Contains(got, want) {
					t.Errorf("output lacks %q:\n%s", want, got)
				}
			}
			if len(test.WantLines) > 0 && !strings.Contains(got, `{"step": "check"}`) {
				t.Errorf("output lacks the fields of the warn line:\n%s", got)
			}
		})
	}
}

// TestNewPanics checks that a nil writer is a developer error.
func TestNewPanics(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Level       string
		WantMessage string
	}{{ // Test 0: A valid level with no writer.
		Level: "warn", WantMessage: "writer required",
	}, { // Test 1: The writer is checked before the level.
		Level: "loud", WantMessage: "writer required",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("New(nil, ...) did not panic")
				}
				if msg := fmt.Sprint(r); !strings.Contains(msg, test.WantMessage) {
					t.Errorf("panic %q does not mention %q", msg, test.WantMessage)
				}
			}()
			_, _ = New(nil, test.Level)
		})
	}
}

// TestWrapLogger checks the round trip through a context and the no-op fallback.
func TestWrapLogger(t *testing.T) {
	t.Parallel()
	example := zap.NewExample()
	tests := []struct {
		Ctx        context.Context
		WantLogger *zap.Logger
		WantNop    bool
	}{{ // Test 0: A wrapped logger comes back unchanged.
		Ctx: WrapLogger(context.Background(), example), WantLogger: example, WantNop: false,
	}, { // Test 1: A bare context yields a no-op logger, never nil.
		Ctx: context.Background(), WantLogger: nil, WantNop: true,
	}, { // Test 2: A wrapped nil logger yields a no-op logger.
		Ctx: WrapLogger(context.Background(), nil), WantLogger: nil, WantNop: true,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := UnwrapLogger(test.Ctx)
			if got == nil {
				t.Fatal("UnwrapLogger returned nil")
			}
			if test.WantNop {
				if got.Core().Enabled(zapcore.FatalLevel) {
					t.Error("fallback logger is enabled, want a no-op logger")
				}
				return
			}
			if got != test.WantLogger {
				t.Errorf("UnwrapLogger = %p, want %p", got, test.WantLogger)
			}
		})
	}
}
