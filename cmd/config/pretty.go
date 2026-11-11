package config

import (
	"strconv"

	"github.com/spf13/pflag"
)

// KeyPretty is the pretty flag name.
const KeyPretty = "pretty"

// DefaultPretty is false, meaning compact JSON.
var DefaultPretty = false

// FlagPretty indents the JSON output.
var FlagPretty = pflag.Flag{
	Name:        KeyPretty,
	Usage:       "Indent the --json output.",
	Value:       &FlagValue{Val: strconv.FormatBool(DefaultPretty), ValType: "bool"},
	DefValue:    strconv.FormatBool(DefaultPretty),
	NoOptDefVal: "true",
}

// Pretty reports whether JSON output should be indented.
func Pretty() bool { return loadBool(KeyPretty, DefaultPretty) }
