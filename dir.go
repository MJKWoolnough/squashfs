package squashfs

import (
	"io/fs"
	"sync"
)

type dir struct {
	dir dirStat

	mu       sync.Mutex
	squashfs *squashfs
}

func (*dir) Read(_ []byte) (int, error) {
	return 0, fs.ErrInvalid
}

func (d *dir) Stat() (fs.FileInfo, error) {
	return d.dir, nil
}

func (d *dir) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.squashfs == nil {
		return fs.ErrClosed
	}

	d.squashfs = nil

	return nil
}
