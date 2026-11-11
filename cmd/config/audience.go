package config

import "github.com/spf13/pflag"

// KeyAudience is the audience flag name.
const KeyAudience = "audience"

// DefaultAudience is empty, which leaves the listener to the engine's default.
var DefaultAudience = ""

// FlagAudience describes who will hear the analogy.
var FlagAudience = pflag.Flag{
	Name:     KeyAudience,
	Usage:    `Who will hear the analogy, e.g. "my director" or "new engineers".`,
	Value:    &FlagValue{Val: DefaultAudience, ValType: "string"},
	DefValue: DefaultAudience,
}

// Audience returns who will hear the analogy, or empty for the engine's default.
func Audience() string { return loadString(KeyAudience, DefaultAudience) }
