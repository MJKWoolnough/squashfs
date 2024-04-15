package squashfs

import (
	"os"
	"testing"
)

func TestGetStats(t *testing.T) {
	sqfs, err := buildSquashFS(
		t,
		dir("dirA", []child{
			file("fileA", "my contents"),
		}),
	)
	if err != nil {
		t.Fatalf("unexpected error creating squashfs file: %s", err)
	}

	f, err := os.Open(sqfs)
	if err != nil {
		t.Fatalf("unexpected error opening squashfs file: %s", err)
	}
	defer f.Close()

	stats, err := GetStats(f)
	if err != nil {
		t.Fatalf("unexpected error reading squashfs file: %s", err)
	}

	if stats.Inodes != 3 {
		t.Errorf("expecting 3 inodes, got %d", stats.Inodes)
	}

	const blockSize = 128 << 10

	if stats.BlockSize != blockSize {
		t.Errorf("expecting block size of %d, got %d", blockSize, stats.BlockSize)
	}
}
