package config

import "errors"

// ErrValue means a flag or environment value does not parse as the flag's type.
var ErrValue = errors.New("parse value")
