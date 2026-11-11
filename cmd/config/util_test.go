package config

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// TestEnvKey checks the flag name to environment variable mapping.
func TestEnvKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         string
		WantResult string
	}{{ // Test 0: A single word gets the prefix and upper case.
		In: "audience", WantResult: "FERRY_AUDIENCE",
	}, { // Test 1: Dashes become underscores.
		In: "log-level", WantResult: "FERRY_LOG_LEVEL",
	}, { // Test 2: Every dash is replaced.
		In: "show-work", WantResult: "FERRY_SHOW_WORK",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(test.WantResult, envKey(test.In)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestSplitList checks trimming and dropping empty items.
func TestSplitList(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         string
		WantResult []string
	}{{ // Test 0: Empty input gives no items.
		In: "", WantResult: nil,
	}, { // Test 1: One item.
		In: "cars", WantResult: []string{"cars"},
	}, { // Test 2: Spaces are trimmed and blanks dropped.
		In: " cars , , kitchens,", WantResult: []string{"cars", "kitchens"},
	}, { // Test 3: Spaces inside an item stay.
		In: "home repair,board games", WantResult: []string{"home repair", "board games"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := splitList(test.In)
			if diff := cmp.Diff(test.WantResult, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
