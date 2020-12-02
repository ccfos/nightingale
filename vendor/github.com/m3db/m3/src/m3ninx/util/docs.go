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

package util

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/m3db/m3/src/m3ninx/doc"
)

// ReadDocs reads up to n documents from a JSON formatted file at the provided path.
// It is useful for getting a set of documents to run tests with.
func ReadDocs(path string, n int) ([]doc.Document, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var (
		docs    []doc.Document
		scanner = bufio.NewScanner(f)
	)
	for scanner.Scan() && len(docs) < n {
		var fieldsMap map[string]string
		if err := json.Unmarshal(scanner.Bytes(), &fieldsMap); err != nil {
			return nil, err
		}

		// Generate a new random UUID for the document.
		id, err := NewUUID()
		if err != nil {
			return nil, err
		}

		fields := make([]doc.Field, 0, len(fieldsMap))
		for k, v := range fieldsMap {
			if len(k) == 0 || len(v) == 0 {
				continue
			}
			fields = append(fields, doc.Field{
				Name:  []byte(k),
				Value: []byte(v),
			})
		}
		docs = append(docs, doc.Document{
			ID:     id,
			Fields: fields,
		})
	}

	if len(docs) != n {
		return nil, fmt.Errorf("requested %d metrics but found %d", n, len(docs))
	}

	return docs, nil
}

// MustReadDocs calls ReadDocs and panics if there is an error.
func MustReadDocs(path string, n int) []doc.Document {
	docs, err := ReadDocs(path, n)
	if err != nil {
		panic(err)
	}
	return docs
}
