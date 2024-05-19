package squashfs

import (
	"io"
)

type Creater struct {
	writer     io.WriterAt
	superblock superblock
}

func Create(w io.WriterAt) (*Creater, error) {
	return &Creater{
		writer: w,
		superblock: superblock{
			CompressionOptions: defaultGzipOptions(),
		},
	}, nil
}
