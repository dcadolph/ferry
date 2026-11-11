package schema

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// testWord is a named string type.
type testWord string

// testInner is a small struct nested inside testOuter.
type testInner struct {
	// Label is a plain string field with a description.
	Label string `json:"label" desc:"A short label."`
}

// testOuter covers every supported kind and each field naming rule.
type testOuter struct {
	// Name has a json name and a description.
	Name string `json:"name" desc:"The name."`
	// On is a boolean.
	On bool `json:"on"`
	// Count is a signed integer.
	Count int64 `json:"count"`
	// Small is an unsigned integer.
	Small uint8 `json:"small"`
	// Ratio is a floating-point number.
	Ratio float32 `json:"ratio"`
	// Word is a named string type.
	Word testWord `json:"word"`
	// Tags is a slice of strings with a description.
	Tags []string `json:"tags" desc:"Some tags."`
	// Inner is a nested struct with a description.
	Inner testInner `json:"inner" desc:"The inner part."`
	// Items is a slice of structs.
	Items []testInner `json:"items"`
	// Pair is a fixed-size array.
	Pair [2]int `json:"pair"`
	// Ptr is described as the value it points at.
	Ptr *string `json:"ptr"`
	// Bare has no json tag, so it keeps its Go name.
	Bare string
	// Opts has json options after its name.
	Opts string `json:"opts,omitempty"`
	// Skipped is left out by its json tag.
	Skipped string `json:"-"`
	// hidden is unexported, so encoding/json leaves it out.
	hidden string
}

// testLoop refers to itself through a pointer.
type testLoop struct {
	// Next points at another testLoop.
	Next *testLoop `json:"next"`
}

// testEmbed embeds a struct without giving it a json name.
type testEmbed struct {
	// testInner is embedded, so encoding/json would flatten its fields.
	testInner
}

// testDup ends up with two fields that share one JSON name.
type testDup struct {
	// A keeps its Go name, A.
	A string
	// B takes the json name A.
	B string `json:"A"`
}

// testAny holds an interface field.
type testAny struct {
	// Value has no fixed JSON shape.
	Value any `json:"value"`
}

// testMarked holds a field whose description uses characters JSON encoders often escape.
type testMarked struct {
	// Note has a description with angle brackets, an ampersand, and quotes.
	Note string `json:"note" desc:"Use <b> & \"quotes\"."`
}

// testMap holds a map field.
type testMap struct {
	// Values has open-ended keys.
	Values map[string]string `json:"values"`
}

// testInnerSchema returns the schema For gives testInner, with desc as its description when set.
func testInnerSchema(desc string) string {
	if desc != "" {
		desc = `,"description":"` + desc + `"`
	}
	return `{"type":"object"` + desc + `,"properties":{"label":{"type":"string",` +
		`"description":"A short label."}},"required":["label"],"additionalProperties":false}`
}

// TestFor checks the schema For builds for each supported kind and field rule, byte for byte,
// so the order of the keys is part of what is checked.
func TestFor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         any
		WantResult string
	}{{ // Test 0: A string.
		In: "x", WantResult: `{"type":"string"}`,
	}, { // Test 1: A boolean.
		In: true, WantResult: `{"type":"boolean"}`,
	}, { // Test 2: A signed integer.
		In: 7, WantResult: `{"type":"integer"}`,
	}, { // Test 3: An unsigned integer.
		In: uint16(7), WantResult: `{"type":"integer"}`,
	}, { // Test 4: A floating-point number.
		In: 1.5, WantResult: `{"type":"number"}`,
	}, { // Test 5: A slice of strings.
		In: []string{"a"}, WantResult: `{"type":"array","items":{"type":"string"}}`,
	}, { // Test 6: A fixed-size array.
		In: [3]bool{}, WantResult: `{"type":"array","items":{"type":"boolean"}}`,
	}, { // Test 7: A pointer is described by the value it points at.
		In: new(int), WantResult: `{"type":"integer"}`,
	}, { // Test 8: A struct becomes a closed object.
		In: testInner{}, WantResult: testInnerSchema(""),
	}, { // Test 9: A pointer to a struct matches the struct.
		In: &testInner{}, WantResult: testInnerSchema(""),
	}, { // Test 10: Every field rule at once, with the properties in field order.
		In: testOuter{hidden: "x"},
		WantResult: `{"type":"object","properties":{` +
			`"name":{"type":"string","description":"The name."},` +
			`"on":{"type":"boolean"},` +
			`"count":{"type":"integer"},` +
			`"small":{"type":"integer"},` +
			`"ratio":{"type":"number"},` +
			`"word":{"type":"string"},` +
			`"tags":{"type":"array","description":"Some tags.","items":{"type":"string"}},` +
			`"inner":` + testInnerSchema("The inner part.") + `,` +
			`"items":{"type":"array","items":` + testInnerSchema("") + `},` +
			`"pair":{"type":"array","items":{"type":"integer"}},` +
			`"ptr":{"type":"string"},` +
			`"Bare":{"type":"string"},` +
			`"opts":{"type":"string"}},` +
			`"required":["name","on","count","small","ratio","word","tags","inner","items",` +
			`"pair","ptr","Bare","opts"],"additionalProperties":false}`,
	}, { // Test 11: An empty struct keeps an empty properties object and required list, never
		// null, since the API rejects a null required list.
		In:         struct{}{},
		WantResult: `{"type":"object","properties":{},"required":[],"additionalProperties":false}`,
	}, { // Test 12: Angle brackets and ampersands in a description stay as written.
		In: testMarked{},
		WantResult: `{"type":"object","properties":{"note":{"type":"string",` +
			`"description":"Use <b> & \"quotes\"."}},"required":["note"],` +
			`"additionalProperties":false}`,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := For(test.In)
			if diff := cmp.Diff(test.WantResult, string(got)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
			if !json.Valid(got) {
				t.Errorf("For(%T) is not valid JSON: %s", test.In, got)
			}
		})
	}
}

// testFirst and testSecond hold the same fields in opposite orders.
type (
	// testFirst declares B before A.
	testFirst struct {
		// B is declared first.
		B string `json:"b"`
		// A is declared second.
		A string `json:"a"`
	}
	// testSecond declares A before B.
	testSecond struct {
		// A is declared first.
		A string `json:"a"`
		// B is declared second.
		B string `json:"b"`
	}
)

// TestForOrder checks that properties follow the order of the struct's fields and not the
// order of their names, since the model writes a reply's fields in schema order.
func TestForOrder(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In        any
		WantOrder []string
	}{{ // Test 0: A field declared first comes first, though its name sorts last.
		In: testFirst{}, WantOrder: []string{`"b":`, `"a":`},
	}, { // Test 1: The same fields declared the other way around.
		In: testSecond{}, WantOrder: []string{`"a":`, `"b":`},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := string(For(test.In))
			last := -1
			for _, key := range test.WantOrder {
				at := strings.Index(got, key)
				if at <= last {
					t.Errorf("%s is at %d, want it after %d in %s", key, at, last, got)
				}
				last = at
			}
		})
	}
}

// TestForPanics checks that For panics on types it cannot describe.
func TestForPanics(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In          any
		WantMessage string
	}{{ // Test 0: A nil value.
		In: nil, WantMessage: "nil value",
	}, { // Test 1: A map.
		In: map[string]string{}, WantMessage: "unsupported kind map",
	}, { // Test 2: A map inside a struct.
		In: testMap{}, WantMessage: "unsupported kind map",
	}, { // Test 3: An interface inside a struct.
		In: testAny{}, WantMessage: "unsupported kind interface",
	}, { // Test 4: A channel.
		In: make(chan int), WantMessage: "unsupported kind chan",
	}, { // Test 5: A function.
		In: func() {}, WantMessage: "unsupported kind func",
	}, { // Test 6: A complex number.
		In: complex(1, 2), WantMessage: "unsupported kind complex128",
	}, { // Test 7: A byte slice, which encodes as base64 text.
		In: []byte("x"), WantMessage: "base64",
	}, { // Test 8: A recursive struct.
		In: testLoop{}, WantMessage: "recursive",
	}, { // Test 9: An embedded struct without a json name.
		In: testEmbed{}, WantMessage: "embedded field testInner",
	}, { // Test 10: Two fields with one JSON name.
		In: testDup{}, WantMessage: `two fields named "A"`,
	}, { // Test 11: A type with its own JSON decoding.
		In: time.Time{}, WantMessage: "decodes itself",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("For(%T) did not panic", test.In)
				}
				if msg := fmt.Sprint(r); !strings.Contains(msg, test.WantMessage) {
					t.Errorf("panic %q does not mention %q", msg, test.WantMessage)
				}
			}()
			For(test.In)
		})
	}
}
