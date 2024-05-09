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
	squashfs *squashfs
	reader   io.Reader
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
	if f.reader == nil {
		reader, err := f.getOffsetReader(f.pos)
		if err != nil {
			return 0, err
		}

		f.reader = reader
	}

	n, err := f.reader.Read(p)

	f.pos += int64(n)

	if errors.Is(err, io.EOF) {
		if uint64(f.pos) < f.file.fileSize {
			f.reader = nil
			err = nil
		}
	}

	if err == nil {
		if n < len(p) {
			var m int

			m, err = f.read(p[n:])

			n += m
		}
	}

	return n, err
}

func (f *file) getReader(block int) (io.Reader, error) {
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

func (f *file) getOffsetReader(pos int64) (io.Reader, error) {
	if uint64(pos) >= f.file.fileSize {
		return nil, io.ErrUnexpectedEOF
	}

	block, skipBytes := f.getBlockOffset(pos)

	reader, err := f.getReader(block)
	if err != nil {
		return nil, err
	}

	if skipBytes > 0 {
		err = skip(reader, skipBytes)
		if err != nil {
			return nil, err
		}
	}

	return reader, nil
}

const (
	sizeMask           = 0x00ffffff
	compressionMask    = 0xff000000
	fragmentIndexShift = 4
)

func (f *file) getBlockReader(block int) (io.Reader, error) {
	start := int64(f.file.blocksStart)

	for _, size := range f.file.blockSizes[:block] {
		start += int64(size & sizeMask)
	}

	size := int64(f.file.blockSizes[block])
	if size&compressionMask == 0 {
		return f.squashfs.superblock.Compressor.decompress(io.NewSectionReader(f.squashfs.reader, start, size))
	}

	return io.NewSectionReader(f.squashfs.reader, start, size&sizeMask), nil
}

func (f *file) getFragmentDetails() (start uint64, size uint32, err error) {
	ler := byteio.StickyLittleEndianReader{Reader: io.NewSectionReader(f.squashfs.reader, int64(f.squashfs.superblock.FragTable)+int64(f.file.fragIndex>>10), 8)}

	mdPos := ler.ReadUint64()

	if ler.Err != nil {
		return 0, 0, ler.Err
	}

	ler.Reader, ler.Err = f.squashfs.readMetadata((uint64(f.file.fragIndex)<<fragmentIndexShift)%blockSize, mdPos)

	start = ler.ReadUint64()
	size = ler.ReadUint32()

	if ler.ReadUint32() != 0 {
		return 0, 0, fs.ErrInvalid
	}

	return start, size, ler.Err
}

func (f *file) getFragmentReader() (io.Reader, error) {
	start, size, err := f.getFragmentDetails()
	if err != nil {
		return nil, err
	}

	fragmentSize := int64(f.file.fileSize) % int64(f.squashfs.superblock.BlockSize)

	if size&compressionMask == 0 {
		reader, err := f.squashfs.superblock.Compressor.decompress(io.NewSectionReader(f.squashfs.reader, int64(start), int64(size)))
		if err != nil {
			return nil, err
		}

		if err := skip(reader, int64(f.file.blockOffset)); err != nil {
			return nil, err
		}

		return io.LimitReader(reader, fragmentSize), nil
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

	if base > f.pos && f.reader != nil {
		cBlock, _ := f.getBlockOffset(f.pos)
		bBlock, _ := f.getBlockOffset(base)

		if cBlock == bBlock {
			if err := skip(f.reader, base-f.pos); err != nil {
				return f.pos, err
			}
		} else {
			f.reader = nil
		}
	} else {
		f.reader = nil
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
