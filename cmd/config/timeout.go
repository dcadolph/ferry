package config

import (
	"time"

	"github.com/spf13/pflag"
)

// KeyTimeout is the timeout flag name.
const KeyTimeout = "timeout"

// DefaultTimeout is the time limit for one model call, retries included.
var DefaultTimeout = 5 * time.Minute

// FlagTimeout caps each model call.
var FlagTimeout = pflag.Flag{
	Name:     KeyTimeout,
	Usage:    "Time limit for one model call, retries included, e.g. 90s or 10m.",
	Value:    &FlagValue{Val: DefaultTimeout.String(), ValType: "duration"},
	DefValue: DefaultTimeout.String(),
}

// Timeout returns the time limit for one model call, retries included.
func Timeout() time.Duration { return loadDuration(KeyTimeout, DefaultTimeout) }
