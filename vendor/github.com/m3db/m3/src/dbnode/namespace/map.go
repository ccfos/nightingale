// Copyright (c) 2017 Uber Technologies, Inc.
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

package namespace

import (
	"errors"
	"fmt"

	xerrors "github.com/m3db/m3/src/x/errors"
	"github.com/m3db/m3/src/x/ident"
)

var (
	errEmptyMetadatas = errors.New("no namespace metadata provided")
)

type nsMap struct {
	namespaces *metadataMap
	ids        []ident.ID
	metadatas  []Metadata
}

// NewMap returns a new registry containing provided metadatas,
// providing a consistent order.
func NewMap(metadatas []Metadata) (Map, error) {
	if len(metadatas) == 0 {
		return nil, errEmptyMetadatas
	}

	var (
		ns          = newMetadataMap(metadataMapOptions{})
		ids         = make([]ident.ID, 0, len(metadatas))
		nsMetadatas = make([]Metadata, 0, len(metadatas))
		multiErr    xerrors.MultiError
	)
	for _, m := range metadatas {
		id := m.ID()
		ids = append(ids, id)
		nsMetadatas = append(nsMetadatas, m)

		if _, ok := ns.Get(id); ok {
			multiErr = multiErr.Add(fmt.Errorf(
				"namespace ids must be unique, duplicate found: %v", id.String()))
		}
		ns.Set(id, m)
	}

	if err := multiErr.FinalError(); err != nil {
		return nil, err
	}

	return &nsMap{
		namespaces: ns,
		ids:        ids,
		metadatas:  nsMetadatas,
	}, nil
}

func (r *nsMap) Get(namespace ident.ID) (Metadata, error) {
	metadata, ok := r.namespaces.Get(namespace)
	if !ok {
		return nil, fmt.Errorf("unable to find namespace (%v) in registry", namespace.String())
	}
	return metadata, nil
}

func (r *nsMap) IDs() []ident.ID {
	return r.ids
}

func (r *nsMap) Metadatas() []Metadata {
	return r.metadatas
}

func (r *nsMap) Equal(value Map) bool {
	// short circuit ptr equals
	if value == r {
		return true
	}

	ourMds := r.Metadatas()
	theirMds := value.Metadatas()
	if len(ourMds) != len(theirMds) {
		return false
	}

	// O(n**2) test, not a big deal because this is only 3-5 elements
	for _, om := range ourMds {
		found := false
		for _, tm := range theirMds {
			if om.Equal(tm) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}
