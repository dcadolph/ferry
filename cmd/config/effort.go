package config

import (
	"strings"

	"github.com/spf13/pflag"

	"github.com/dcadolph/ferry/internal/claude"
)

// KeyEffort is the effort flag name.
const KeyEffort = "effort"

// DefaultEffort is the reasoning effort used when none is set.
var DefaultEffort = claude.DefaultEffort

// FlagEffort sets how hard Claude reasons at each step.
var FlagEffort = pflag.Flag{
	Name:     KeyEffort,
	Usage:    "Reasoning effort, one of " + strings.Join(claude.Efforts(), ", ") + ".",
	Value:    &FlagValue{Val: DefaultEffort, ValType: "string"},
	DefValue: DefaultEffort,
}

// Effort returns the reasoning effort.
func Effort() string { return loadString(KeyEffort, DefaultEffort) }
