// Package squashfs is a SquashFS reader and writer using fs.FS
package squashfs // import "vimagination.zapto.org/squashfs"

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"

	"vimagination.zapto.org/byteio"
)

const (
	headerLength     = 104
	magic            = 0x73717368 // hsqs
	defaultCacheSize = 1024
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
	var buf [headerLength]byte

	_, err := io.ReadFull(r, buf[:])
	if err != nil {
		return err
	}

	ler := byteio.StickyLittleEndianReader{Reader: bytes.NewBuffer(buf[:])}

	if ler.ReadUint32() != magic {
		return ErrInvalidMagicNumber
	}

	if err = s.readSuperBlockDetails(&ler); err != nil {
		return err
	}

	s.CompressionOptions, err = s.Compressor.parseOptions(s.Flags&0x400 != 0, &ler)

	return err
}

func (s *superblock) readSuperBlockDetails(ler *byteio.StickyLittleEndianReader) error {
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

	return nil
}

type squashfs struct {
	superblock superblock
	reader     io.ReaderAt

	blockCache blockCache
}

func (s *squashfs) Open(path string) (fs.File, error) {
	f, err := s.resolve(path, true)
	if err != nil {
		return nil, err
	}

	switch f := f.(type) {
	case fileStat:
		return &file{
			squashfs: s,
			file:     f,
		}, nil
	case dirStat:
		return s.newDir(f)
	}

	return nil, fs.ErrInvalid
}

func (s *squashfs) ReadFile(name string) ([]byte, error) {
	f, err := s.Open(name)
	if err != nil {
		return nil, err
	}

	ff, ok := f.(*file)
	if !ok {
		return nil, fs.ErrInvalid
	}

	buf := make([]byte, ff.file.fileSize)

	if _, err = ff.read(buf); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}

	return buf, nil
}

func (s *squashfs) ReadDir(name string) ([]fs.DirEntry, error) {
	d, err := s.Open(name)
	if err != nil {
		return nil, err
	}

	dd, ok := d.(*dir)
	if !ok {
		return nil, fs.ErrInvalid
	}

	return dd.ReadDir(-1)
}

type FS interface {
	fs.ReadFileFS
	fs.ReadDirFS
	fs.StatFS

	// Lstat returns a FileInfo describing the named file. If the file is a
	// symbolic link, the returned FileInfo describes the symbolic link.
	LStat(name string) (fs.FileInfo, error)
}

// Open reads the passed io.ReaderAt as a SquashFS image, returning a fs.FS
// implementation.
//
// The returned fs.FS, and any files opened from it will cease to work if the
// io.ReaderAt is closed.
func Open(r io.ReaderAt) (FS, error) {
	return OpenWithCacheSize(r, defaultCacheSize)
}

// OpenWithCacheSize acts like Open, but allows a custom cache size, which
// normally defaults to 1024.
func OpenWithCacheSize(r io.ReaderAt, cacheSize uint) (FS, error) {
	var sb superblock
	if err := sb.readFrom(io.NewSectionReader(r, 0, headerLength)); err != nil {
		return nil, fmt.Errorf("error reading superblock: %w", err)
	}

	return &squashfs{
		superblock: sb,
		reader:     r,
		blockCache: newBlockCache(cacheSize),
	}, nil
}

func (s *squashfs) Stat(path string) (fs.FileInfo, error) {
	return s.resolve(path, true)
}

func (s *squashfs) LStat(path string) (fs.FileInfo, error) {
	return s.resolve(path, false)
}
