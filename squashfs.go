package squashfs

import (
	"io"
	"time"

	"vimagination.zapto.org/byteio"
	"vimagination.zapto.org/errors"
	"vimagination.zapto.org/memio"
)

type Stats struct {
	Inodes     uint32
	ModTime    time.Time
	BlockSize  uint32
	FragCount  uint32
	Compressor uint16
	Flags      uint16
	BytesUsed  uint64
}

func GetStats(r io.Reader) (*Stats, error) {
	var buf [104]byte

	_, err := io.ReadFull(r, buf[:])
	if err != nil {
		return nil, err
	}

	mb := memio.Buffer(buf[:])

	ler := byteio.StickyLittleEndianReader{Reader: &mb}

	if ler.ReadUint32() != 0x73717368 {
		return nil, ErrInvalidMagicNumber
	}

	inodes := ler.ReadUint32()
	modtime := ler.ReadUint32()
	blocksize := ler.ReadUint32()
	fragcount := ler.ReadUint32()
	compressor := ler.ReadUint16()

	if 1<<ler.ReadUint16() != blocksize {
		return nil, ErrInvalidBlockSize
	}

	flags := ler.ReadUint16()
	ler.ReadUint16() // id count

	if ler.ReadUint16() != 4 || ler.ReadUint16() != 0 {
		return nil, ErrInvalidVersion
	}

	ler.ReadUint64() // root inode
	bytesused := ler.ReadUint64()
	ler.ReadUint64() // xattr table
	ler.ReadUint64() // inode table
	ler.ReadUint64() // dir table
	ler.ReadUint64() // frag table
	ler.ReadUint64() // export table

	return &Stats{
		Inodes:     inodes,
		ModTime:    time.Unix(int64(modtime), 0),
		BlockSize:  blocksize,
		FragCount:  fragcount,
		Compressor: compressor,
		Flags:      flags,
		BytesUsed:  bytesused,
	}, nil
}

const (
	ErrInvalidMagicNumber = errors.Error("invalid magic number")
	ErrInvalidBlockSize   = errors.Error("invalid block size")
	ErrInvalidVersion     = errors.Error("invalid version")
)
