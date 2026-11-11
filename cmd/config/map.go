// Package config defines ferry's flags and resolves each setting from the command line, then
// the environment, then the built-in default.
package config

import (
	"fmt"
	"os"
	"slices"

	"github.com/spf13/pflag"
)

// flags indexes every flag by key so lookups and resets can walk them.
var flags = map[string]*pflag.Flag{
	KeyAudience: &FlagAudience,
	KeyWorld:    &FlagWorld,
	KeyModel:    &FlagModel,
	KeyEffort:   &FlagEffort,
	KeyFile:     &FlagFile,
	KeyJSON:     &FlagJSON,
	KeyPretty:   &FlagPretty,
	KeyShowWork: &FlagShowWork,
	KeyTimeout:  &FlagTimeout,
	KeyLogLevel: &FlagLogLevel,
}

// Reset puts every flag back to its default and marks it unset. Tests use it between command
// runs because flag state is shared package-wide. It panics when a default does not parse as
// its flag's type, which is a developer error.
func Reset() {
	for key, f := range flags {
		if v, ok := f.Value.(*FlagSliceValue); ok {
			v.Vals = splitList(f.DefValue)
		} else if err := f.Value.Set(f.DefValue); err != nil {
			panic(fmt.Sprintf("config.Reset: default for --%s: %v", key, err))
		}
		f.Changed = false
	}
}

// CheckEnv returns ErrValue, naming the variable, when a FERRY_ variable holds a value that
// does not parse as its flag's type. A variable whose flag is set on the command line is not
// checked, since the flag wins. Call it before the getters: they fall back to the default on a
// value that does not parse, and CheckEnv is what keeps that from happening silently.
func CheckEnv() error {
	keys := make([]string, 0, len(flags))
	for key := range flags {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		f := flags[key]
		v, ok := f.Value.(*FlagValue)
		if !ok || f.Changed {
			continue
		}
		raw, set := os.LookupEnv(envKey(key))
		if !set {
			continue
		}
		if err := checkValue(v.ValType, raw); err != nil {
			return fmt.Errorf("%w: %s: %w", ErrValue, envKey(key), err)
		}
	}
	return nil
}
