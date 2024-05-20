package squashfs

import (
	"math/bits"
	"time"
)

const (
	minBlockSize     = 1 << 12 // 4K
	defaultBlockSize = 1 << 17 // 128K
	maxBlockSize     = 1 << 20 // 1MB
)

type Option func(*superblock) error

func BlockSize(blockSize uint32) Option {
	return func(s *superblock) error {
		if blockSize < minBlockSize || blockSize > maxBlockSize || bits.OnesCount32(blockSize) != 1 {
			return ErrInvalidBlockSize
		}

		s.BlockSize = blockSize

		return nil
	}
}

var (
	BlockSize4K   = BlockSize(minBlockSize)
	BlockSize16K  = BlockSize(1 << 14)
	BlockSize128K = BlockSize(defaultBlockSize)
	BlockSize1M   = BlockSize(maxBlockSize)
)

func Compression(c CompressorOptions) Option {
	return func(s *superblock) error {
		if c == nil {
			return ErrInvalidCompressor
		}

		s.CompressionOptions = c

		return nil
	}
}

func ExportTable() Option {
	return func(s *superblock) error {
		s.Stats.Flags |= 0x80

		return nil
	}
}

func ModTime(t uint32) Option {
	return func(s *superblock) error {
		s.Stats.ModTime = time.Unix(int64(t), 0)

		return nil
	}
}
