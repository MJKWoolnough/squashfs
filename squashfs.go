// Package squashfs is a SquashFS reader and writer using fs.FS
package squashfs // import "vimagination.zapto.org/squashfs"

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"time"

	"vimagination.zapto.org/byteio"
)

const headerLength = 104

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
	var buf [headerLength]byte

	_, err := io.ReadFull(r, buf[:])
	if err != nil {
		return err
	}

	ler := byteio.StickyLittleEndianReader{Reader: bytes.NewBuffer(buf[:])}

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
	f, err := s.resolve(path)
	if err != nil {
		return nil, err
	}

	if f, ok := f.(fileStat); ok {
		return &file{
			squashfs: s,
			file:     f,
		}, nil
	}

	return nil, fs.ErrInvalid
}

type FS interface {
	fs.StatFS
}

// Open reads the passed io.ReaderAt as a SquashFS image, returning a fs.FS
// implementation.
//
// The returned fs.FS, and any files opened from it will cease to work if the
// io.ReaderAt is closed.
func Open(r io.ReaderAt) (FS, error) {
	var sb superblock
	if err := sb.readFrom(io.NewSectionReader(r, 0, headerLength)); err != nil {
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
