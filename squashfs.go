package squashfs

import (
	"io"
	"time"

	"vimagination.zapto.org/byteio"
	"vimagination.zapto.org/errors"
	"vimagination.zapto.org/memio"
)

type superblock struct {
	Stats
	IDCount            uint16
	RootInode          uint64
	IDTable            uint64
	XattrTable         uint64
	InodeTable         uint64
	DirTable           uint64
	FragTable          uint64
	ExportTable        uint64
	CompressionOptions CompressorOptions
}

func readSuperBlock(r io.Reader) (*superblock, error) {
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
	compressor := Compressor(ler.ReadUint16())

	if 1<<ler.ReadUint16() != blocksize {
		return nil, ErrInvalidBlockSize
	}

	flags := ler.ReadUint16()
	idcount := ler.ReadUint16()

	if ler.ReadUint16() != 4 || ler.ReadUint16() != 0 {
		return nil, ErrInvalidVersion
	}

	rootinode := ler.ReadUint64()
	bytesused := ler.ReadUint64()
	xattrtable := ler.ReadUint64()
	inodetable := ler.ReadUint64()
	dirtable := ler.ReadUint64()
	fragtable := ler.ReadUint64()
	exporttable := ler.ReadUint64()

	compressoroptions, err := compressor.parseOptions(flags&0x400 != 0, &ler)
	if err != nil {
		return nil, err
	}

	return &superblock{
		Stats: Stats{
			Inodes:     inodes,
			ModTime:    time.Unix(int64(modtime), 0),
			BlockSize:  blocksize,
			FragCount:  fragcount,
			Compressor: compressor,
			Flags:      flags,
			BytesUsed:  bytesused,
		},
		IDCount:            idcount,
		RootInode:          rootinode,
		XattrTable:         xattrtable,
		InodeTable:         inodetable,
		DirTable:           dirtable,
		FragTable:          fragtable,
		ExportTable:        exporttable,
		CompressionOptions: compressoroptions,
	}, nil
}

const (
	ErrInvalidMagicNumber = errors.Error("invalid magic number")
	ErrInvalidBlockSize   = errors.Error("invalid block size")
	ErrInvalidVersion     = errors.Error("invalid version")
)
