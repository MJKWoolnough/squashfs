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
	Compressor Compressor
	Flags      uint16
	BytesUsed  uint64
}

func GetStats(r io.Reader) (*Stats, error) {
	sb, err := readSuperBlock(r)
	if err != nil {
		return nil, err
	}

	return &sb.Stats, nil
}