package config

import "github.com/spf13/pflag"

// KeyLogLevel is the log-level flag name.
const KeyLogLevel = "log-level"

// DefaultLogLevel keeps stderr quiet apart from warnings and errors.
var DefaultLogLevel = "warn"

// FlagLogLevel sets the lowest log level written to stderr.
var FlagLogLevel = pflag.Flag{
	Name:     KeyLogLevel,
	Usage:    "Lowest log level written to stderr: debug, info, warn, or error.",
	Value:    &FlagValue{Val: DefaultLogLevel, ValType: "string"},
	DefValue: DefaultLogLevel,
}

// LogLevel returns the lowest log level written to stderr.
func LogLevel() string { return loadString(KeyLogLevel, DefaultLogLevel) }
