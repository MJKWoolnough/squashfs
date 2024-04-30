package squashfs

import (
	"errors"
	"io"
	"io/fs"
	"sync"
)

type file struct {
	squashfs *squashfs
	file     fileStat

	mu     sync.Mutex
	block  int
	reader io.Reader
}

func (f *file) Read(p []byte) (int, error) {
	return 0, errors.New("unimplemented")
}

func (f *file) Stat() (fs.FileInfo, error) {
	return f.file, nil
}

func (f *file) Close() error {
	return nil
}
