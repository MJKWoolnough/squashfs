package squashfs

import (
	"errors"
	"io"
	"io/fs"
	"sync"

	"vimagination.zapto.org/byteio"
)

type file struct {
	squashfs *squashfs
	file     fileStat

	mu     sync.Mutex
	block  int
	reader io.Reader
}

func (f *file) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.reader == nil {
		if f.block < len(f.file.blockSizes) {
			start := int64(f.file.blocksStart)

			for _, size := range f.file.blockSizes[:f.block] {
				start += int64(size & 0xeffffff)
			}

			var err error
			size := int64(f.file.blockSizes[f.block])
			if size&(1<<24) == 0 {
				if f.reader, err = f.squashfs.superblock.Compressor.decompress(io.NewSectionReader(f.squashfs.reader, start, size)); err != nil {
					return 0, err
				}
			} else {
				f.reader = io.NewSectionReader(f.squashfs.reader, start, size&0xeffffff)
			}
		} else if f.file.fragIndex != 0xFFFFFFFF {
			ler := byteio.LittleEndianReader{Reader: io.NewSectionReader(f.squashfs.reader, int64(f.squashfs.superblock.FragTable)+int64(f.file.fragIndex>>10), 8)}

			mdPos, _, err := ler.ReadUint64()
			if err != nil {
				return 0, err
			}

			r, err := f.squashfs.readMetadata((uint64(f.file.fragIndex)<<3)%8192, mdPos)
			if err != nil {
				return 0, err
			}

			ler = byteio.LittleEndianReader{Reader: r}

			start, _, err := ler.ReadUint64()
			if err != nil {
				return 0, err
			}

			size, _, err := ler.ReadUint32()
			if err != nil {
				return 0, err
			}

			fragmentSize := int64(f.file.fileSize) % int64(f.squashfs.superblock.BlockSize)

			if size&(1<<24) == 0 {
				if f.reader, err = f.squashfs.superblock.Compressor.decompress(io.NewSectionReader(f.squashfs.reader, int64(start), int64(size))); err != nil {
					return 0, err
				}

				if err := skip(f.reader, int64(f.file.blockOffset)); err != nil {
					return 0, err
				}

				f.reader = io.LimitReader(f.reader, fragmentSize)
			} else {
				f.reader = io.NewSectionReader(f.squashfs.reader, int64(start)+int64(f.file.blockOffset), fragmentSize)
			}

		} else {
			return 0, io.EOF
		}
	}

	n, err := f.reader.Read(p)

	if errors.Is(err, io.EOF) {
		if f.block < len(f.file.blockSizes) {
			f.block++
			err = nil
			f.reader = nil
		}
	}

	return n, err
}

func (f *file) Stat() (fs.FileInfo, error) {
	return f.file, nil
}

func (f *file) Close() error {
	return nil
}
