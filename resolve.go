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

type dirStat struct {
	commonStat
	blockIndex  uint32
	linkCount   uint32
	fileSize    uint16
	blockOffset uint16
	parentInode uint32
}

func readBasicDir(ler *byteio.StickyLittleEndianReader, common commonStat) dirStat {
	return dirStat{
		commonStat:  common,
		blockIndex:  ler.ReadUint32(),
		linkCount:   ler.ReadUint32(),
		fileSize:    ler.ReadUint16(),
		blockOffset: ler.ReadUint16(),
		parentInode: ler.ReadUint32(),
	}
}

func (d dirStat) Mode() fs.FileMode {
	return fs.ModeDir | fs.FileMode(d.perms)
}

func (d dirStat) IsDir() bool {
	return true
}

func (d dirStat) Size() int64 {
	return 0
}

func (d dirStat) Sys() any {
	return d
}

type fileStat struct {
	commonStat
	blocksStart uint64
	sparse      uint64
	linkCount   uint32
	fragIndex   uint32
	blockOffset uint32
	fileSize    uint64
	xattrIndex  uint32
	blockSizes  []uint32
}

func (f *fileStat) readBlocks(ler *byteio.StickyLittleEndianReader, blockSize uint32) {
	var blockCount uint64

	if f.fileSize > 0 {
		if f.fragIndex == 0xFFFFFFFF {
			blockCount = 1 + (f.fileSize-1)/uint64(blockSize)
		} else {
			blockCount = f.fileSize / uint64(blockSize)
		}
	}

	f.blockSizes = make([]uint32, blockCount)

	for n := range f.blockSizes {
		f.blockSizes[n] = ler.ReadUint32()
	}
}

func readBasicFile(ler *byteio.StickyLittleEndianReader, common commonStat, blockSize uint32) fileStat {
	f := fileStat{
		blocksStart: uint64(ler.ReadUint32()),
		fragIndex:   ler.ReadUint32(),
		blockOffset: ler.ReadUint32(),
		fileSize:    uint64(ler.ReadUint32()),
		xattrIndex:  0xFFFFFFFF,
	}

	f.readBlocks(ler, blockSize)

	return f
}

func readExtendedFile(ler *byteio.StickyLittleEndianReader, common commonStat, blockSize uint32) fileStat {
	f := fileStat{
		blocksStart: ler.ReadUint64(),
		fileSize:    ler.ReadUint64(),
		sparse:      ler.ReadUint64(),
		linkCount:   ler.ReadUint32(),
		fragIndex:   ler.ReadUint32(),
		blockOffset: ler.ReadUint32(),
		xattrIndex:  ler.ReadUint32(),
	}

	f.readBlocks(ler, blockSize)

	return f
}

func (f fileStat) Size() int64 {
	return int64(f.fileSize)
}

func (f fileStat) Sys() any {
	return f
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
	case inodeBasicFile:
		fi = readBasicFile(&ler, common, s.superblock.BlockSize)
	case inodeExtFile:
		fi = readExtendedFile(&ler, common, s.superblock.BlockSize)
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
		case dirStat:
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