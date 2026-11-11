package config

import "strings"

// FlagSliceValue is a pflag.Value that collects a list. Each use of the flag adds to the list,
// and one use can add several items separated by commas.
type FlagSliceValue struct {
	// Vals holds the items collected so far, in the order given.
	Vals []string
}

// String returns the items joined by commas.
func (v *FlagSliceValue) String() string { return strings.Join(v.Vals, ",") }

// Set adds the comma-separated items in s, skipping empty ones.
func (v *FlagSliceValue) Set(s string) error {
	v.Vals = append(v.Vals, splitList(s)...)
	return nil
}

// Type returns the pflag type name, which help output shows as strings.
func (v *FlagSliceValue) Type() string { return "stringSlice" }
