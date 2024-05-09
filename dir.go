package squashfs

import (
	"errors"
	"io"
	"io/fs"
	"sync"

	"vimagination.zapto.org/byteio"
)

type dir struct {
	dir dirStat

	mu       sync.Mutex
	squashfs *squashfs
	reader   io.Reader
	count    uint32
	start    uint32
}

func (s *squashfs) newDir(dirStat dirStat) (*dir, error) {
	r, err := s.readMetadata(uint64(dirStat.blockIndex)<<16|uint64(dirStat.blockOffset), s.superblock.DirTable)
	if err != nil {
		return nil, err
	}

	return &dir{
		dir:      dirStat,
		squashfs: s,
		reader:   io.LimitReader(r, int64(dirStat.fileSize-3)),
	}, nil
}

func (*dir) Read(_ []byte) (int, error) {
	return 0, fs.ErrInvalid
}

func (d *dir) ReadDir(n int) ([]fs.DirEntry, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.squashfs == nil {
		return nil, fs.ErrClosed
	}

	ler := byteio.StickyLittleEndianReader{Reader: d.reader}

	m := n

	if n <= 0 {
		m = int(d.dir.linkCount)
	}

	entries := make([]fs.DirEntry, m)

	for m := range entries {
		if d.count == 0 {
			d.count = ler.ReadUint32()
			d.start = ler.ReadUint32()
			ler.ReadUint32()
		}

		offset := uint64(ler.ReadUint16())
		ler.ReadInt16() // inode offset
		typ := ler.ReadUint16()
		name := ler.ReadString(int(ler.ReadUint16()) + 1)

		if ler.Err != nil {
			if n > 0 || !errors.Is(ler.Err, io.EOF) {
				return nil, ler.Err
			}

			return entries[:m], nil
		}

		entries[m] = dirEntry{
			squashfs: d.squashfs,
			typ:      typ,
			name:     name,
			ptr:      uint64(d.start<<16) | offset,
		}

		d.count--
	}

	return entries, nil
}

func (d *dir) Stat() (fs.FileInfo, error) {
	return d.dir, nil
}

func (d *dir) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.squashfs == nil {
		return fs.ErrClosed
	}

	d.squashfs = nil

	return nil
}

type dirEntry struct {
	squashfs *squashfs
	typ      uint16
	name     string
	ptr      uint64
}

func (d dirEntry) Name() string {
	return d.name
}

func (d dirEntry) IsDir() bool {
	return d.typ == inodeBasicDir
}

func (d dirEntry) Type() fs.FileMode {
	switch d.typ {
	case inodeBasicDir:
		return fs.ModeDir
	case inodeBasicFile:
		return 0
	case inodeBasicSymlink:
		return fs.ModeSymlink
	case inodeBasicBlock:
		return fs.ModeDevice
	case inodeBasicChar:
		return fs.ModeCharDevice
	case inodeBasicPipe:
		return fs.ModeNamedPipe
	case inodeBasicSock:
		return fs.ModeSocket
	}

	return fs.ModeIrregular
}

func (d dirEntry) Info() (fs.FileInfo, error) {
	return d.squashfs.getEntry(d.ptr, d.name)
}
