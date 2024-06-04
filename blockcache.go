package squashfs

import (
	"bytes"
	"io"
	"sync"
)

var cbPool = sync.Pool{
	New: func() any {
		return &cachedBlock{}
	},
}

type cachedBlock struct {
	ptr  int64
	data []byte
	next *cachedBlock
}

type blockCache struct {
	mu             sync.Mutex
	head, tail     *cachedBlock
	bytesRemaining int
}

func newBlockCache(length int) blockCache {
	return blockCache{
		bytesRemaining: length,
	}
}

func (b *blockCache) getBlock(ptr int64, r io.ReadSeeker, c Compressor) (*bytes.Reader, error) {
	data, err := b.getOrSetBlock(ptr, r, c)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(data), nil
}

func (b *blockCache) getOrSetBlock(ptr int64, r io.ReadSeeker, c Compressor) ([]byte, error) {
	b.mu.Lock()
	cb := b.getExistingBlock(ptr)
	b.mu.Unlock()

	if cb != nil {
		return cb, nil
	}

	data, err := decompressBlock(r, c)
	if err != nil {
		return nil, err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if cb = b.getExistingBlock(ptr); cb != nil {
		return cb, nil
	}

	b.clearSpace(len(data))
	b.addData(ptr, data)

	return data, nil
}

func (b *blockCache) getExistingBlock(ptr int64) []byte {
	for node := &b.head; *node != nil; {
		curr := *node

		if curr.ptr != ptr {
			node = &curr.next

			continue
		}

		if curr != b.tail {
			*node = curr.next
			b.tail.next = curr
			b.tail = curr
			curr.next = nil
		}

		return curr.data
	}

	return nil
}

func (b *blockCache) clearSpace(l int) {
	for node := b.head; node != nil && b.bytesRemaining < l; node = node.next {
		b.bytesRemaining += len(node.data)

		b.head = node.next

		node.data = nil
		node.next = nil

		cbPool.Put(node)
	}
}

func (b *blockCache) addData(ptr int64, data []byte) {
	if b.bytesRemaining < len(data) {
		return
	}

	node := cbPool.Get().(*cachedBlock)
	node.ptr = ptr
	node.data = data

	if b.head == nil {
		b.head = node
	} else {
		b.tail.next = node
	}

	b.tail = node
	b.bytesRemaining -= len(data)
}

func decompressBlock(r io.Reader, c Compressor) ([]byte, error) {
	if c != 0 {
		cr, err := c.decompress(r)
		if err != nil {
			return nil, err
		}

		r = cr
	}

	return io.ReadAll(r)
}
