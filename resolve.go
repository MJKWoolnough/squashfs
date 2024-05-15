package squashfs

import (
	"errors"
	"io"
	"io/fs"
	"path"
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

func (c commonStat) Size() int64 {
	return 0
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

func (c commonStat) Type() fs.FileMode {
	return c.Mode().Type()
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

type dirIndex struct {
	index uint32
	start uint32
	name  string
}

type dirStat struct {
	commonStat
	blockIndex  uint32
	linkCount   uint32
	fileSize    uint32
	blockOffset uint16
	parentInode uint32
	xattrIndex  uint32
	index       []dirIndex
}

func readBasicDir(ler *byteio.StickyLittleEndianReader, common commonStat) dirStat {
	return dirStat{
		commonStat:  common,
		blockIndex:  ler.ReadUint32(),
		linkCount:   ler.ReadUint32(),
		fileSize:    uint32(ler.ReadUint16()),
		blockOffset: ler.ReadUint16(),
		parentInode: ler.ReadUint32(),
	}
}

func readExtDir(ler *byteio.StickyLittleEndianReader, common commonStat) dirStat {
	d := dirStat{
		commonStat:  common,
		linkCount:   ler.ReadUint32(),
		fileSize:    ler.ReadUint32(),
		blockIndex:  ler.ReadUint32(),
		parentInode: ler.ReadUint32(),
		index:       make([]dirIndex, ler.ReadUint16()),
		blockOffset: ler.ReadUint16(),
		xattrIndex:  ler.ReadUint32(),
	}

	for n := range d.index {
		d.index[n] = dirIndex{
			index: ler.ReadUint32(),
			start: ler.ReadUint32(),
			name:  ler.ReadString(int(ler.ReadUint32()) + 1),
		}
	}

	return d
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

func (d dirStat) Type() fs.FileMode {
	return d.Mode().Type()
}

func (d dirStat) Info() (fs.FileInfo, error) {
	return d, nil
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

const fieldDisabled = 0xFFFFFFFF

func (f *fileStat) readBlocks(ler *byteio.StickyLittleEndianReader, blockSize uint32) {
	var blockCount uint64

	if f.fileSize > 0 {
		if f.fragIndex == fieldDisabled {
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
		commonStat:  common,
		blocksStart: uint64(ler.ReadUint32()),
		fragIndex:   ler.ReadUint32(),
		blockOffset: ler.ReadUint32(),
		fileSize:    uint64(ler.ReadUint32()),
		xattrIndex:  fieldDisabled,
	}

	f.readBlocks(ler, blockSize)

	return f
}

func readExtFile(ler *byteio.StickyLittleEndianReader, common commonStat, blockSize uint32) fileStat {
	f := fileStat{
		commonStat:  common,
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

func (f fileStat) Info() (fs.FileInfo, error) {
	return f, nil
}

type symlinkStat struct {
	commonStat
	linkCount  uint32
	targetPath string
	xattrIndex uint32
}

func readBasicSymlink(ler *byteio.StickyLittleEndianReader, common commonStat) symlinkStat {
	return symlinkStat{
		commonStat: common,
		linkCount:  ler.ReadUint32(),
		targetPath: ler.ReadString(int(ler.ReadUint32())),
		xattrIndex: fieldDisabled,
	}
}

func readExtSymlink(ler *byteio.StickyLittleEndianReader, common commonStat) symlinkStat {
	return symlinkStat{
		commonStat: common,
		linkCount:  ler.ReadUint32(),
		targetPath: ler.ReadString(int(ler.ReadUint32())),
		xattrIndex: ler.ReadUint32(),
	}
}

func (s symlinkStat) Mode() fs.FileMode {
	return fs.ModeSymlink | fs.FileMode(s.perms)
}

func (s symlinkStat) Sys() any {
	return s
}

type blockStat struct {
	commonStat
	linkCount    uint32
	deviceNumber uint32
	xattrIndex   uint32
}

func readBasicBlock(ler *byteio.StickyLittleEndianReader, common commonStat) blockStat {
	return blockStat{
		commonStat:   common,
		linkCount:    ler.ReadUint32(),
		deviceNumber: ler.ReadUint32(),
		xattrIndex:   fieldDisabled,
	}
}

func readExtBlock(ler *byteio.StickyLittleEndianReader, common commonStat) blockStat {
	return blockStat{
		commonStat:   common,
		linkCount:    ler.ReadUint32(),
		deviceNumber: ler.ReadUint32(),
		xattrIndex:   ler.ReadUint32(),
	}
}

func (b blockStat) Mode() fs.FileMode {
	return fs.ModeDevice | fs.FileMode(b.perms)
}

func (b blockStat) Sys() any {
	return b
}

type charStat blockStat

func (c charStat) Mode() fs.FileMode {
	return fs.ModeCharDevice | fs.FileMode(c.perms)
}

func (c charStat) Sys() any {
	return c
}

type fifoStat struct {
	commonStat
	linkCount  uint32
	xattrIndex uint32
}

func readBasicFifo(ler *byteio.StickyLittleEndianReader, common commonStat) fifoStat {
	return fifoStat{
		commonStat: common,
		linkCount:  ler.ReadUint32(),
		xattrIndex: fieldDisabled,
	}
}

func readExtFifo(ler *byteio.StickyLittleEndianReader, common commonStat) fifoStat {
	return fifoStat{
		commonStat: common,
		linkCount:  ler.ReadUint32(),
		xattrIndex: ler.ReadUint32(),
	}
}

func (f fifoStat) Mode() fs.FileMode {
	return fs.ModeNamedPipe | fs.FileMode(f.perms)
}

func (f fifoStat) Sys() any {
	return f
}

type socketStat fifoStat

func (s socketStat) Mode() fs.FileMode {
	return fs.ModeSocket | fs.FileMode(s.perms)
}

func (s socketStat) Sys() any {
	return s
}

func (s *squashfs) readEntry(ler *byteio.StickyLittleEndianReader, typ uint16, common commonStat) fs.FileInfo {
	switch typ {
	case inodeBasicDir:
		return readBasicDir(ler, common)
	case inodeExtDir:
		return readExtDir(ler, common)
	case inodeBasicFile:
		return readBasicFile(ler, common, s.superblock.BlockSize)
	case inodeExtFile:
		return readExtFile(ler, common, s.superblock.BlockSize)
	case inodeBasicSymlink:
		return readBasicSymlink(ler, common)
	case inodeExtSymlink:
		return readExtSymlink(ler, common)
	case inodeBasicBlock:
		return readBasicBlock(ler, common)
	case inodeExtBlock:
		return readExtBlock(ler, common)
	case inodeBasicChar:
		return charStat(readBasicBlock(ler, common))
	case inodeExtChar:
		return charStat(readExtBlock(ler, common))
	case inodeBasicPipe:
		return readBasicFifo(ler, common)
	case inodeExtPipe:
		return readExtFifo(ler, common)
	case inodeBasicSock:
		return socketStat(readBasicFifo(ler, common))
	case inodeExtSock:
		return socketStat(readExtFifo(ler, common))
	default:
		ler.Err = fs.ErrInvalid

		return nil
	}
}

func (s *squashfs) getEntry(inode uint64, name string) (fs.FileInfo, error) {
	r, err := s.readMetadata(inode, s.superblock.InodeTable)
	if err != nil {
		return nil, err
	}

	ler := byteio.StickyLittleEndianReader{Reader: r}

	typ := ler.ReadUint16()
	perms := ler.ReadUint16()

	common := commonStat{
		name:  name,
		perms: perms,
		uid:   s.getID(&ler),
		gid:   s.getID(&ler),
		mtime: time.Unix(int64(ler.ReadUint32()), 0),
	}

	ler.ReadUint32() // inode number?

	fi := s.readEntry(&ler, typ, common)

	if ler.Err != nil {
		return nil, ler.Err
	}

	return fi, nil
}

func (s *squashfs) getID(ler *byteio.StickyLittleEndianReader) uint32 {
	id := ler.ReadUint16()
	if id >= s.superblock.IDCount {
		ler.Err = fs.ErrInvalid

		return 0
	}

	const (
		idPosShift = 2
		idLength   = 4
	)

	r := ler.Reader
	ler.Reader = io.NewSectionReader(s.reader, int64(id<<idPosShift), idLength)
	pid := ler.ReadUint32()
	ler.Reader = r

	return pid
}

func (s *squashfs) getDirEntry(name string, blockIndex uint32, blockOffset uint16, totalSize uint32) (fs.FileInfo, error) {
	r, err := s.readMetadata(uint64(blockIndex)<<metadataPointerShift|uint64(blockOffset), s.superblock.DirTable)
	if err != nil {
		return nil, err
	}

	ler := byteio.StickyLittleEndianReader{Reader: io.LimitReader(r, int64(totalSize-dirFileSizeOffset))}

	d := dir{
		squashfs: s,
	}

	for {
		de := d.readDirEntry(&ler)

		if errors.Is(ler.Err, io.EOF) {
			return nil, fs.ErrNotExist
		} else if ler.Err != nil {
			return nil, ler.Err
		} else if de.name == name {
			return de.Info()
		} else if name < de.name {
			return nil, fs.ErrNotExist
		}
	}
}

type resolver struct {
	*squashfs
	fullPath, path     string
	cutAt              int
	redirectsRemaining int
}

func (s *squashfs) resolve(fpath string, resolveLast bool) (fs.FileInfo, error) {
	if !fs.ValidPath(fpath) {
		return nil, fs.ErrInvalid
	}

	root, err := s.getEntry(s.superblock.RootInode, "")
	if err != nil {
		return nil, err
	}

	const maximumRedirects = 1024

	r := resolver{
		squashfs:           s,
		fullPath:           fpath,
		path:               fpath,
		redirectsRemaining: maximumRedirects,
	}

	return r.resolve(root, resolveLast)
}

func (r *resolver) resolve(root fs.FileInfo, resolveLast bool) (curr fs.FileInfo, err error) {
	curr = root

	for r.path != "" {
		if dir, ok := curr.(dirStat); !ok {
			return nil, fs.ErrInvalid
		} else if name := r.splitOffNamePart(); isEmptyName(name) {
			continue
		} else if curr, err = r.getDirEntry(name, dir.blockIndex, dir.blockOffset, dir.fileSize); err != nil {
			return nil, err
		} else if r.isDone(resolveLast) {
			break
		} else if sym, ok := curr.(symlinkStat); !ok {
			continue
		} else if err := r.handleSymlink(sym); err != nil {
			return nil, err
		}

		curr = root
	}

	return curr, nil
}

func (r *resolver) splitOffNamePart() string {
	slashPos := strings.Index(r.path, "/")

	var name string

	if slashPos == -1 {
		name, r.path = r.path, ""
	} else {
		name, r.path = r.path[:slashPos], r.path[slashPos+1:]
		r.cutAt += slashPos + 1
	}

	return name
}

func (r *resolver) handleSymlink(sym symlinkStat) error {
	r.redirectsRemaining--
	if r.redirectsRemaining == 0 {
		return fs.ErrInvalid
	}

	if strings.HasPrefix(sym.targetPath, "/") {
		r.fullPath = path.Clean(sym.targetPath)[1:]
	} else if r.path == "" {
		r.fullPath = path.Join(r.fullPath[:r.cutAt], sym.targetPath, r.path)
	} else {
		r.fullPath = path.Join(r.fullPath[:r.cutAt-len(sym.name)-1], sym.targetPath, r.path)
	}

	r.path = r.fullPath
	r.cutAt = 0

	return nil
}

func (r *resolver) isDone(resolveLast bool) bool {
	return r.path == "" && !resolveLast
}

func isEmptyName(name string) bool {
	return name == "" || name == "."
}
