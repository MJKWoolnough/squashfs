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
			err = nil
			f.reader = nil
		}
	}

	return n, err
}

func (f *file) getOffsetReader(pos int64) (io.Reader, error) {
	block, skipBytes := int(pos/int64(f.squashfs.superblock.BlockSize)), pos%int64(f.squashfs.superblock.BlockSize)

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

func (f *file) getReader(block int) (io.Reader, error) {
	if block < len(f.file.blockSizes) {
		start := int64(f.file.blocksStart)

		for _, size := range f.file.blockSizes[:block] {
			start += int64(size & 0xeffffff)
		}

		size := int64(f.file.blockSizes[block])
		if size&(1<<24) == 0 {
			return f.squashfs.superblock.Compressor.decompress(io.NewSectionReader(f.squashfs.reader, start, size))
		}

		return io.NewSectionReader(f.squashfs.reader, start, size&0xeffffff), nil
	} else if f.file.fragIndex != 0xFFFFFFFF {
		ler := byteio.LittleEndianReader{Reader: io.NewSectionReader(f.squashfs.reader, int64(f.squashfs.superblock.FragTable)+int64(f.file.fragIndex>>10), 8)}

		mdPos, _, err := ler.ReadUint64()
		if err != nil {
			return nil, err
		}

		r, err := f.squashfs.readMetadata((uint64(f.file.fragIndex)<<4)%8192, mdPos)
		if err != nil {
			return nil, err
		}

		ler = byteio.LittleEndianReader{Reader: r}

		start, _, err := ler.ReadUint64()
		if err != nil {
			return nil, err
		}

		size, _, err := ler.ReadUint32()
		if err != nil {
			return nil, err
		}

		if unused, _, err := ler.ReadUint32(); err != nil {
			return nil, err
		} else if unused != 0 {
			return nil, fs.ErrInvalid
		}

		fragmentSize := int64(f.file.fileSize) % int64(f.squashfs.superblock.BlockSize)

		if size&(1<<24) == 0 {
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

	return nil, io.EOF
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

	f.pos = base + offset

	if f.pos < 0 {
		return f.pos, fs.ErrInvalid
	}

	f.reader = nil

	return base, nil
}

func (f *file) ReadAt(p []byte, offset int64) (int, error) {
	reader, err := f.getOffsetReader(offset)
	if err != nil {
		return 0, err
	}

	return reader.Read(p)
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
