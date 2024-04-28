package squashfs

import (
	"errors"
	"io"
	"io/fs"
	"strings"
	"time"

	"vimagination.zapto.org/byteio"
)

type commonStat struct {
	name  string
	perms uint16
	uid   uint32
	gid   uint32
	mtime time.Time
}

func (c commonStat) Name() string {
	return c.name
}

func (c commonStat) Mode() fs.FileMode {
	return fs.FileMode(c.perms)
}

func (c commonStat) ModTime() time.Time {
	return c.mtime
}

func (c commonStat) IsDir() bool {
	return false
}

const (
	inodeBasicDir     = 1
	inodeBasicFile    = 2
	inodeBasicSymlink = 3
	inodeBasicBlock   = 4
	inodeBasicChar    = 5
	inodeBasicPipe    = 6
	inodeBasicSock    = 7
	inodeExtDir       = 8
	inodeExtFile      = 9
	inodeExtSymlink   = 10
	inodeExtBlock     = 11
	inodeExtChar      = 12
	inodeExtPipe      = 13
	inodeExtSock      = 14
)

type basicDir struct {
	commonStat
	blockIndex  uint32
	linkCount   uint32
	fileSize    uint16
	blockOffset uint16
	parentInode uint32
}

func readBasicDir(ler *byteio.StickyLittleEndianReader, common commonStat) basicDir {
	return basicDir{
		commonStat:  common,
		blockIndex:  ler.ReadUint32(),
		linkCount:   ler.ReadUint32(),
		fileSize:    ler.ReadUint16(),
		blockOffset: ler.ReadUint16(),
		parentInode: ler.ReadUint32(),
	}
}

func (d basicDir) Mode() fs.FileMode {
	return fs.ModeDir | fs.FileMode(d.perms)
}

func (d basicDir) IsDir() bool {
	return true
}

func (d basicDir) Size() int64 {
	return 0
}

func (d basicDir) Sys() any {
	return d
}

func (s *squashfs) getEntry(inode uint64) (fs.FileInfo, error) {
	r, err := s.readMetadata(inode, s.superblock.InodeTable)
	if err != nil {
		return nil, err
	}

	ler := byteio.StickyLittleEndianReader{Reader: r}

	typ := ler.ReadUint16()
	perms := ler.ReadUint16()
	uid := ler.ReadUint16()
	gid := ler.ReadUint16()
	mtime := ler.ReadUint32()
	ler.ReadUint32() // inode number?

	common := commonStat{
		perms: perms,
		uid:   uint32(uid), // TODO: Lookup actual ID
		gid:   uint32(gid), // TODO: Lookup actual ID
		mtime: time.Unix(int64(mtime), 0),
	}

	var fi fs.FileInfo

	switch typ {
	case inodeBasicDir:
		fi = readBasicDir(&ler, common)
	default:
		return nil, errors.New("unimplemented")
	}

	if ler.Err != nil {
		return nil, ler.Err
	}

	return fi, nil
}

func (s *squashfs) getDirEntry(name string, blockIndex uint32, blockOffset, totalSize uint16) (fs.FileInfo, error) {
	r, err := s.readMetadata(uint64(blockIndex)<<16|uint64(blockOffset), s.superblock.DirTable)
	if err != nil {
		return nil, err
	}

	ler := byteio.StickyLittleEndianReader{Reader: io.LimitReader(r, int64(totalSize))}

	for {
		count := ler.ReadUint32()
		start := uint64(ler.ReadUint32())
		ler.ReadUint32() // inode number

		if errors.Is(ler.Err, io.EOF) {
			return nil, fs.ErrNotExist
		} else if ler.Err != nil {
			return nil, ler.Err
		}

		for i := uint32(0); i <= count; i++ {
			offset := uint64(ler.ReadUint16())
			ler.ReadInt16()  // inode offset
			ler.ReadUint16() // type
			nameSize := int(ler.ReadUint16())
			dname := ler.ReadString(nameSize + 1)

			if dname == name {
				return s.getEntry(start<<16 | offset)
			} else if name < dname {
				return nil, fs.ErrNotExist
			}
		}
	}
}

func (s *squashfs) resolve(path string) (fs.FileInfo, error) {
	curr, err := s.getEntry(s.superblock.RootInode)
	if err != nil {
		return nil, err
	}

	for path != "" {
		slashPos := strings.Index(path, "/")

		var name string

		if slashPos == -1 {
			name = path
			path = ""
		} else {
			name = path[:slashPos]
			path = path[slashPos+1:]
		}

		if name == "" {
			continue
		}

		switch dir := curr.(type) {
		case basicDir:
			curr, err = s.getDirEntry(name, dir.blockIndex, dir.blockOffset, dir.fileSize)
			if err != nil {
				return nil, err
			}
		default:
			return nil, fs.ErrInvalid
		}
	}

	return curr, nil
}

func (s *squashfs) Stat(path string) (fs.FileInfo, error) {
	return s.resolve(path)
}
