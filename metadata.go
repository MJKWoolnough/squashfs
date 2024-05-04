package squashfs

import (
	"errors"
	"io"

	"vimagination.zapto.org/byteio"
)

const maxBlockSize = 1 << 13

func (s *squashfs) readMetadata(pointer, table uint64) (*blockReader, error) {
	onDisk := int64(table + (pointer >> 16))

	pos := int64(pointer & 0xffff)
	if pos > maxBlockSize {
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
	r    io.Reader
	next int64
}

func (b *blockReader) nextReader() error {
	r := io.NewSectionReader(b.reader, b.next, 2)
	ler := byteio.LittleEndianReader{Reader: r}

	header, _, err := ler.ReadUint16()
	if err != nil {
		return err
	}

	size := int64(header & 0x7fff)

	if size > maxBlockSize {
		return ErrInvalidBlockHeader
	}

	b.r = io.NewSectionReader(b.reader, b.next+2, size)

	if header>>15 == 0 {
		c, err := b.squashfs.superblock.Compressor.decompress(b.r)
		if err != nil {
			return err
		}

		b.r = c
	}

	b.next += int64(size)

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

		if err := b.nextReader(); err != nil {
			return n, err
		}

		var m int

		m, err = b.Read(p[n:])
		n += m
	}

	return n, err
}
