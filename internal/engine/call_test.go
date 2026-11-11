package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/ferry/internal/schema"
)

// TestReplySchemas checks that the schema of every step's reply type closes every object,
// requires every property, and uses no keyword the API rejects.
func TestReplySchemas(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In          any
		WantObjects int
	}{{ // Test 0: The skeleton reply and its players.
		In: Skeleton{}, WantObjects: 2,
	}, { // Test 1: The draft reply and its mappings.
		In: Candidate{}, WantObjects: 2,
	}, { // Test 2: The check reply.
		In: Verdict{}, WantObjects: 1,
	}, { // Test 3: The final reply and its mappings.
		In: Analogy{}, WantObjects: 2,
	}}
	rejected := []string{"minimum", "maximum", "minLength", "maxLength", "$ref"}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			objects := 0
			var walk func(path string, node map[string]any)
			walk = func(path string, node map[string]any) {
				for _, key := range rejected {
					if _, ok := node[key]; ok {
						t.Errorf("%s uses the rejected keyword %s", path, key)
					}
				}
				switch node["type"] {
				case "object":
					objects++
					if node["additionalProperties"] != false {
						t.Errorf("%s does not set additionalProperties to false", path)
					}
					props, ok := node["properties"].(map[string]any)
					if !ok {
						t.Fatalf("%s has no properties map", path)
					}
					list, ok := node["required"].([]any)
					if !ok {
						t.Fatalf("%s has no required list", path)
					}
					required := make([]string, 0, len(list))
					for _, name := range list {
						required = append(required, fmt.Sprint(name))
					}
					slices.Sort(required)
					names := slices.Sorted(maps.Keys(props))
					if diff := cmp.Diff(names, required); diff != "" {
						t.Errorf("%s required mismatch (-properties +required):\n%s", path, diff)
					}
					for _, name := range names {
						child, ok := props[name].(map[string]any)
						if !ok {
							t.Fatalf("%s.%s is not a schema", path, name)
						}
						walk(path+"."+name, child)
					}
				case "array":
					items, ok := node["items"].(map[string]any)
					if !ok {
						t.Fatalf("%s has no items schema", path)
					}
					walk(path+"[]", items)
				}
			}
			var root map[string]any
			if err := json.Unmarshal(schema.For(test.In), &root); err != nil {
				t.Fatalf("decode schema: %v", err)
			}
			walk(fmt.Sprintf("%T", test.In), root)
			if diff := cmp.Diff(test.WantObjects, objects); diff != "" {
				t.Errorf("object count mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestReplySchemaOrder checks the order each reply type's properties go out in. The model
// writes a reply's fields in that order, so a field has to come after the ones it depends on:
// the problems before the fit and the fix, and the spoken text before the decision line and
// the breaking point.
func TestReplySchemaOrder(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In        any
		WantOrder []string
	}{{ // Test 0: The skeleton ends on the causal chain.
		In: Skeleton{},
		WantOrder: []string{
			"players", "constraint", "tradeoff", "cost_of_inaction", "decision", "point",
			"causal_chain",
		},
	}, { // Test 1: A draft writes its spoken text before the parts drawn from it.
		In:        Candidate{},
		WantOrder: []string{"world", "spoken", "decision_line", "mappings", "breaks_down"},
	}, { // Test 2: A verdict lists the problems first and scores last.
		In: Verdict{},
		WantOrder: []string{
			"problems", "same_shape", "familiar_world", "points_at_decision", "fit", "fix",
		},
	}, { // Test 3: The final analogy writes its spoken text first.
		In:        Analogy{},
		WantOrder: []string{"world", "spoken", "mappings", "breaks_down"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := string(schema.For(test.In))
			last := -1
			for _, name := range test.WantOrder {
				at := strings.Index(got, `"`+name+`":{`)
				if at <= last {
					t.Errorf("property %s is at %d, want it after %d in %s", name, at, last, got)
				}
				last = at
			}
			quoted := make([]string, 0, len(test.WantOrder))
			for _, name := range test.WantOrder {
				quoted = append(quoted, `"`+name+`"`)
			}
			required := `"required":[` + strings.Join(quoted, ",") +
				`],"additionalProperties":false}`
			if !strings.HasSuffix(got, required) {
				t.Errorf("schema does not end with %s:\n%s", required, got)
			}
		})
	}
}

// TestEach checks that each runs every index once, never above the worker cap.
func TestEach(t *testing.T) {
	t.Parallel()
	tests := []struct {
		N         int
		Workers   int
		WantCalls []int
	}{{ // Test 0: No work.
		N: 0, Workers: 2, WantCalls: []int{},
	}, { // Test 1: One worker runs everything in turn.
		N: 4, Workers: 1, WantCalls: []int{1, 1, 1, 1},
	}, { // Test 2: Two workers share five jobs.
		N: 5, Workers: 2, WantCalls: []int{1, 1, 1, 1, 1},
	}, { // Test 3: More workers than jobs.
		N: 3, Workers: 8, WantCalls: []int{1, 1, 1},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			var mu sync.Mutex
			calls := make([]int, test.N)
			running, peak := 0, 0
			each(context.Background(), test.N, test.Workers, func(i int) {
				mu.Lock()
				calls[i]++
				running++
				peak = max(peak, running)
				mu.Unlock()
				runtime.Gosched()
				mu.Lock()
				running--
				mu.Unlock()
			})
			if diff := cmp.Diff(test.WantCalls, calls); diff != "" {
				t.Errorf("calls mismatch (-want +got):\n%s", diff)
			}
			if peak > test.Workers {
				t.Errorf("peak of %d at once, want at most %d", peak, test.Workers)
			}
		})
	}
}
