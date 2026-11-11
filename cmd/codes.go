package cmd

import (
	"errors"

	"github.com/dcadolph/ferry/internal/claude"
	"github.com/dcadolph/ferry/internal/engine"
)

// Exit codes for the ferry binary.
const (
	// codeOK means the run printed an analogy.
	codeOK = 0
	// codeError means a general failure, such as a network error, a file error, or no
	// credentials found.
	codeError = 1
	// codeUsage means the command line or the summary could not be used.
	codeUsage = 2
	// codeNoFit means no drafted analogy held up against the situation.
	codeNoFit = 3
	// codeRefused means Claude declined the request.
	codeRefused = 4
	// codeAuth means Claude would not accept the credentials.
	codeAuth = 5
	// codeInterrupted means the user pressed Ctrl-C.
	codeInterrupted = 130
)

// exitCode maps err to the process exit code. When an error carries more than one cause, the
// one the user can act on first wins: credentials, then a refusal, then no fit. An effort or
// timeout the Claude client rejects counts as a usage error, the same as one ferry rejects.
func exitCode(err error) int {
	var quiet *QuietExitError
	switch {
	case err == nil:
		return codeOK
	case errors.As(err, &quiet):
		return quiet.Code
	case errors.Is(err, ErrUsage), errors.Is(err, ErrInputTooLong),
		errors.Is(err, engine.ErrNoSummary), errors.Is(err, engine.ErrReservedTag),
		errors.Is(err, claude.ErrEffort), errors.Is(err, claude.ErrTimeout):
		return codeUsage
	case errors.Is(err, claude.ErrAuth):
		return codeAuth
	case errors.Is(err, claude.ErrRefused):
		return codeRefused
	case errors.Is(err, engine.ErrNoFit):
		return codeNoFit
	default:
		return codeError
	}
}
