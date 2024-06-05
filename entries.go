package squashfs

import (
	"errors"
	"io"
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
	inode uint32
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

func (c commonStat) writeTo(lew *byteio.StickyLittleEndianWriter) {
	lew.WriteUint16(c.perms)
	lew.WriteUint16(uint16(c.uid))
	lew.WriteUint16(uint16(c.gid))
	lew.WriteUint32(uint32(c.mtime.Unix()))
	lew.WriteUint32(c.inode)
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

	fieldDisabled = 0xffffffff
)

type dirIndex struct {
	index uint32
	start uint32
	name  string
}

func (d dirIndex) writeTo(lew *byteio.StickyLittleEndianWriter) {
	lew.WriteUint32(d.index)
	lew.WriteUint32(d.start)
	lew.WriteUint32(uint32(len(d.name) - 1))
	lew.WriteString(d.name)
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

func (d dirStat) writeTo(lew *byteio.StickyLittleEndianWriter) {
	if d.xattrIndex != fieldDisabled || len(d.index) > 0 || d.fileSize > 0xffff {
		d.writeExtTo(lew)
	} else {
		d.writeBasicTo(lew)
	}
}

func (d dirStat) writeExtTo(lew *byteio.StickyLittleEndianWriter) {
	lew.WriteUint16(inodeExtDir)
	d.commonStat.writeTo(lew)
	lew.WriteUint32(d.linkCount)
	lew.WriteUint32(d.fileSize)
	lew.WriteUint32(d.blockIndex)
	lew.WriteUint32(d.parentInode)
	lew.WriteUint16(uint16(len(d.index)))
	lew.WriteUint16(d.blockOffset)
	lew.WriteUint32(d.xattrIndex)

	for _, e := range d.index {
		e.writeTo(lew)
	}
}

func (d dirStat) writeBasicTo(lew *byteio.StickyLittleEndianWriter) {
	lew.WriteUint16(inodeBasicDir)
	d.commonStat.writeTo(lew)
	lew.WriteUint32(d.blockIndex)
	lew.WriteUint32(d.linkCount)
	lew.WriteUint16(uint16(d.fileSize))
	lew.WriteUint16(d.blockOffset)
	lew.WriteUint32(d.parentInode)
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

	if f.fileSize != 0 {
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

func (f fileStat) writeTo(lew *byteio.StickyLittleEndianWriter) {
	if f.blocksStart > 0xffffffff || f.fileSize > 0xffffffff || f.linkCount > 0 || f.xattrIndex != fieldDisabled || f.sparse > 0 {
		f.writeExtTo(lew)
	} else {
		f.writeBasicTo(lew)
	}
}

func (f fileStat) writeExtTo(lew *byteio.StickyLittleEndianWriter) {
	lew.WriteUint16(inodeExtFile)
	f.commonStat.writeTo(lew)
	lew.WriteUint64(f.blocksStart)
	lew.WriteUint64(f.fileSize)
	lew.WriteUint64(f.sparse)
	lew.WriteUint32(f.linkCount)
	lew.WriteUint32(f.fragIndex)
	lew.WriteUint32(f.blockOffset)
	lew.WriteUint32(f.xattrIndex)
	f.writeBlocks(lew)
}

func (f fileStat) writeBlocks(lew *byteio.StickyLittleEndianWriter) {
	for _, b := range f.blockSizes {
		lew.WriteUint32(b)
	}
}

func (f fileStat) writeBasicTo(lew *byteio.StickyLittleEndianWriter) {
	lew.WriteUint16(inodeBasicFile)
	f.commonStat.writeTo(lew)
	lew.WriteUint32(uint32(f.blocksStart))
	lew.WriteUint32(f.fragIndex)
	lew.WriteUint32(f.blockOffset)
	lew.WriteUint32(uint32(f.fileSize))
	f.writeBlocks(lew)
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
		targetPath: ler.ReadString32(),
		xattrIndex: fieldDisabled,
	}
}

func readExtSymlink(ler *byteio.StickyLittleEndianReader, common commonStat) symlinkStat {
	return symlinkStat{
		commonStat: common,
		linkCount:  ler.ReadUint32(),
		targetPath: ler.ReadString32(),
		xattrIndex: ler.ReadUint32(),
	}
}

func (s symlinkStat) Mode() fs.FileMode {
	return fs.ModeSymlink | fs.FileMode(s.perms)
}

func (s symlinkStat) Sys() any {
	return s
}

func (s symlinkStat) writeTo(lew *byteio.StickyLittleEndianWriter) {
	if s.xattrIndex != fieldDisabled {
		s.writeExtTo(lew)
	} else {
		s.writeBasicTo(lew)
	}
}

func (s symlinkStat) writeExtTo(lew *byteio.StickyLittleEndianWriter) {
	lew.WriteUint16(inodeExtSymlink)
	s.commonStat.writeTo(lew)
	lew.WriteUint32(s.linkCount)
	lew.WriteString32(s.targetPath)
	lew.WriteUint32(s.xattrIndex)
}

func (s symlinkStat) writeBasicTo(lew *byteio.StickyLittleEndianWriter) {
	lew.WriteUint16(inodeBasicSymlink)
	s.commonStat.writeTo(lew)
	lew.WriteUint32(s.linkCount)
	lew.WriteString32(s.targetPath)
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

func (b blockStat) writeTo(lew *byteio.StickyLittleEndianWriter) {
	if b.xattrIndex != fieldDisabled {
		b.writeExtTo(lew)
	} else {
		b.writeBasicTo(lew)
	}
}

func (b blockStat) writeExtTo(lew *byteio.StickyLittleEndianWriter) {
	lew.WriteUint16(inodeExtBlock)
	b.commonStat.writeTo(lew)
	lew.WriteUint32(b.linkCount)
	lew.WriteUint32(b.deviceNumber)
	lew.WriteUint32(b.xattrIndex)
}

func (b blockStat) writeBasicTo(lew *byteio.StickyLittleEndianWriter) {
	lew.WriteUint16(inodeBasicBlock)
	b.commonStat.writeTo(lew)
	lew.WriteUint32(b.linkCount)
	lew.WriteUint32(b.deviceNumber)
}

type charStat blockStat

func (c charStat) Mode() fs.FileMode {
	return fs.ModeCharDevice | fs.FileMode(c.perms)
}

func (c charStat) Sys() any {
	return c
}

func (c charStat) writeTo(lew *byteio.StickyLittleEndianWriter) {
	if c.xattrIndex != fieldDisabled {
		c.writeExtTo(lew)
	} else {
		c.writeBasicTo(lew)
	}
}

func (c charStat) writeExtTo(lew *byteio.StickyLittleEndianWriter) {
	lew.WriteUint16(inodeExtChar)
	c.commonStat.writeTo(lew)
	lew.WriteUint32(c.linkCount)
	lew.WriteUint32(c.deviceNumber)
	lew.WriteUint32(c.xattrIndex)
}

func (c charStat) writeBasicTo(lew *byteio.StickyLittleEndianWriter) {
	lew.WriteUint16(inodeBasicChar)
	c.commonStat.writeTo(lew)
	lew.WriteUint32(c.linkCount)
	lew.WriteUint32(c.deviceNumber)
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

func (f fifoStat) writeTo(lew *byteio.StickyLittleEndianWriter) {
	if f.xattrIndex != fieldDisabled {
		f.writeExtTo(lew)
	} else {
		f.writeBasicTo(lew)
	}
}

func (f fifoStat) writeExtTo(lew *byteio.StickyLittleEndianWriter) {
	lew.WriteUint16(inodeExtPipe)
	f.commonStat.writeTo(lew)
	lew.WriteUint32(f.linkCount)
	lew.WriteUint32(f.xattrIndex)
}

func (f fifoStat) writeBasicTo(lew *byteio.StickyLittleEndianWriter) {
	lew.WriteUint16(inodeBasicPipe)
	f.commonStat.writeTo(lew)
	lew.WriteUint32(f.linkCount)
}

type socketStat fifoStat

func (s socketStat) Mode() fs.FileMode {
	return fs.ModeSocket | fs.FileMode(s.perms)
}

func (s socketStat) Sys() any {
	return s
}

func (s socketStat) writeTo(lew *byteio.StickyLittleEndianWriter) {
	if s.xattrIndex != fieldDisabled {
		s.writeExtTo(lew)
	} else {
		s.writeBasicTo(lew)
	}
}

func (s socketStat) writeExtTo(lew *byteio.StickyLittleEndianWriter) {
	lew.WriteUint16(inodeExtSock)
	s.commonStat.writeTo(lew)
	lew.WriteUint32(s.linkCount)
	lew.WriteUint32(s.xattrIndex)
}

func (s socketStat) writeBasicTo(lew *byteio.StickyLittleEndianWriter) {
	lew.WriteUint16(inodeBasicSock)
	s.commonStat.writeTo(lew)
	lew.WriteUint32(s.linkCount)
}

func (s *SquashFS) readEntry(ler *byteio.StickyLittleEndianReader, typ uint16, common commonStat) fs.FileInfo {
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

func (s *SquashFS) getEntry(inode uint64, name string) (fs.FileInfo, error) {
	r, err := s.readMetadata(inode, s.superblock.InodeTable)
	if err != nil {
		return nil, err
	}

	ler := byteio.StickyLittleEndianReader{Reader: r}

	typ := ler.ReadUint16()

	common := commonStat{
		name:  name,
		perms: ler.ReadUint16(),
		uid:   s.getID(&ler),
		gid:   s.getID(&ler),
		mtime: time.Unix(int64(ler.ReadUint32()), 0),
		inode: ler.ReadUint32(),
	}

	fi := s.readEntry(&ler, typ, common)

	if ler.Err != nil {
		return nil, ler.Err
	}

	return fi, nil
}

func (s *SquashFS) getID(ler *byteio.StickyLittleEndianReader) uint32 {
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
	mr, err := s.readMetadataFromLookupTable(int64(s.superblock.IDTable), int64(id), 4)
	if err != nil && ler.Err == nil {
		ler.Err = err
	}

	ler.Reader = mr
	pid := ler.ReadUint32()
	ler.Reader = r

	return pid
}

func (s *SquashFS) getDirEntry(name string, index uint32, offset uint16, totalSize uint32) (fs.FileInfo, error) {
	r, err := s.readMetadata(uint64(index)<<metadataPointerShift|uint64(offset), s.superblock.DirTable)
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
