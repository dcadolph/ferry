package jsonutil

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestMarshal checks compact and indented encoding, unescaped prose, and the error path.
func TestMarshal(t *testing.T) {
	t.Parallel()
	// payload is a small value to encode.
	type payload struct {
		// Name is a text field.
		Name string `json:"name"`
		// Count is a number field.
		Count int `json:"count"`
	}
	tests := []struct {
		In         any
		WantResult string
		Want       error
		Pretty     bool
	}{{ // Test 0: Compact encoding has no whitespace and no trailing newline.
		In: payload{Name: "a", Count: 1}, Pretty: false,
		WantResult: `{"name":"a","count":1}`, Want: nil,
	}, { // Test 1: Pretty encoding indents with two spaces.
		In: payload{Name: "a", Count: 1}, Pretty: true,
		WantResult: "{\n  \"name\": \"a\",\n  \"count\": 1\n}", Want: nil,
	}, { // Test 2: Ampersands and angle brackets stay as written.
		In: payload{Name: "R&D <ops>", Count: 2}, Pretty: false,
		WantResult: `{"name":"R&D <ops>","count":2}`, Want: nil,
	}, { // Test 3: A nil slice encodes as null.
		In: []string(nil), Pretty: false,
		WantResult: "null", Want: nil,
	}, { // Test 4: A value JSON cannot hold is an error.
		In: make(chan int), Pretty: false,
		WantResult: "", Want: ErrMarshal,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got, err := Marshal(test.In, test.Pretty)
			if !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			if diff := cmp.Diff(test.WantResult, string(got)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
