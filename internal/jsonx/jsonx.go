// Package jsonx reads and edits JSON that cadre does not own, such as
// Claude Code's ~/.claude.json and ~/.claude/settings.json, without
// changing anything it does not mean to change.
//
// encoding/json would not do: maps lose key order, a lone surrogate
// ("\ud83d", which JavaScript writes for a cut-off emoji) decodes to U+FFFD
// and would be written back changed, and <, > and & get escaped. Here a
// document is an ordered tree whose strings, numbers and literals keep the
// exact bytes they were read from. Only what an edit touches is new.
package jsonx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Kind is what a Value holds.
type Kind int

const (
	Scalar Kind = iota // a string, number, true, false or null, kept as written
	Object
	Array
)

// Value is a node of a document.
type Value struct {
	Kind    Kind
	Raw     []byte    // Scalar: the bytes as written
	Members []*Member // Object, in order
	Items   []*Value  // Array, in order
}

// Member is one key of an object.
type Member struct {
	Key    string // decoded, for lookups
	RawKey []byte // as written, quotes included
	Value  *Value
}

// ErrSyntax is returned for text that is not one valid JSON value.
var ErrSyntax = errors.New("not valid JSON")

// Parse reads one JSON value, with surrounding whitespace.
func Parse(data []byte) (*Value, error) {
	if !json.Valid(data) {
		return nil, ErrSyntax
	}
	p := &parser{data: data}
	v := p.value()
	return v, nil
}

type parser struct {
	data []byte
	i    int
}

func (p *parser) space() {
	for p.i < len(p.data) {
		switch p.data[p.i] {
		case ' ', '\t', '\n', '\r':
			p.i++
		default:
			return
		}
	}
}

// value reads one value. The input is known to be valid JSON.
func (p *parser) value() *Value {
	p.space()
	switch p.data[p.i] {
	case '{':
		p.i++
		v := &Value{Kind: Object, Members: []*Member{}}
		for {
			p.space()
			if p.data[p.i] == '}' {
				p.i++
				return v
			}
			if p.data[p.i] == ',' {
				p.i++
				continue
			}
			raw := p.str()
			var key string
			json.Unmarshal(raw, &key)
			p.space()
			p.i++ // the colon
			v.Members = append(v.Members, &Member{Key: key, RawKey: raw, Value: p.value()})
		}
	case '[':
		p.i++
		v := &Value{Kind: Array, Items: []*Value{}}
		for {
			p.space()
			if p.data[p.i] == ']' {
				p.i++
				return v
			}
			if p.data[p.i] == ',' {
				p.i++
				continue
			}
			v.Items = append(v.Items, p.value())
		}
	case '"':
		return &Value{Kind: Scalar, Raw: p.str()}
	default:
		start := p.i
		for p.i < len(p.data) && !bytes.ContainsRune([]byte(" \t\n\r,]}"), rune(p.data[p.i])) {
			p.i++
		}
		return &Value{Kind: Scalar, Raw: p.data[start:p.i]}
	}
}

// str reads a string and returns it as written, quotes included.
func (p *parser) str() []byte {
	start := p.i
	p.i++
	for p.data[p.i] != '"' {
		if p.data[p.i] == '\\' {
			p.i++
		}
		p.i++
	}
	p.i++
	return p.data[start:p.i]
}

// String makes a string value.
func String(s string) *Value {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.Encode(s)
	return &Value{Kind: Scalar, Raw: bytes.TrimRight(b.Bytes(), "\n")}
}

// Literal makes a number, true, false or null from its JSON text.
func Literal(text string) *Value { return &Value{Kind: Scalar, Raw: []byte(text)} }

// NewObject makes an object from key and value pairs, in order.
func NewObject(pairs ...any) *Value {
	v := &Value{Kind: Object, Members: []*Member{}}
	for i := 0; i+1 < len(pairs); i += 2 {
		v.Set(pairs[i].(string), pairs[i+1].(*Value))
	}
	return v
}

// IsTrue reports whether v is the literal true.
func (v *Value) IsTrue() bool { return v != nil && v.Kind == Scalar && string(v.Raw) == "true" }

// Text returns a string value decoded, and false for anything else.
func (v *Value) Text() (string, bool) {
	if v == nil || v.Kind != Scalar || len(v.Raw) == 0 || v.Raw[0] != '"' {
		return "", false
	}
	var s string
	if json.Unmarshal(v.Raw, &s) != nil {
		return "", false
	}
	return s, true
}

// Get returns the value of key in an object, the last one when the key is
// repeated (as JavaScript reads it), or nil.
func (v *Value) Get(key string) *Value {
	if v == nil || v.Kind != Object {
		return nil
	}
	for i := len(v.Members) - 1; i >= 0; i-- {
		if v.Members[i].Key == key {
			return v.Members[i].Value
		}
	}
	return nil
}

// Count returns how many times key appears in an object.
func (v *Value) Count(key string) int {
	n := 0
	for _, m := range v.Members {
		if m.Key == key {
			n++
		}
	}
	return n
}

// Set replaces the value of key (its last occurrence) or appends it.
func (v *Value) Set(key string, val *Value) {
	for i := len(v.Members) - 1; i >= 0; i-- {
		if v.Members[i].Key == key {
			v.Members[i].Value = val
			return
		}
	}
	raw := String(key).Raw
	v.Members = append(v.Members, &Member{Key: key, RawKey: raw, Value: val})
}

// Delete removes every occurrence of key and reports whether there was one.
func (v *Value) Delete(key string) bool {
	kept := v.Members[:0]
	found := false
	for _, m := range v.Members {
		if m.Key == key {
			found = true
			continue
		}
		kept = append(kept, m)
	}
	v.Members = kept
	return found
}

// Duplicate returns the first key that appears twice in one object
// anywhere in v, and false when there is none.
func (v *Value) Duplicate() (string, bool) {
	switch v.Kind {
	case Object:
		seen := map[string]bool{}
		for _, m := range v.Members {
			if seen[m.Key] {
				return m.Key, true
			}
			seen[m.Key] = true
			if k, ok := m.Value.Duplicate(); ok {
				return k, true
			}
		}
	case Array:
		for _, it := range v.Items {
			if k, ok := it.Duplicate(); ok {
				return k, true
			}
		}
	}
	return "", false
}

// Indent returns the indentation unit a document was written with: the
// whitespace before the first line that starts inside its outer object or
// array, or "" for a document on one line.
func Indent(data []byte) string {
	i := bytes.IndexAny(data, "{[")
	if i < 0 {
		return ""
	}
	rest := data[i+1:]
	nl := bytes.IndexByte(rest, '\n')
	if nl < 0 {
		return ""
	}
	line := rest[nl+1:]
	n := 0
	for n < len(line) && (line[n] == ' ' || line[n] == '\t') {
		n++
	}
	return string(line[:n])
}

// Format writes v with indent per level (as JSON.stringify(v, null,
// indent) does), or on one line when indent is "". Scalars and keys are
// written exactly as they were read. The result ends with a newline when
// indent is not "".
func Format(v *Value, indent string) []byte {
	var b bytes.Buffer
	write(&b, v, indent, 0)
	if indent != "" {
		b.WriteByte('\n')
	}
	return b.Bytes()
}

func write(b *bytes.Buffer, v *Value, indent string, depth int) {
	nl := func(d int) {
		if indent != "" {
			b.WriteByte('\n')
			b.WriteString(strings.Repeat(indent, d))
		}
	}
	switch v.Kind {
	case Scalar:
		b.Write(v.Raw)
	case Object:
		if len(v.Members) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteByte('{')
		for i, m := range v.Members {
			if i > 0 {
				b.WriteByte(',')
			}
			nl(depth + 1)
			b.Write(m.RawKey)
			b.WriteByte(':')
			if indent != "" {
				b.WriteByte(' ')
			}
			write(b, m.Value, indent, depth+1)
		}
		nl(depth)
		b.WriteByte('}')
	case Array:
		if len(v.Items) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteByte('[')
		for i, it := range v.Items {
			if i > 0 {
				b.WriteByte(',')
			}
			nl(depth + 1)
			write(b, it, indent, depth+1)
		}
		nl(depth)
		b.WriteByte(']')
	}
}

// ShapeError says a document is valid JSON but not shaped as expected.
type ShapeError struct{ What string }

func (e *ShapeError) Error() string { return fmt.Sprintf("unexpected shape: %s", e.What) }
