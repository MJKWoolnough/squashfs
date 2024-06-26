package squashfs

import (
	"errors"
	"io"
	"io/fs"
	"sync"

	"vimagination.zapto.org/byteio"
)

type file struct {
	file fileStat

	mu       sync.Mutex
	squashfs *SquashFS
	reader   io.ReadSeeker
	pos      int64
}

func (f *file) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.squashfs == nil {
		return 0, fs.ErrClosed
	}

	return f.read(p)
}

func (f *file) read(p []byte) (int, error) {
	if err := f.prepareReader(); err != nil {
		return 0, err
	}

	n, err := f.reader.Read(p)

	f.pos += int64(n)

	if errors.Is(err, io.EOF) && uint64(f.pos) < f.file.fileSize {
		f.reader = nil
		err = nil
	}

	if err == nil && n < len(p) {
		var m int

		m, err = f.read(p[n:])

		n += m
	}

	return n, err
}

func (f *file) prepareReader() error {
	if f.reader != nil {
		return nil
	}

	if uint64(f.pos) == f.file.fileSize {
		return io.EOF
	}

	reader, err := f.getOffsetReader(f.pos)
	if err != nil {
		return err
	}

	f.reader = reader

	return nil
}

func (f *file) getReader(block int) (io.ReadSeeker, error) {
	if block < len(f.file.blockSizes) {
		return f.getBlockReader(block)
	} else if f.file.fragIndex != fieldDisabled {
		return f.getFragmentReader()
	}

	return nil, io.EOF
}

func (f *file) getBlockOffset(pos int64) (int, int64) {
	return int(pos / int64(f.squashfs.superblock.BlockSize)), pos % int64(f.squashfs.superblock.BlockSize)
}

func (f *file) getOffsetReader(pos int64) (io.ReadSeeker, error) {
	if uint64(pos) >= f.file.fileSize {
		return nil, io.ErrUnexpectedEOF
	}

	block, skipBytes := f.getBlockOffset(pos)

	reader, err := f.getReader(block)
	if err != nil {
		return nil, err
	}

	if skipBytes > 0 {
		if _, err = reader.Seek(skipBytes, io.SeekStart); err != nil {
			return nil, err
		}
	}

	return reader, nil
}

const (
	sizeMask        = 0x00ffffff
	compressionMask = 0x01000000

	fragmentDetailSize = 16
)

func (f *file) getBlockReader(block int) (io.ReadSeeker, error) {
	start := int64(f.file.blocksStart)

	for _, size := range f.file.blockSizes[:block] {
		start += int64(size & sizeMask)
	}

	size := int64(f.file.blockSizes[block])

	var c Compressor
	if size&compressionMask == 0 {
		c = f.squashfs.superblock.Compressor
	}

	r := io.NewSectionReader(f.squashfs.reader, start, size&sizeMask)

	return f.squashfs.blockCache.getBlock(start, r, c)
}

func (f *file) getFragmentDetails() (start uint64, size uint32, err error) {
	r, err := f.squashfs.readMetadataFromLookupTable(int64(f.squashfs.superblock.FragTable), int64(f.file.fragIndex), fragmentDetailSize)
	ler := byteio.StickyLittleEndianReader{
		Reader: r,
	}

	start = ler.ReadUint64()
	size = ler.ReadUint32()

	if ler.ReadUint32() != 0 {
		return 0, 0, fs.ErrInvalid
	}

	return start, size, ler.Err
}

func (f *file) getFragmentReader() (io.ReadSeeker, error) {
	start, size, err := f.getFragmentDetails()
	if err != nil {
		return nil, err
	}

	fragmentSize := int64(f.file.fileSize) % int64(f.squashfs.superblock.BlockSize)

	if size&compressionMask == 0 {
		r := io.NewSectionReader(f.squashfs.reader, int64(start), int64(size))

		reader, err := f.squashfs.blockCache.getBlock(int64(start), r, f.squashfs.superblock.Compressor)
		if err != nil {
			return nil, err
		}

		return io.NewSectionReader(reader, int64(f.file.blockOffset), fragmentSize), nil
	}

	return io.NewSectionReader(f.squashfs.reader, int64(start)+int64(f.file.blockOffset), fragmentSize), nil
}

func (f *file) Seek(offset int64, whence int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.squashfs == nil {
		return 0, fs.ErrClosed
	}

	var base int64

	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		base = f.pos
	case io.SeekEnd:
		base = int64(f.file.fileSize)
	default:
		return f.pos, fs.ErrInvalid
	}

	base += offset

	if base < 0 {
		return f.pos, fs.ErrInvalid
	}

	return f.setPos(base)
}

func (f *file) setPos(base int64) (int64, error) {
	if f.reader != nil {
		cBlock, _ := f.getBlockOffset(f.pos)
		bBlock, offset := f.getBlockOffset(base)

		if cBlock != bBlock {
			f.reader = nil
		} else if _, err := f.reader.Seek(offset, io.SeekStart); err != nil {
			return f.pos, err
		}
	}

	f.pos = base

	return base, nil
}

func (f *file) ReadAt(p []byte, offset int64) (int, error) {
	f.mu.Lock()
	sqfs := f.squashfs
	f.mu.Unlock()

	if sqfs == nil {
		return 0, fs.ErrClosed
	}

	if uint64(offset) == f.file.fileSize {
		return 0, io.EOF
	}

	g := file{
		file:     f.file,
		squashfs: sqfs,
		pos:      offset,
	}

	return g.read(p)
}

func (f *file) Stat() (fs.FileInfo, error) {
	return f.file, nil
}

func (f *file) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.squashfs == nil {
		return fs.ErrClosed
	}

	f.squashfs = nil

	return nil
}
