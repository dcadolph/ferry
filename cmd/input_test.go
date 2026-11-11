package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/ferry/internal/engine"
)

// writeTemp writes content to name under dir and returns the path.
func writeTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// TestSourceRead checks where the summary comes from, the terminal prompt, and the checks for
// conflicting sources, empty input, input over the limit, and input that is not UTF-8 text.
func TestSourceRead(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := writeTemp(t, dir, "situation.txt", "  from the file \n")
	long := strings.Repeat("a", maxSummaryBytes+1)
	longFile := writeTemp(t, dir, "long.txt", long)
	atLimit := strings.Repeat("b", maxSummaryBytes)
	const head, tail = "The first part.", "The second part."
	padded := head + strings.Repeat(" ", readCap) + tail
	paddedFile := writeTemp(t, dir, "padded.txt", padded)
	atCap := head + strings.Repeat("\n", readCap-len(head))
	atCapFile := writeTemp(t, dir, "at-cap.txt", atCap)
	latinFile := writeTemp(t, dir, "latin1.txt", "The caf\xe9 floods.")
	tests := []struct {
		Want       error
		Stdin      io.Reader
		File       string
		WantResult string
		WantPrompt string
		Args       []string
		Terminal   bool
	}{{ // Test 0: Arguments are joined by spaces and trimmed.
		Args: []string{"  we", "keep", "patching  "}, WantResult: "we keep patching",
	}, { // Test 1: A file is read and trimmed.
		File: file, WantResult: "from the file",
	}, { // Test 2: A file of - reads stdin.
		File: stdinPath, Stdin: strings.NewReader("from stdin\n"), WantResult: "from stdin",
	}, { // Test 3: Piped stdin is read without a prompt.
		Stdin: strings.NewReader("piped text"), WantResult: "piped text",
	}, { // Test 4: A terminal gets the prompt before the read.
		Stdin: strings.NewReader("typed text"), Terminal: true,
		WantResult: "typed text", WantPrompt: promptText + "\n",
	}, { // Test 5: Arguments win over stdin.
		Args: []string{"from", "args"}, Stdin: strings.NewReader("ignored"),
		WantResult: "from args",
	}, { // Test 6: Arguments and a file together are a usage error.
		Args: []string{"x"}, File: file, Want: ErrUsage,
	}, { // Test 7: Empty stdin has no summary.
		Stdin: strings.NewReader(""), Want: engine.ErrNoSummary,
	}, { // Test 8: Whitespace alone has no summary.
		Stdin: strings.NewReader(" \n\t "), Want: engine.ErrNoSummary,
	}, { // Test 9: An empty argument has no summary.
		Args: []string{""}, Want: engine.ErrNoSummary,
	}, { // Test 10: Arguments over the limit are rejected.
		Args: []string{long}, Want: ErrInputTooLong,
	}, { // Test 11: A file over the limit is rejected.
		File: longFile, Want: ErrInputTooLong,
	}, { // Test 12: Stdin over the limit is rejected.
		Stdin: strings.NewReader(long), Want: ErrInputTooLong,
	}, { // Test 13: A summary exactly at the limit passes whole.
		Stdin: strings.NewReader(atLimit), WantResult: atLimit,
	}, { // Test 14: Surrounding whitespace does not count toward the limit.
		Stdin: strings.NewReader("\n" + atLimit + "\n"), WantResult: atLimit,
	}, { // Test 15: Stdin far past the read cap is rejected, not cut.
		Stdin: strings.NewReader(strings.Repeat("c", readCap+10)), Want: ErrInputTooLong,
	}, { // Test 16: A missing file is a read error.
		File: filepath.Join(dir, "missing.txt"), Want: ErrRead,
	}, { // Test 17: A failing stdin is a read error.
		Stdin: iotest.ErrReader(errors.New("broken pipe")), Want: ErrRead,
	}, { // Test 18: A directory is a read error.
		File: dir, Want: ErrRead,
	}, { // Test 19: A file with blank space past the read cap and more text after it is
		// rejected, though the text before the cap is short.
		File: paddedFile, Want: ErrInputTooLong,
	}, { // Test 20: The same input on stdin is rejected too.
		Stdin: strings.NewReader(padded), Want: ErrInputTooLong,
	}, { // Test 21: A file exactly at the read cap is read whole.
		File: atCapFile, WantResult: head,
	}, { // Test 22: Stdin exactly at the read cap is read whole.
		Stdin: strings.NewReader(atCap), WantResult: head,
	}, { // Test 23: One byte past the read cap is rejected, even when that byte is blank.
		Stdin: strings.NewReader(atCap + "\n"), Want: ErrInputTooLong,
	}, { // Test 24: A file in another encoding is a usage error.
		File: latinFile, Want: ErrUsage,
	}, { // Test 25: Stdin that is not valid UTF-8 is a usage error.
		Stdin: strings.NewReader("ok \xff\xfe then \xc3\x28"), Want: ErrUsage,
	}, { // Test 26: An argument that is not valid UTF-8 is a usage error.
		Args: []string{"the", "caf\xe9"}, Want: ErrUsage,
	}, { // Test 27: A NUL byte is a usage error.
		Stdin: strings.NewReader("the service\x00crashes"), Want: ErrUsage,
	}, { // Test 28: Text outside ASCII that is valid UTF-8 passes whole.
		Stdin:      strings.NewReader("Le caf\u00e9 d\u00e9borde. \u6f0f\u308c\u308b\u3002"),
		WantResult: "Le caf\u00e9 d\u00e9borde. \u6f0f\u308c\u308b\u3002",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			var prompt bytes.Buffer
			src := source{
				args:     test.Args,
				file:     test.File,
				stdin:    test.Stdin,
				terminal: test.Terminal,
				prompt:   &prompt,
			}
			got, err := src.read(t.Context())
			if !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			if diff := cmp.Diff(test.WantResult, got); diff != "" {
				t.Errorf("summary mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.WantPrompt, prompt.String()); diff != "" {
				t.Errorf("prompt mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestReadStreamCanceled checks that a canceled context ends a read that would otherwise wait
// forever, the way a Ctrl-C ends a wait for typed input.
func TestReadStreamCanceled(t *testing.T) {
	t.Parallel()
	r, w := io.Pipe()
	t.Cleanup(func() { _ = w.Close() })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	got, err := readStream(ctx, r)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want %v", err, context.Canceled)
	}
	if got != nil {
		t.Errorf("read = %q, want nothing", got)
	}
}

// TestIsTerminal checks terminal detection on files and on other readers.
func TestIsTerminal(t *testing.T) {
	t.Parallel()
	regular, err := os.Create(filepath.Join(t.TempDir(), "plain.txt"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _ = regular.Close() })
	device, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = device.Close() })
	closed, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	if err := closed.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	tests := []struct {
		In         any
		WantResult bool
	}{{ // Test 0: A buffer is not a terminal.
		In: &bytes.Buffer{}, WantResult: false,
	}, { // Test 1: A regular file is not a terminal.
		In: regular, WantResult: false,
	}, { // Test 2: The null device is a character device and still not a terminal.
		In: device, WantResult: false,
	}, { // Test 3: A file that cannot be inspected is not a terminal.
		In: closed, WantResult: false,
	}, { // Test 4: Nil is not a terminal.
		In: nil, WantResult: false,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(test.WantResult, isTerminal(test.In)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
