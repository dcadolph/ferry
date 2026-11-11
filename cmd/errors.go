package cmd

import (
	"errors"
	"fmt"
)

var (
	// ErrUsage means the command line asks for something ferry cannot do.
	ErrUsage = errors.New("usage")
	// ErrInputTooLong means the summary is over the size limit.
	ErrInputTooLong = errors.New("summary too long")
	// ErrRead means the summary could not be read.
	ErrRead = errors.New("read summary")
	// ErrClient means the Claude client could not be set up.
	ErrClient = errors.New("set up claude client")
	// ErrWrite means the privacy reminder or the result could not be written.
	ErrWrite = errors.New("write output")
)

// QuietExitError ends the run with Code and prints nothing. It marks aborts the user started,
// such as Ctrl-C, where an error message would only repeat what they already know.
type QuietExitError struct {
	// Code is the process exit code.
	Code int
}

// Error describes the exit for logs and tests. Execute never prints it.
func (e *QuietExitError) Error() string {
	return fmt.Sprintf("quiet exit %d", e.Code)
}
