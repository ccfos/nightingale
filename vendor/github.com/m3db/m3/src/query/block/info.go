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

package block

// String returns the block type as a string.
func (t BlockType) String() string {
	switch t {
	case BlockM3TSZCompressed:
		return "compressed_m3tsz"
	case BlockDecompressed:
		return "decompressed"
	case BlockScalar:
		return "scalar"
	case BlockLazy:
		return "lazy"
	case BlockTime:
		return "time"
	case BlockContainer:
		return "container"
	case BlockEmpty:
		return "empty"
	case BlockTest:
		return "test"
	}

	return "unknown"
}

// BlockInfo describes information about the block.
type BlockInfo struct {
	blockType BlockType
	inner     []BlockType
}

// NewBlockInfo creates a BlockInfo of the specified type.
func NewBlockInfo(blockType BlockType) BlockInfo {
	return BlockInfo{blockType: blockType}
}

// NewWrappedBlockInfo creates a BlockInfo of the specified type, wrapping an
// existing BlockInfo.
func NewWrappedBlockInfo(
	blockType BlockType,
	wrap BlockInfo,
) BlockInfo {
	inner := make([]BlockType, 0, len(wrap.inner)+1)
	inner = append(inner, wrap.blockType)
	inner = append(inner, wrap.inner...)
	return BlockInfo{
		blockType: blockType,
		inner:     inner,
	}
}

// Type is the block type for this block.
func (b BlockInfo) Type() BlockType {
	return b.blockType
}

// InnerType is the block type for any block wrapped by this block, or this
// block itself if it doesn't wrap anything.
func (b BlockInfo) InnerType() BlockType {
	if b.inner == nil {
		return b.Type()
	}

	return b.inner[0]
}

// BaseType is the block type for the innermost block wrapped by this block, or
// the block itself if it doesn't wrap anything.
func (b BlockInfo) BaseType() BlockType {
	if b.inner == nil {
		return b.Type()
	}

	return b.inner[len(b.inner)-1]
}
