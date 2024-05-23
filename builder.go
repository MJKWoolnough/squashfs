package squashfs

import (
	"io"
	"io/fs"
	"slices"
	"strings"
)

type Builder struct {
	writer     io.WriterAt
	superblock superblock

	defaultMode  fs.FileMode
	defaultOwner uint32
	defaultGroup uint32

	root *node
}

func Create(w io.WriterAt, options ...Option) (*Builder, error) {
	b := &Builder{
		writer: w,

		superblock: superblock{
			Stats: Stats{
				BlockSize: defaultBlockSize,
			},
			CompressionOptions: DefaultGzipOptions(),
		},
	}

	for _, o := range options {
		if err := o(b); err != nil {
			return nil, err
		}
	}

	b.root = &node{
		owner: b.defaultOwner,
		group: b.defaultGroup,
	}

	return b, nil
}

func (b *Builder) Dir(path string, mode fs.FileMode) error {
	if !fs.ValidPath(path) {
		return fs.ErrInvalid
	}

	return b.root.addDir(path, mode)
}

type node struct {
	name         string
	owner, group uint32
	modTime      uint32
	children     []*node
}

func (n *node) addDir(path string, mode fs.FileMode) error {
	return nil
}

func (n *node) insertSortedNode(i *node) error {
	pos, exists := slices.BinarySearchFunc(n.children, i, func(a, b *node) int {
		return strings.Compare(a.name, b.name)
	})

	if exists {
		return fs.ErrExist
	}

	n.children = slices.Insert(n.children, pos, i)

	return nil
}
