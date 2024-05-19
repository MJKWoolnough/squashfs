package squashfs

import (
	"io"
)

type CompressorMaker interface {
	makeWriter(io.Writer) (io.WriteCloser, error)
}

type Creater struct {
	writer     io.WriterAt
	compressor CompressorMaker
}

func Create(w io.WriterAt) (*Creater, error) {
	return &Creater{
		writer:     w,
		compressor: defaultGzipOptions(),
	}, nil
}
