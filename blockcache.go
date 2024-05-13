package squashfs

import (
	"bytes"
	"io"
	"sync"
)

type cachedBlock struct {
	ptr  int64
	data []byte
}

type blockCache struct {
	mu    sync.RWMutex
	cache []cachedBlock
	pos   int
}

func newBlockCache(length uint) blockCache {
	return blockCache{
		cache: make([]cachedBlock, 0, length),
	}
}

func (b *blockCache) getBlock(ptr int64, r io.ReadSeeker, c Compressor) (io.ReadSeeker, error) {
	data, err := b.getOrSetBlock(ptr, r, c)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(data), nil
}

func (b *blockCache) getOrSetBlock(ptr int64, r io.ReadSeeker, c Compressor) ([]byte, error) {
	b.mu.RLock()
	cb := b.getExistingBlock(ptr)
	b.mu.RUnlock()

	if cb != nil {
		return cb, nil
	}

	cr, err := c.decompress(r)
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(cr)
	if err != nil {
		return nil, err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if cb = b.getExistingBlock(ptr); cb != nil {
		return cb, nil
	}

	block := cachedBlock{
		ptr:  ptr,
		data: data,
	}

	if len(b.cache) == cap(b.cache) {
		b.cache[b.pos] = block

		b.pos++
		if b.pos == len(b.cache) {
			b.pos = 0
		}
	} else {
		b.cache = append(b.cache, block)
	}

	return data, nil
}

func (b *blockCache) getExistingBlock(ptr int64) []byte {
	for _, cb := range b.cache {
		if cb.ptr == ptr {
			return cb.data
		}
	}

	return nil
}
