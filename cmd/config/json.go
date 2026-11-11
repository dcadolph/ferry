package config

import (
	"strconv"

	"github.com/spf13/pflag"
)

// KeyJSON is the json flag name.
const KeyJSON = "json"

// DefaultJSON is false, meaning text output.
var DefaultJSON = false

// FlagJSON prints the result as JSON.
var FlagJSON = pflag.Flag{
	Name:        KeyJSON,
	Usage:       "Print the result as JSON, with the skeleton and every candidate.",
	Value:       &FlagValue{Val: strconv.FormatBool(DefaultJSON), ValType: "bool"},
	DefValue:    strconv.FormatBool(DefaultJSON),
	NoOptDefVal: "true",
}

// JSON reports whether stdout should carry JSON.
func JSON() bool { return loadBool(KeyJSON, DefaultJSON) }
