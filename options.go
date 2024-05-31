package squashfs

import (
	"io/fs"
	"math/bits"
	"time"
)

const (
	minBlockSize     = 1 << 12 // 4K
	defaultBlockSize = 1 << 17 // 128K
	maxBlockSize     = 1 << 20 // 1MB
)

type Option func(*Builder) error

func BlockSize(blockSize uint32) Option {
	return func(b *Builder) error {
		if blockSize < minBlockSize || blockSize > maxBlockSize || bits.OnesCount32(blockSize) != 1 {
			return ErrInvalidBlockSize
		}

		b.superblock.BlockSize = blockSize

		return nil
	}
}

var (
	BlockSize4K   = BlockSize(minBlockSize)
	BlockSize16K  = BlockSize(1 << 14)
	BlockSize128K = BlockSize(defaultBlockSize)
	BlockSize1M   = BlockSize(maxBlockSize)
)

func Compression(c CompressorOptions) Option {
	return func(b *Builder) error {
		if c == nil {
			return ErrInvalidCompressor
		}

		b.superblock.CompressionOptions = c

		if c.isDefault() {
			b.superblock.Flags &= ^uint16(flagCompressionOptions)
		} else {
			b.superblock.Flags |= flagCompressionOptions
		}

		return nil
	}
}

func ExportTable() Option {
	return func(b *Builder) error {
		b.superblock.Stats.Flags |= 0x80

		return nil
	}
}

func SqfsModTime(t uint32) Option {
	return func(b *Builder) error {
		b.superblock.Stats.ModTime = time.Unix(int64(t), 0)

		return nil
	}
}

func DefaultMode(m fs.FileMode) Option {
	return func(b *Builder) error {
		b.defaultMode = m

		return nil
	}
}

func DefaultOwner(owner, group uint32) Option {
	return func(b *Builder) error {
		b.defaultOwner = owner
		b.defaultGroup = group

		return nil
	}
}

func DefaultModTime(t time.Time) Option {
	return func(b *Builder) error {
		b.defaultModTime = t

		return nil
	}
}

type InodeOption func(*node)

func Owner(owner, group uint32) InodeOption {
	return func(n *node) {
		n.owner = owner
		n.group = group
	}
}

func ModTime(t time.Time) InodeOption {
	return func(n *node) {
		n.modTime = t
	}
}

func Mode(m fs.FileMode) InodeOption {
	return func(n *node) {
		n.mode = m
	}
}
