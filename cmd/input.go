package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"

	"github.com/dcadolph/ferry/internal/engine"
)

// Limits and prompts for reading the summary.
const (
	// maxSummaryBytes caps the summary. Longer input is rejected, never cut.
	maxSummaryBytes = 20000
	// readCap bounds how much input is read, so a huge pipe cannot fill memory. Input that
	// runs past it is rejected, never cut, even when most of it is blank space.
	readCap = 1 << 20
	// stdinPath is the --file value that means stdin.
	stdinPath = "-"
	// promptText asks for the summary when stdin is an interactive terminal.
	promptText = "Describe the situation, then press Ctrl-D."
)

// source says where one run reads its summary from.
type source struct {
	// args holds the summary words from the command line.
	args []string
	// file is the --file path, "-" for stdin, or empty.
	file string
	// stdin is the standard input stream.
	stdin io.Reader
	// terminal reports whether stdin is an interactive terminal.
	terminal bool
	// prompt receives the prompt line when ferry waits for typed input.
	prompt io.Writer
}

// check returns a usage error when the summary is given both as arguments and with --file.
func (s source) check() error {
	if len(s.args) > 0 && s.file != "" {
		return fmt.Errorf("%w: give the summary as arguments or with --file, not both",
			ErrUsage)
	}
	return nil
}

// read returns the summary, trimmed of surrounding space, from the arguments joined by spaces,
// else the file, else stdin. It prompts first when stdin is a terminal, rejects a summary that
// is empty, over maxSummaryBytes, or not UTF-8 text, and returns the context's error if ctx
// ends while it waits.
func (s source) read(ctx context.Context) (string, error) {
	if err := s.check(); err != nil {
		return "", err
	}
	var text string
	switch {
	case len(s.args) > 0:
		text = strings.Join(s.args, " ")
	case s.file != "" && s.file != stdinPath:
		b, err := readFile(s.file)
		if err != nil {
			return "", err
		}
		text = string(b)
	default:
		if s.terminal {
			fmt.Fprintln(s.prompt, promptText)
		}
		b, err := readStream(ctx, s.stdin)
		if err != nil {
			return "", err
		}
		text = string(b)
	}
	return checkSummary(text)
}

// readFile returns the whole file at path, or ErrInputTooLong when it runs past readCap bytes.
func readFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRead, err)
	}
	defer func() { _ = f.Close() }()
	b, err := readCapped(f)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRead, err)
	}
	return checkCap(b)
}

// readStream reads r to EOF and returns ErrInputTooLong when it runs past readCap bytes. It
// returns the context's error as soon as ctx ends, so Ctrl-C works while ferry waits for typed
// input. The read itself keeps running until r yields, which is harmless because the process
// exits right after.
func readStream(ctx context.Context, r io.Reader) ([]byte, error) {
	var (
		b    []byte
		err  error
		done = make(chan struct{})
	)
	go func() {
		b, err = readCapped(r)
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRead, err)
	}
	return checkCap(b)
}

// readCapped reads r to EOF, stopping one byte past readCap. That extra byte is how checkCap
// tells input that fits from input that was longer.
func readCapped(r io.Reader) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, readCap+1))
}

// checkCap returns b, or ErrInputTooLong when b holds more than readCap bytes, which means
// the input did not end inside the cap.
func checkCap(b []byte) ([]byte, error) {
	if len(b) > readCap {
		return nil, fmt.Errorf("%w: the input runs past %d bytes, and the summary must be %d "+
			"bytes or less", ErrInputTooLong, readCap, maxSummaryBytes)
	}
	return b, nil
}

// checkSummary trims text and rejects it when it is empty, is not valid UTF-8 text, or is
// longer than maxSummaryBytes. Text in another encoding is rejected, never repaired, because
// a repair would send something the user did not write.
func checkSummary(text string) (string, error) {
	text = strings.TrimSpace(text)
	switch {
	case text == "":
		return "", fmt.Errorf("%w: pass it as arguments, with --file, or on stdin",
			engine.ErrNoSummary)
	case !utf8.ValidString(text) || strings.ContainsRune(text, 0):
		return "", fmt.Errorf("%w: the summary must be UTF-8 text", ErrUsage)
	case len(text) > maxSummaryBytes:
		return "", fmt.Errorf("%w: it must be %d bytes or less", ErrInputTooLong, maxSummaryBytes)
	}
	return text, nil
}

// isTerminal reports whether v is a file attached to an interactive terminal. Other character
// devices, such as the null device, do not count.
func isTerminal(v any) bool {
	f, ok := v.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}
