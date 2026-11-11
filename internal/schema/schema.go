// Package schema builds JSON schemas for Claude structured outputs from Go types.
package schema

import (
	"bytes"
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/dcadolph/ferry/internal/jsonutil"
)

// member is one key of a schema object with its value.
type member struct {
	// Key is the JSON key.
	Key string
	// Value is the JSON value: a string, a bool, a list of strings, or a node.
	Value any
}

// node is one JSON object of a schema. Its members are written in the order held, which a Go
// map cannot promise. The order matters because Claude writes the fields of a reply in the
// order the schema lists its properties.
type node []member

// MarshalJSON writes n as a JSON object with its members in order.
func (n node) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, m := range n {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := jsonutil.Marshal(m.Key, false)
		if err != nil {
			return nil, err
		}
		value, err := jsonutil.Marshal(m.Value, false)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(value)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// For returns the JSON schema for values shaped like v, limited to what Claude structured
// outputs accept. A struct becomes an object that lists and requires every field, in field
// order, and allows no other properties. Claude writes a reply's fields in that order, so a
// field that others depend on belongs above them in the struct. Field names come from json
// tags, and a desc tag becomes the field's description. Pointers are described by the value
// they point at. For panics on a type it cannot describe, such as a map, an interface, a
// channel, or a recursive struct, because passing one is a developer error.
func For(v any) json.RawMessage {
	if v == nil {
		panic("schema.For: nil value")
	}
	b, err := jsonutil.Marshal(typeSchema(reflect.TypeOf(v), "", map[reflect.Type]bool{}), false)
	if err != nil {
		panic(fmt.Sprintf("schema.For: %v", err))
	}
	return b
}

// typeSchema returns the schema for t, with desc as its description when desc is set. Open
// holds the struct types being described further up the current path, so a recursive type
// panics instead of looping forever.
func typeSchema(t reflect.Type, desc string, open map[reflect.Type]bool) node {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if decodesItself(t) {
		panic(fmt.Sprintf("schema.For: %s decodes itself, so its JSON shape is unknown", t))
	}
	switch t.Kind() {
	case reflect.String:
		return described("string", desc)
	case reflect.Bool:
		return described("boolean", desc)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return described("integer", desc)
	case reflect.Float32, reflect.Float64:
		return described("number", desc)
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			panic(fmt.Sprintf("schema.For: %s encodes as base64 text, not an array", t))
		}
		return append(described("array", desc), member{"items", typeSchema(t.Elem(), "", open)})
	case reflect.Array:
		return append(described("array", desc), member{"items", typeSchema(t.Elem(), "", open)})
	case reflect.Struct:
		return structSchema(t, desc, open)
	default:
		panic(fmt.Sprintf("schema.For: unsupported kind %s in %s", t.Kind(), t))
	}
}

// described returns the opening members of a schema: its type and, when desc is set, its
// description.
func described(typeName, desc string) node {
	n := node{{"type", typeName}}
	if desc != "" {
		n = append(n, member{"description", desc})
	}
	return n
}

// structSchema returns the object schema for the struct type t, with nested structs inline and
// the properties in field order.
func structSchema(t reflect.Type, desc string, open map[reflect.Type]bool) node {
	if open[t] {
		panic(fmt.Sprintf("schema.For: %s is recursive, which structured outputs reject", t))
	}
	open[t] = true
	defer delete(open, t)

	props := make(node, 0, t.NumField())
	required := make([]string, 0, t.NumField())
	seen := make(map[string]bool, t.NumField())
	for i := range t.NumField() {
		f := t.Field(i)
		name, ok := fieldName(f)
		if !ok {
			continue
		}
		if seen[name] {
			panic(fmt.Sprintf("schema.For: %s has two fields named %q", t, name))
		}
		seen[name] = true
		props = append(props, member{name, typeSchema(f.Type, f.Tag.Get("desc"), open)})
		required = append(required, name)
	}
	return append(described("object", desc),
		member{"properties", props},
		member{"required", required},
		member{"additionalProperties", false},
	)
}

// fieldName returns the JSON name of f, or false when encoding/json leaves f out. It panics on
// an embedded field without a json name, since encoding/json would flatten its fields.
func fieldName(f reflect.StructField) (string, bool) {
	tag := f.Tag.Get("json")
	if tag == "-" {
		return "", false
	}
	name, _, _ := strings.Cut(tag, ",")
	if f.Anonymous && name == "" {
		panic(fmt.Sprintf("schema.For: embedded field %s needs a json name", f.Name))
	}
	if !f.IsExported() && !f.Anonymous {
		return "", false
	}
	if name == "" {
		name = f.Name
	}
	return name, true
}

// decodesItself reports whether values of t take their JSON shape from their own decoding
// method, which For cannot see.
func decodesItself(t reflect.Type) bool {
	p := reflect.PointerTo(t)
	return p.Implements(reflect.TypeFor[json.Unmarshaler]()) ||
		p.Implements(reflect.TypeFor[encoding.TextUnmarshaler]())
}
