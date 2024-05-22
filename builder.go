package squashfs

import (
	"io"
)

type Builder struct {
	writer     io.WriterAt
	superblock superblock
}

func Create(w io.WriterAt, options ...Option) (*Builder, error) {
	s := superblock{
		Stats: Stats{
			BlockSize: defaultBlockSize,
		},
		CompressionOptions: DefaultGzipOptions(),
	}

	for _, o := range options {
		if err := o(&s); err != nil {
			return nil, err
		}
	}

	return &Builder{
		writer:     w,
		superblock: s,
	}, nil
}
