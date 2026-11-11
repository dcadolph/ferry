package config

import (
	"github.com/spf13/pflag"

	"github.com/dcadolph/ferry/internal/claude"
)

// KeyModel is the model flag name.
const KeyModel = "model"

// DefaultModel is the Claude model used when none is set.
var DefaultModel = claude.DefaultModel

// FlagModel picks the Claude model.
var FlagModel = pflag.Flag{
	Name:     KeyModel,
	Usage:    "Claude model ID.",
	Value:    &FlagValue{Val: DefaultModel, ValType: "string"},
	DefValue: DefaultModel,
}

// Model returns the Claude model ID.
func Model() string { return loadString(KeyModel, DefaultModel) }
