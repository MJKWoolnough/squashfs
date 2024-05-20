package squashfs

import (
	"io"
	"math/bits"
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

type Creater struct {
	writer     io.WriterAt
	superblock superblock
}

func Create(w io.WriterAt, options ...Option) (*Creater, error) {
	s := superblock{
		Stats: Stats{
			BlockSize: defaultBlockSize,
		},
		CompressionOptions: defaultGzipOptions(),
	}

	for _, o := range options {
		if err := o(&s); err != nil {
			return nil, err
		}
	}

	return &Creater{
		writer:     w,
		superblock: s,
	}, nil
}
