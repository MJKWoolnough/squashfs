// Package squashfs is a SquashFS reader and writer using fs.FS
package squashfs // import "vimagination.zapto.org/squashfs"

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"

	"vimagination.zapto.org/byteio"
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

func (s *superblock) readFrom(r io.Reader) error {
	var buf [104]byte

	_, err := io.ReadFull(r, buf[:])
	if err != nil {
		return err
	}

	mb := memio.Buffer(buf[:])

	ler := byteio.StickyLittleEndianReader{Reader: &mb}

	if ler.ReadUint32() != 0x73717368 {
		return ErrInvalidMagicNumber
	}

	s.Inodes = ler.ReadUint32()
	s.ModTime = time.Unix(int64(ler.ReadUint32()), 0)
	s.BlockSize = ler.ReadUint32()
	s.FragCount = ler.ReadUint32()
	s.Compressor = Compressor(ler.ReadUint16())

	if 1<<ler.ReadUint16() != s.BlockSize {
		return ErrInvalidBlockSize
	}

	s.Flags = ler.ReadUint16()
	s.IDCount = ler.ReadUint16()

	if ler.ReadUint16() != 4 || ler.ReadUint16() != 0 {
		return ErrInvalidVersion
	}

	s.RootInode = ler.ReadUint64()
	s.BytesUsed = ler.ReadUint64()
	s.IDTable = ler.ReadUint64()
	s.XattrTable = ler.ReadUint64()
	s.InodeTable = ler.ReadUint64()
	s.DirTable = ler.ReadUint64()
	s.FragTable = ler.ReadUint64()
	s.ExportTable = ler.ReadUint64()

	s.CompressionOptions, err = s.Compressor.parseOptions(s.Flags&0x400 != 0, &ler)

	return err
}

type squashfs struct {
	superblock superblock
	reader     io.ReaderAt
}

func (s *squashfs) Open(path string) (fs.File, error) {
	return nil, errors.New("unimplemented")
}

type FS interface {
	fs.StatFS
}

func Open(r io.ReaderAt) (FS, error) {
	var sb superblock
	if err := sb.readFrom(io.NewSectionReader(r, 0, 104)); err != nil {
		return nil, fmt.Errorf("error reading superblock: %w", err)
	}

	return &squashfs{
		superblock: sb,
		reader:     r,
	}, nil
}

func (s *squashfs) Stat(path string) (fs.FileInfo, error) {
	return s.resolve(path)
}

var (
	ErrInvalidMagicNumber = errors.New("invalid magic number")
	ErrInvalidBlockSize   = errors.New("invalid block size")
	ErrInvalidVersion     = errors.New("invalid version")
)
