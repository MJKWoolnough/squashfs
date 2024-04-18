package squashfs

import (
	"fmt"
	"io"
	"time"
)

// Type Stats contains basic data about the SquashFS file, read from the
// superblock.
type Stats struct {
	Inodes     uint32
	ModTime    time.Time
	BlockSize  uint32
	FragCount  uint32
	Compressor Compressor
	Flags      uint16
	BytesUsed  uint64
}

// ReadStats reads the superblock from the passed reader and returns useful
// stats.
func ReadStats(r io.Reader) (*Stats, error) {
	sb, err := readSuperBlock(r)
	if err != nil {
		return nil, fmt.Errorf("error reading superblock: %w", err)
	}

	return &sb.Stats, nil
}
