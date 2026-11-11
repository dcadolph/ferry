package logutil

import "errors"

// ErrLevel means the log level name is not one zap knows.
var ErrLevel = errors.New("unknown log level")
