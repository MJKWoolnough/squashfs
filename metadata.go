package squashfs

import (
	"bytes"
	"errors"
	"io"
	"sync"

	"vimagination.zapto.org/byteio"
)

const (
	blockSize       = 1 << 13
	blockHeaderSize = 2

	metadataPointerShift = 16
	metadataPointerMask  = 0xffff

	metadataBlockSizeMask       = 0x7fff
	metadataBlockCompressedMask = 0x8000
)

func (s *squashfs) readMetadata(pointer, table uint64) (*blockReader, error) {
	onDisk := int64(table + (pointer >> metadataPointerShift))

	pos := int64(pointer & metadataPointerMask)
	if pos > blockSize {
		return nil, ErrInvalidPointer
	}

	b := &blockReader{
		squashfs: s,
		next:     onDisk,
	}

	if err := b.init(pos); err != nil {
		return nil, err
	}

	return b, nil
}

func skip(r io.Reader, count int64) error {
	if s, ok := r.(io.Seeker); ok {
		_, err := s.Seek(count, io.SeekCurrent)

		return err
	}

	_, err := io.Copy(io.Discard, io.LimitReader(r, count))

	return err
}

type blockReader struct {
	*squashfs
	r    io.ReadSeeker
	next int64
}

func (b *blockReader) nextReader() error {
	r := io.NewSectionReader(b.reader, b.next, blockHeaderSize)
	ler := byteio.LittleEndianReader{Reader: r}

	header, _, err := ler.ReadUint16()
	if err != nil {
		return err
	}

	size := int64(header & metadataBlockSizeMask)

	if size > blockSize {
		return ErrInvalidBlockHeader
	}

	b.r = io.NewSectionReader(b.reader, b.next+blockHeaderSize, size)

	if header&metadataBlockCompressedMask == 0 {
		b.r, err = b.blockCache.getBlock(b.next, b.r, b.superblock.Compressor)
		if err != nil {
			return err
		}
	}

	b.next += size

	return nil
}

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

	return bytes.NewReader(data), nil
}

func (b *blockCache) getExistingBlock(ptr int64) io.ReadSeeker {
	for _, cb := range b.cache {
		if cb.ptr == ptr {
			return bytes.NewReader(cb.data)
		}
	}

	return nil
}

func (b *blockReader) init(skipCount int64) error {
	if err := b.nextReader(); err != nil {
		return err
	}

	if skipCount > 0 {
		return skip(b.r, skipCount)
	}

	return nil
}

func (b *blockReader) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	if err != nil {
		if !errors.Is(err, io.EOF) {
			return n, err
		}

		if err = b.nextReader(); err != nil {
			return n, err
		}

		var m int

		m, err = b.Read(p[n:])
		n += m
	}

	return n, err
}
