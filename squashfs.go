// Package squashfs is a SquashFS reader and writer using fs.FS
package squashfs // import "vimagination.zapto.org/squashfs"

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
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

type inodeData struct {
	name        string
	permissions uint16
	uid         uint16
	gid         uint16
	mtime       time.Time
	inode       uint32
}

func (i *inodeData) Stat() (fs.FileInfo, error) {
	return nil, nil
}

type dirInode struct {
	inodeData
	blockIndex  uint32
	linkCount   uint32
	fileSize    uint16
	blockOffset uint16
	parent      uint32
}

func (d *dirInode) getChild(name string) (fs.File, error) {
	return nil, nil
}

func (d *dirInode) Read(_ []byte) (int, error) {
	return 0, nil
}

func (d *dirInode) Close() error {
	return nil
}

type squashfs struct {
	superblock *superblock
	reader     io.ReaderAt
	root       *dirInode
}

func (s *squashfs) Open(path string) (fs.File, error) {
	dir, file := filepath.Split(path)

	path = filepath.ToSlash(dir)
	if strings.HasPrefix(path, "/") {
		path = path[1:]
	}

	inode := s.root

	for _, part := range strings.Split(path, "/") {
		child, err := inode.getChild(part)
		if err != nil {
			return nil, err
		}

		switch child := child.(type) {
		case *dirInode:
			inode = child
		default:
			return nil, fs.ErrInvalid
		}
	}

	return inode.getChild(file)
}

type FS interface {
	fs.FS
	fs.StatFS
}

func Open(r io.ReaderAt) (FS, error) {
	sb, err := readSuperBlock(io.NewSectionReader(r, 0, 104))
	if err != nil {
		return nil, fmt.Errorf("error reading superblock: %w", err)
	}

	return &squashfs{
		superblock: sb,
		reader:     r,
	}, nil
}

var (
	ErrInvalidMagicNumber = errors.New("invalid magic number")
	ErrInvalidBlockSize   = errors.New("invalid block size")
	ErrInvalidVersion     = errors.New("invalid version")
)
