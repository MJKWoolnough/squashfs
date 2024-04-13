package squashfs

import (
	"io"
	"time"
)

type Stats struct {
	Inodes     uint32
	ModTime    time.Time
	BlockSize  uint32
	FragCount  uint32
	Compressor uint16
	Flags      uint16
	BytesUsed  uint64
}

func GetStats(r io.ReaderAt) (Stats, error) {
	return Stats{}, nil
}
