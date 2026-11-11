package engine

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// TestRoundOneWorlds checks how preferred and default worlds merge, de-duplicate, and cap.
func TestRoundOneWorlds(t *testing.T) {
	t.Parallel()
	defaults := []string{"cars", "kitchens", "construction", "sports"}
	tests := []struct {
		Preferred  []string
		Defaults   []string
		Limit      int
		WantResult []string
	}{{ // Test 0: No preferred worlds gives the defaults.
		Defaults: defaults, Limit: 6, WantResult: defaults,
	}, { // Test 1: Preferred worlds come first, and a repeated default is dropped.
		Preferred: []string{"gardening", "cars"}, Defaults: defaults, Limit: 6,
		WantResult: []string{"gardening", "cars", "kitchens", "construction", "sports"},
	}, { // Test 2: Duplicates match without regard to case or spacing, and the first one stays.
		Preferred: []string{"Home  Plumbing", " home plumbing ", "CARS"}, Defaults: defaults,
		Limit:      6,
		WantResult: []string{"Home Plumbing", "CARS", "kitchens", "construction", "sports"},
	}, { // Test 3: Blank entries are skipped.
		Preferred: []string{"", "  ", "boats"}, Defaults: []string{"", "cars"}, Limit: 6,
		WantResult: []string{"boats", "cars"},
	}, { // Test 4: Defaults fill the round only up to the cap.
		Preferred: []string{"boats", "trains"}, Defaults: defaults, Limit: 3,
		WantResult: []string{"boats", "trains", "cars"},
	}, { // Test 5: Preferred worlds are capped too, in order, and no default joins them.
		Preferred: []string{"a", "b", "c", "d"}, Defaults: defaults, Limit: 3,
		WantResult: []string{"a", "b", "c"},
	}, { // Test 6: Duplicate defaults appear once.
		Defaults: []string{"cars", "Cars", "kitchens"}, Limit: 6,
		WantResult: []string{"cars", "kitchens"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := roundOneWorlds(test.Preferred, test.Defaults, test.Limit)
			if diff := cmp.Diff(test.WantResult, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestDistinctWorlds checks that worlds are cleaned and that repeats and blanks are dropped.
func TestDistinctWorlds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         []string
		WantResult []string
	}{{ // Test 0: No worlds gives none.
		In: nil, WantResult: nil,
	}, { // Test 1: Repeats that differ in case or spacing count once, and the first one stays.
		In:         []string{"cars", "Cars", " cars ", "home  plumbing", "Home Plumbing"},
		WantResult: []string{"cars", "home plumbing"},
	}, { // Test 2: Blank entries are skipped and order is kept.
		In: []string{"", "sports", "  ", "boats"}, WantResult: []string{"sports", "boats"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := DistinctWorlds(test.In)
			if diff := cmp.Diff(test.WantResult, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestNextWorlds checks that later rounds draw only unused fallback worlds, capped.
func TestNextWorlds(t *testing.T) {
	t.Parallel()
	fallbacks := []string{"home plumbing", "gardening", "air travel", "household budgets"}
	tests := []struct {
		Fallbacks  []string
		Used       map[string]bool
		Limit      int
		WantResult []string
		WantUsed   map[string]bool
	}{{ // Test 0: Nothing used gives every fallback.
		Fallbacks: fallbacks, Used: map[string]bool{}, Limit: 6,
		WantResult: fallbacks, WantUsed: map[string]bool{},
	}, { // Test 1: Used worlds are skipped without regard to case, and used is left alone.
		Fallbacks: []string{"Gardening", "air travel"}, Used: map[string]bool{"gardening": true},
		Limit:      6,
		WantResult: []string{"air travel"},
		WantUsed:   map[string]bool{"gardening": true},
	}, { // Test 2: The cap applies.
		Fallbacks: fallbacks, Used: map[string]bool{}, Limit: 2,
		WantResult: []string{"home plumbing", "gardening"}, WantUsed: map[string]bool{},
	}, { // Test 3: Every fallback used gives none.
		Fallbacks: []string{"cars"}, Used: map[string]bool{"cars": true}, Limit: 6,
		WantResult: nil, WantUsed: map[string]bool{"cars": true},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := nextWorlds(test.Fallbacks, test.Used, test.Limit)
			if diff := cmp.Diff(test.WantResult, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("worlds mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.WantUsed, test.Used, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("used mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
