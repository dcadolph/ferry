package config

import (
	"strings"

	"github.com/spf13/pflag"
)

// KeyWorld is the world flag name.
const KeyWorld = "world"

// DefaultWorld is empty, which leaves the choice of worlds to the engine.
var DefaultWorld []string

// FlagWorld names a preferred world to draw the analogy from. It can repeat, and one use can
// hold a comma-separated list.
var FlagWorld = pflag.Flag{
	Name:     KeyWorld,
	Usage:    "World you prefer, e.g. cars. It wins if it holds up. Repeat it or use commas.",
	Value:    &FlagSliceValue{},
	DefValue: strings.Join(DefaultWorld, ","),
}

// World returns the preferred worlds, most preferred first, or nil for the engine's defaults.
func World() []string { return loadStrings(KeyWorld, DefaultWorld) }
