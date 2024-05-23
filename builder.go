package squashfs

import (
	"io"
	"io/fs"
)

type Builder struct {
	writer     io.WriterAt
	superblock superblock

	defaultMode  fs.FileMode
	defaultOwner uint32
	defaultGroup uint32
}

func Create(w io.WriterAt, options ...Option) (*Builder, error) {
	b := &Builder{
		writer: w,

		superblock: superblock{
			Stats: Stats{
				BlockSize: defaultBlockSize,
			},
			CompressionOptions: DefaultGzipOptions(),
		},
	}

	for _, o := range options {
		if err := o(b); err != nil {
			return nil, err
		}
	}

	return b, nil
}
