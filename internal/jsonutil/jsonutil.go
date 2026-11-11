// Package jsonutil encodes command output as JSON, compact by default and indented on request.
package jsonutil

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Marshal returns v as JSON with no trailing newline. The output is compact unless pretty is
// true, which indents it with two spaces. Characters such as & and < stay as written, so prose
// in the output reads the way the model wrote it.
func Marshal(v any, pretty bool) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if pretty {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMarshal, err)
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}
