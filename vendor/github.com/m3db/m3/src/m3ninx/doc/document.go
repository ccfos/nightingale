// Copyright (c) 2018 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package doc

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"unicode/utf8"
)

var (
	errReservedFieldName = fmt.Errorf("'%s' is a reserved field name", IDReservedFieldName)
	// ErrEmptyDocument is an error for an empty document.
	ErrEmptyDocument = errors.New("document cannot be empty")
)

// IDReservedFieldName is the field name reserved for IDs.
var IDReservedFieldName = []byte("_m3ninx_id")

// Field represents a field in a document. It is composed of a name and a value.
type Field struct {
	Name  []byte
	Value []byte
}

// Fields is a list of fields.
type Fields []Field

func (f Fields) Len() int {
	return len(f)
}

func (f Fields) Less(i, j int) bool {
	l, r := f[i], f[j]

	c := bytes.Compare(l.Name, r.Name)
	switch {
	case c < 0:
		return true
	case c > 0:
		return false
	}

	c = bytes.Compare(l.Value, r.Value)
	switch {
	case c < 0:
		return true
	case c > 0:
		return false
	}

	return true
}

func (f Fields) Swap(i, j int) {
	f[i], f[j] = f[j], f[i]
}

func (f Fields) shallowCopy() Fields {
	cp := make([]Field, 0, len(f))
	for _, fld := range f {
		cp = append(cp, Field{
			Name:  fld.Name,
			Value: fld.Value,
		})
	}
	return cp
}

// Document represents a document to be indexed.
type Document struct {
	ID     []byte
	Fields []Field
}

// Get returns the value of the specified field name in the document if it exists.
func (d Document) Get(fieldName []byte) ([]byte, bool) {
	for _, f := range d.Fields {
		if bytes.Equal(fieldName, f.Name) {
			return f.Value, true
		}
	}
	return nil, false
}

// Compare returns an integer comparing two documents. The result will be 0 if the documents
// are equal, -1 if d is ordered before other, and 1 if d is ordered aftered other.
func (d Document) Compare(other Document) int {
	if c := bytes.Compare(d.ID, other.ID); c != 0 {
		return c
	}

	l, r := Fields(d.Fields), Fields(other.Fields)

	// Make a shallow copy of the Fields so we don't mutate the document.
	if !sort.IsSorted(l) {
		l = l.shallowCopy()
		sort.Sort(l)
	}
	if !sort.IsSorted(r) {
		r = r.shallowCopy()
		sort.Sort(r)
	}

	min := len(l)
	if len(r) < min {
		min = len(r)
	}

	for i := 0; i < min; i++ {
		if c := bytes.Compare(l[i].Name, r[i].Name); c != 0 {
			return c
		}
		if c := bytes.Compare(l[i].Value, r[i].Value); c != 0 {
			return c
		}
	}

	if len(l) < len(r) {
		return -1
	} else if len(l) > len(r) {
		return 1
	}

	return 0
}

// Equal returns a bool indicating whether d is equal to other.
func (d Document) Equal(other Document) bool {
	return d.Compare(other) == 0
}

// Validate returns a bool indicating whether the document is valid.
func (d Document) Validate() error {
	if len(d.Fields) == 0 && !d.HasID() {
		return ErrEmptyDocument
	}

	if !utf8.Valid(d.ID) {
		return fmt.Errorf("document has invalid ID: id=%v, id_hex=%x", d.ID, d.ID)
	}

	for _, f := range d.Fields {
		// TODO: Should we enforce uniqueness of field names?
		if !utf8.Valid(f.Name) {
			return fmt.Errorf("document has invalid field name: name=%v, name_hex=%x",
				f.Name, f.Name)
		}

		if bytes.Equal(f.Name, IDReservedFieldName) {
			return errReservedFieldName
		}

		if !utf8.Valid(f.Value) {
			return fmt.Errorf("document has invalid field value: value=%v, value_hex=%x",
				f.Value, f.Value)
		}
	}

	return nil
}

// HasID returns a bool indicating whether the document has an ID or not.
func (d Document) HasID() bool {
	return len(d.ID) > 0
}

func (d Document) String() string {
	var buf bytes.Buffer
	for i, f := range d.Fields {
		buf.WriteString(fmt.Sprintf("%s: %s", f.Name, f.Value))
		if i != len(d.Fields)-1 {
			buf.WriteString(", ")
		}
	}
	return fmt.Sprintf("{id: %s, fields: {%s}}", d.ID, buf.String())
}

// Documents is a list of documents.
type Documents []Document

func (ds Documents) Len() int {
	return len(ds)
}

func (ds Documents) Less(i, j int) bool {
	l, r := ds[i], ds[j]

	return l.Compare(r) < 1
}

func (ds Documents) Swap(i, j int) {
	ds[i], ds[j] = ds[j], ds[i]
}
