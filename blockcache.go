package squashfs

import (
	"bytes"
	"io"
	"slices"
	"sync"
)

type cachedBlock struct {
	ptr  int64
	data []byte
}

type blockCache struct {
	mu    sync.RWMutex
	cache []cachedBlock
}

func newBlockCache(length uint) blockCache {
	return blockCache{
		cache: make([]cachedBlock, 0, length),
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
	b.mu.RLock()
	cb := b.getExistingBlock(ptr)
	b.mu.RUnlock()

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

	block := cachedBlock{
		ptr:  ptr,
		data: data,
	}

	if len(b.cache) == cap(b.cache) {
		b.cache = b.cache[:len(b.cache)-1]
	}

	b.cache = slices.Insert(b.cache, 0, block)

	return data, nil
}

func (b *blockCache) getExistingBlock(ptr int64) []byte {
	for n, cb := range b.cache {
		if cb.ptr == ptr {
			if n != 0 {
				b.cache = slices.Insert(slices.Delete(b.cache, n, n+1), 0, cb)
			}

			return cb.data
		}
	}

	return nil
}

func decompressBlock(r io.Reader, c Compressor) ([]byte, error) {
	cr, err := c.decompress(r)
	if err != nil {
		return nil, err
	}

	return io.ReadAll(cr)
}
