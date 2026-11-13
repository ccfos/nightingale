package doris

import (
	"context"
	"encoding/json"
	"io"
	"sort"
	"unicode/utf8"
)

const jsonRowChunkSize = 32 << 10

type jsonRowField struct {
	text     string
	raw      []byte
	isString bool
}

// JSONRow validates values before output starts and streams driver string values
// without allocating an encoded copy of the complete row. Do not mutate a row
// or its values until Write returns.
type JSONRow struct {
	fields           map[string]jsonRowField
	keys             []string
	normalizedFields map[string]jsonRowField
}

func PrepareJSONRow(row map[string]any) (*JSONRow, error) {
	result := &JSONRow{}
	if row == nil {
		return result, nil
	}
	result.fields = make(map[string]jsonRowField, len(row))
	for key := range row {
		result.keys = append(result.keys, key)
	}
	sort.Strings(result.keys)
	for _, key := range result.keys {
		value := row[key]
		field := jsonRowField{}
		if text, ok := value.(string); ok {
			field.text, field.isString = text, true
		} else {
			encoded, err := json.Marshal(value)
			if err != nil {
				return nil, err
			}
			field.raw = encoded
		}
		result.fields[key] = field
		if !utf8.ValidString(key) && result.normalizedFields == nil {
			result.normalizedFields = make(map[string]jsonRowField, len(row))
		}
	}
	if result.normalizedFields != nil {
		for _, key := range result.keys {
			normalized := key
			if !utf8.ValidString(key) {
				raw, _ := json.Marshal(key)
				_ = json.Unmarshal(raw, &normalized)
			}
			result.normalizedFields[normalized] = result.fields[key]
		}
	}
	return result, nil
}

// Value returns a string or an already validated JSON value for field selection.
func (r *JSONRow) Value(name string) (any, bool) {
	fields := r.fields
	if r.normalizedFields != nil {
		fields = r.normalizedFields
	}
	field, ok := fields[name]
	if !ok {
		return nil, false
	}
	if field.isString {
		return field.text, true
	}
	return json.RawMessage(field.raw), true
}

func (r *JSONRow) Write(ctx context.Context, target io.Writer) error {
	w := jsonRowWriter{ctx: ctx, target: target}
	if r.fields == nil {
		w.appendString("null")
		return w.finish()
	}
	w.appendString("{")
	for i, key := range r.keys {
		if i > 0 {
			w.appendString(",")
		}
		w.string(key)
		w.appendString(":")
		field := r.fields[key]
		if field.isString {
			w.string(field.text)
		} else {
			w.appendBytes(field.raw)
		}
		if w.err != nil {
			return w.err
		}
	}
	w.appendString("}")
	return w.finish()
}

type jsonRowWriter struct {
	ctx    context.Context
	target io.Writer
	err    error
}

func (w *jsonRowWriter) finish() error {
	if w.err != nil {
		return w.err
	}
	return w.ctx.Err()
}
func (w *jsonRowWriter) appendString(s string) {
	for len(s) > 0 && w.err == nil {
		if w.err = w.ctx.Err(); w.err != nil {
			return
		}
		size := min(len(s), jsonRowChunkSize)
		n, err := io.WriteString(w.target, s[:size])
		if err == nil && n != size {
			err = io.ErrShortWrite
		}
		w.err = err
		s = s[size:]
	}
}
func (w *jsonRowWriter) appendBytes(b []byte) {
	for len(b) > 0 && w.err == nil {
		if w.err = w.ctx.Err(); w.err != nil {
			return
		}
		size := min(len(b), jsonRowChunkSize)
		n, err := w.target.Write(b[:size])
		if err == nil && n != size {
			err = io.ErrShortWrite
		}
		w.err = err
		b = b[size:]
	}
}

var jsonRowSafeASCII = func() [utf8.RuneSelf]bool {
	var safe [utf8.RuneSelf]bool
	for i := 0x20; i < len(safe); i++ {
		safe[i] = true
	}
	for _, c := range []byte{'\\', '"', '<', '>', '&'} {
		safe[c] = false
	}
	return safe
}()

// Match encoding/json's default HTML escaping and invalid UTF-8 replacement.
func (w *jsonRowWriter) string(s string) {
	const hex = "0123456789abcdef"
	w.appendString(`"`)
	start, i := 0, 0
	for i < len(s) && w.err == nil {
		end := min(i+jsonRowChunkSize, len(s))
		for i < end {
			c := s[i]
			if c < utf8.RuneSelf {
				if jsonRowSafeASCII[c] {
					i++
					continue
				}
				w.appendString(s[start:i])
				switch c {
				case '\\', '"':
					w.appendString("\\")
					w.appendString(s[i : i+1])
				case '\n':
					w.appendString(`\n`)
				case '\r':
					w.appendString(`\r`)
				case '\t':
					w.appendString(`\t`)
				case '\b':
					w.appendString(`\b`)
				case '\f':
					w.appendString(`\f`)
				default:
					w.appendString(`\u00`)
					w.appendString(string([]byte{hex[c>>4], hex[c&0xf]}))
				}
				i++
				start = i
			} else {
				r, size := utf8.DecodeRuneInString(s[i:])
				if r == utf8.RuneError && size == 1 {
					w.appendString(s[start:i])
					w.appendString(`\ufffd`)
					i++
					start = i
				} else if r == '\u2028' || r == '\u2029' {
					w.appendString(s[start:i])
					w.appendString(`\u202`)
					if r == '\u2028' {
						w.appendString("8")
					} else {
						w.appendString("9")
					}
					i += size
					start = i
				} else {
					i += size
				}
			}
			if w.err != nil {
				return
			}
		}
		w.appendString(s[start:i])
		start = i
	}
	w.appendString(`"`)
}
