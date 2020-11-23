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

package block

import "github.com/m3db/m3/src/query/cost"

// AccountedBlock is a wrapper for a block which enforces limits on the number
// of datapoints used by the block.
type AccountedBlock struct {
	Block

	enforcer cost.ChainedEnforcer
}

// NewAccountedBlock wraps the given block and enforces datapoint limits.
func NewAccountedBlock(
	wrapped Block,
	enforcer cost.ChainedEnforcer,
) *AccountedBlock {
	return &AccountedBlock{
		Block:    wrapped,
		enforcer: enforcer,
	}
}

// Close closes the block, and marks the number of datapoints used
// by this block as finished.
func (ab *AccountedBlock) Close() error {
	ab.enforcer.Close()
	return ab.Block.Close()
}
