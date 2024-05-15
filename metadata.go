package squashfs

import (
	"errors"
	"io"

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

type blockReader struct {
	*squashfs
	r    io.ReadSeeker
	next int64
}

func (b *blockReader) nextReader() error {
	ler := byteio.LittleEndianReader{Reader: io.NewSectionReader(b.reader, b.next, blockHeaderSize)}

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

func (b *blockReader) init(skipCount int64) error {
	if err := b.nextReader(); err != nil {
		return err
	}

	if skipCount > 0 {
		_, err := b.r.Seek(skipCount, io.SeekStart)

		return err
	}

	return nil
}

func (b *blockReader) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	if err == nil {
		return n, nil
	}

	if !errors.Is(err, io.EOF) {
		return n, err
	}

	if err = b.nextReader(); err != nil {
		return n, err
	}

	m, err := b.Read(p[n:])
	n += m

	return n, err
}
