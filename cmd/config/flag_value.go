package config

import (
	"fmt"
	"strconv"
	"time"
)

// FlagValue is a pflag.Value backed by a raw string. Every scalar flag in the config package
// uses it so flag state lives in one shape.
type FlagValue struct {
	// Val is the current raw value.
	Val string
	// ValType is the type name pflag shows in help output: string, bool, or duration.
	ValType string
}

// String returns the current raw value.
func (v *FlagValue) String() string { return v.Val }

// Set checks s against the value type and stores it.
func (v *FlagValue) Set(s string) error {
	if err := checkValue(v.ValType, s); err != nil {
		return fmt.Errorf("%w: %w", ErrValue, err)
	}
	v.Val = s
	return nil
}

// checkValue returns the parse error when s does not parse as valType. A string takes
// anything. The command line and the environment both go through it, so each rejects the same
// values.
func checkValue(valType, s string) error {
	var err error
	switch valType {
	case "bool":
		_, err = strconv.ParseBool(s)
	case "duration":
		_, err = time.ParseDuration(s)
	}
	return err
}

// Type returns the pflag type name.
func (v *FlagValue) Type() string { return v.ValType }
