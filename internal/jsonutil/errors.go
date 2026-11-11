package jsonutil

import "errors"

// ErrMarshal means a value could not be encoded as JSON.
var ErrMarshal = errors.New("marshal json")
