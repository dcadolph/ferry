package config

import "github.com/spf13/pflag"

// KeyFile is the file flag name.
const KeyFile = "file"

// DefaultFile is empty, which reads the summary from the arguments or stdin.
var DefaultFile = ""

// FlagFile reads the summary from a file.
var FlagFile = pflag.Flag{
	Name:     KeyFile,
	Usage:    "Read the summary from `path`. Use - for stdin.",
	Value:    &FlagValue{Val: DefaultFile, ValType: "string"},
	DefValue: DefaultFile,
}

// File returns the summary file path, "-" for stdin, or empty when none is set.
func File() string { return loadString(KeyFile, DefaultFile) }
