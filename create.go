package squashfs

import (
	"io"
)

const (
	defaultBlockSize = 1 << 17 // 128K
)

type Option func(*superblock) error

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
