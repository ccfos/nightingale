// Copyright (c) 2019 Uber Technologies, Inc.
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
	"fmt"

	xerrors "github.com/m3db/m3/src/x/errors"

	"go.uber.org/zap"
)

// UpdateSchemaRegistry updates schema registry with namespace updates.
func UpdateSchemaRegistry(newNamespaces Map, schemaReg SchemaRegistry, log *zap.Logger) error {
	schemaUpdates := newNamespaces.Metadatas()
	multiErr := xerrors.NewMultiError()
	for _, metadata := range schemaUpdates {
		var (
			curSchemaID   = "none"
			curSchemaNone = true
		)
		curSchema, err := schemaReg.GetLatestSchema(metadata.ID())
		if err != nil {
			// NB(bodu): Schema history not found is a valid error as this occurs on initial bootstrap for the db.
			if _, ok := err.(*schemaHistoryNotFoundError); !ok {
				multiErr = multiErr.Add(fmt.Errorf("cannot get latest namespace schema: %v", err))
				continue
			}
		}

		if curSchema != nil {
			curSchemaNone = false
			curSchemaID = curSchema.DeployId()
			if len(curSchemaID) == 0 {
				msg := "namespace schema update invalid with empty deploy ID"
				log.Warn(msg, zap.Stringer("namespace", metadata.ID()),
					zap.String("currentSchemaID", curSchemaID))
				multiErr = multiErr.Add(fmt.Errorf("%s: namespace=%s", msg, metadata.ID().String()))
				continue
			}
		}

		// Log schema update.
		latestSchema, found := metadata.Options().SchemaHistory().GetLatest()
		if !found {
			if !curSchemaNone {
				// NB(r): Only interpret this as a warning/error if already had a schema,
				// otherwise this is just a namespace that is not using protobuf schemas.
				msg := "namespace schema not found on update"
				log.Warn(msg, zap.Stringer("namespace", metadata.ID()),
					zap.String("currentSchema", curSchemaID))
				multiErr = multiErr.Add(fmt.Errorf("%s: namespace=%s", msg, metadata.ID().String()))
			}
			continue
		}

		log.Info("updating database namespace schema", zap.Stringer("namespace", metadata.ID()),
			zap.String("currentSchema", curSchemaID), zap.String("latestSchema", latestSchema.DeployId()))

		err = schemaReg.SetSchemaHistory(metadata.ID(), metadata.Options().SchemaHistory())
		if err != nil {
			msg := "namespace schema failed to update to latest schema"
			log.Warn(msg,
				zap.Stringer("namespace", metadata.ID()),
				zap.Error(err))
			multiErr = multiErr.Add(fmt.Errorf("%s: namespace=%s, error=%v",
				msg, metadata.ID().String(), err))
		}
	}
	if !multiErr.Empty() {
		return multiErr
	}
	return nil
}
