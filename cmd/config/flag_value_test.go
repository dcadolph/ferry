package config

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestFlagValue checks Set validation for each value type and the pflag type name.
func TestFlagValue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Want       error
		In         string
		ValType    string
		WantResult string
	}{{ // Test 0: A valid bool is stored.
		In: "true", ValType: "bool", WantResult: "true", Want: nil,
	}, { // Test 1: An invalid bool is rejected and the old value kept.
		In: "maybe", ValType: "bool", WantResult: "old", Want: ErrValue,
	}, { // Test 2: A valid duration is stored as written.
		In: "90s", ValType: "duration", WantResult: "90s", Want: nil,
	}, { // Test 3: A duration with no unit is rejected.
		In: "30", ValType: "duration", WantResult: "old", Want: ErrValue,
	}, { // Test 4: A string takes anything.
		In: "anything at all", ValType: "string", WantResult: "anything at all", Want: nil,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			v := &FlagValue{Val: "old", ValType: test.ValType}
			if err := v.Set(test.In); !errors.Is(err, test.Want) {
				t.Fatalf("Set(%q) error = %v, want %v", test.In, err, test.Want)
			}
			if diff := cmp.Diff(test.WantResult, v.String()); diff != "" {
				t.Errorf("value mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.ValType, v.Type()); diff != "" {
				t.Errorf("type mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestFlagSliceValue checks that repeated and comma-separated uses add up in order.
func TestFlagSliceValue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         []string
		WantResult string
	}{{ // Test 0: No uses leave the list empty.
		In: nil, WantResult: "",
	}, { // Test 1: One use with one item.
		In: []string{"cars"}, WantResult: "cars",
	}, { // Test 2: Commas split one use into several items.
		In: []string{"cars, kitchens"}, WantResult: "cars,kitchens",
	}, { // Test 3: Repeated uses add up in order and empty items are skipped.
		In: []string{"sports", "", "boats,,trains"}, WantResult: "sports,boats,trains",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			v := &FlagSliceValue{}
			for _, raw := range test.In {
				if err := v.Set(raw); err != nil {
					t.Fatalf("Set(%q): %v", raw, err)
				}
			}
			if diff := cmp.Diff(test.WantResult, v.String()); diff != "" {
				t.Errorf("value mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff("stringSlice", v.Type()); diff != "" {
				t.Errorf("type mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
