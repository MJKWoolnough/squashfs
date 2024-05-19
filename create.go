package squashfs

import (
	"io"
)

type Creater struct {
	writer     io.WriterAt
	compressor CompressorOptions
}

func Create(w io.WriterAt) (*Creater, error) {
	return &Creater{
		writer:     w,
		compressor: defaultGzipOptions(),
	}, nil
}
