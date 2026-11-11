package claude

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Errors returned by the Claude client.
var (
	// ErrEffort means the effort is not one of low, medium, high, xhigh, or max.
	ErrEffort = errors.New("invalid effort")
	// ErrMaxTokens means the reply token cap is not positive.
	ErrMaxTokens = errors.New("invalid max tokens")
	// ErrTimeout means the request timeout is negative.
	ErrTimeout = errors.New("invalid timeout")
	// ErrAuth means Claude rejected the credentials or their permissions.
	ErrAuth = errors.New("claude auth rejected")
	// ErrRateLimit means Claude rate limited the request.
	ErrRateLimit = errors.New("claude rate limited")
	// ErrUnavailable means Claude failed on its side or was overloaded.
	ErrUnavailable = errors.New("claude unavailable")
	// ErrAPI means Claude rejected the request for another reason.
	ErrAPI = errors.New("claude api error")
	// ErrRequest means the request got no answer from Claude, e.g. the connection failed.
	ErrRequest = errors.New("claude request failed")
	// ErrRefused means Claude declined the request.
	ErrRefused = errors.New("claude refused")
	// ErrTruncated means the reply hit a token limit before it was complete.
	ErrTruncated = errors.New("claude reply truncated")
	// ErrEmpty means the reply held no text.
	ErrEmpty = errors.New("claude reply empty")
)

// timeoutError means a call ran past the client's time limit. It matches
// context.DeadlineExceeded with errors.Is.
type timeoutError struct {
	// limit is the time limit the call ran past.
	limit time.Duration
}

// Error names the limit and what the user can change.
func (e *timeoutError) Error() string {
	return fmt.Sprintf("timed out after %s: raise --timeout or lower --effort", e.limit)
}

// Unwrap returns context.DeadlineExceeded, the cause of every timeout.
func (e *timeoutError) Unwrap() error {
	return context.DeadlineExceeded
}
