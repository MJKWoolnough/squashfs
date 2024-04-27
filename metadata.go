package squashfs

import (
	"errors"
	"io"

	"vimagination.zapto.org/byteio"
)

const maxBlockSize = 1 << 13

func (s *squashfs) ReadInode(pointer uint64) (*blockReader, error) {
	onDisk := int64(s.superblock.InodeTable + (pointer >> 16))

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

type skipSeeker struct {
	io.Reader
}

func (s *skipSeeker) Seek(offset int64, whence int) (int64, error) {
	if whence != io.SeekCurrent {
		return 0, ErrUnsupportedSeek
	}

	return io.Copy(io.Discard, io.LimitReader(s, offset))
}

type blockReader struct {
	*squashfs
	r           io.ReadSeeker
	next        int64
	compression Compressor
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
		c, err := b.compression.decompress(b.r)
		if err != nil {
			return err
		}

		b.r = &skipSeeker{Reader: c}
	}

	b.next += int64(size)

	return nil
}

func (b *blockReader) init(skip int64) error {
	if err := b.nextReader(); err != nil {
		return err
	}

	_, err := b.r.Seek(skip, io.SeekCurrent)

	return err
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

var (
	ErrInvalidPointer     = errors.New("invalid pointer")
	ErrInvalidBlockHeader = errors.New("invalid block header")
	ErrUnsupportedSeek    = errors.New("unsupported seek")
)
