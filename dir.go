package squashfs

import (
	"io"
	"io/fs"
	"sync"
)

type dir struct {
	dir dirStat

	mu       sync.Mutex
	squashfs *squashfs
	reader   io.Reader
}

func (s *squashfs) newDir(dirStat dirStat) (*dir, error) {
	r, err := s.readMetadata(uint64(dirStat.blockIndex)<<16|uint64(dirStat.blockOffset), s.superblock.DirTable)
	if err != nil {
		return nil, err
	}

	return &dir{
		dir:      dirStat,
		squashfs: s,
		reader:   io.LimitReader(r, int64(dirStat.fileSize)),
	}, nil
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
