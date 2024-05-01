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
		var err error
		if f.file.fragIndex != 0xFFFFFFFF && f.block == len(f.file.blockSizes) {
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

			if size&(1<<24) == 0 {
				f.reader, err = f.squashfs.superblock.Compressor.decompress(io.NewSectionReader(f.squashfs.reader, int64(start), int64(size)))
			} else {
				size = size ^ (1 << 24)
				f.reader = io.NewSectionReader(f.squashfs.reader, int64(start)+int64(f.file.blockOffset), int64(size))
			}

			if _, err := (&skipSeeker{Reader: f.reader}).Seek(int64(f.file.fragIndex), io.SeekCurrent); err != nil {
				return 0, err
			}

			f.reader = io.LimitReader(f.reader, int64(f.file.fileSize)%int64(f.squashfs.superblock.BlockSize))
		} else if f.reader, err = f.squashfs.superblock.Compressor.decompress(io.NewSectionReader(f.squashfs.reader, int64(f.file.blocksStart)+int64(f.block)*int64(f.squashfs.superblock.BlockSize), int64(f.file.blockSizes[f.block]))); err != nil {
			return 0, err
		}
	}

	n, err := f.reader.Read(p)

	if errors.Is(err, io.EOF) {
		f.block++
		if f.block < len(f.file.blockSizes) {
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
