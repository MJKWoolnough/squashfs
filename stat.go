package squashfs

import (
	"errors"
	"io/fs"
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

func (s *squashfs) Stat(path string) (fs.FileInfo, error) {
	r, err := s.ReadInode(s.superblock.RootInode)
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
