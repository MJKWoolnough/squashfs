package squashfs

import (
	"io"
)

type Creater struct {
	writer     io.WriterAt
	superblock superblock
}

func Create(w io.WriterAt, options ...Option) (*Creater, error) {
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

	return &Creater{
		writer:     w,
		superblock: s,
	}, nil
}
