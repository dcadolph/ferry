package render

import "errors"

var (
	// ErrRender means the text template could not be executed.
	ErrRender = errors.New("render text")
	// ErrWrite means the rendered text could not be written.
	ErrWrite = errors.New("write text")
)
