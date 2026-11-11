package util

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestCollapseSpace checks trimming and the collapse of every kind of whitespace run.
func TestCollapseSpace(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         string
		WantResult string
	}{{ // Test 0: Empty input stays empty.
		In: "", WantResult: "",
	}, { // Test 1: Whitespace alone collapses to nothing.
		In: " \n\t ", WantResult: "",
	}, { // Test 2: Clean text is unchanged.
		In: "home plumbing", WantResult: "home plumbing",
	}, { // Test 3: Surrounding space is trimmed and inner runs collapse.
		In: "  home   plumbing ", WantResult: "home plumbing",
	}, { // Test 4: Line breaks and tabs count as space.
		In: "too\nlong\t here\r\n", WantResult: "too long here",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(test.WantResult, CollapseSpace(test.In)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestInc checks that Inc adds one.
func TestInc(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         int
		WantResult int
	}{{ // Test 0: Zero becomes one, so a list numbers from one.
		In: 0, WantResult: 1,
	}, { // Test 1: A later index.
		In: 6, WantResult: 7,
	}, { // Test 2: A negative number.
		In: -1, WantResult: 0,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(test.WantResult, Inc(test.In)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
