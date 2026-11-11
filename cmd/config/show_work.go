package config

import (
	"strconv"

	"github.com/spf13/pflag"
)

// KeyShowWork is the show-work flag name.
const KeyShowWork = "show-work"

// DefaultShowWork is false, meaning only the winning analogy is printed.
var DefaultShowWork = false

// FlagShowWork adds the skeleton and every candidate to the text output.
var FlagShowWork = pflag.Flag{
	Name:        KeyShowWork,
	Usage:       "Also print the skeleton and every candidate with its verdict.",
	Value:       &FlagValue{Val: strconv.FormatBool(DefaultShowWork), ValType: "bool"},
	DefValue:    strconv.FormatBool(DefaultShowWork),
	NoOptDefVal: "true",
}

// ShowWork reports whether the text output should include the skeleton and every candidate.
func ShowWork() bool { return loadBool(KeyShowWork, DefaultShowWork) }
