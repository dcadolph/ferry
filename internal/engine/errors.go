package engine

import "errors"

var (
	// ErrNoSummary means the input held no summary to work from.
	ErrNoSummary = errors.New("no summary")
	// ErrBadReply means a model reply could not be decoded or used, even after one corrective
	// retry.
	ErrBadReply = errors.New("bad model reply")
	// ErrNoFit means no drafted analogy held up when it was checked.
	ErrNoFit = errors.New("no analogy fit")
	// ErrIncomplete means the final analogy still lacked a required part after one corrective
	// retry.
	ErrIncomplete = errors.New("incomplete analogy")
	// ErrModelCall means a call to the model failed.
	ErrModelCall = errors.New("model call")
	// ErrRender means a prompt template could not be rendered.
	ErrRender = errors.New("render prompt")
	// ErrReservedTag means the summary, the audience, or a world holds one of the tags ferry
	// uses to mark its own text in a prompt.
	ErrReservedTag = errors.New("reserved tag")
)
