package squashfs

import (
	"io"
	"io/fs"
	"slices"
	"strings"
	"time"
)

type Builder struct {
	writer     io.WriterAt
	superblock superblock

	defaultMode    fs.FileMode
	defaultOwner   uint32
	defaultGroup   uint32
	defaultModTime time.Time

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
		owner:    b.defaultOwner,
		group:    b.defaultGroup,
		children: make([]*node, 0),
		modTime:  b.nodeModTime(),
	}

	return b, nil
}

func (b *Builder) nodeModTime() time.Time {
	if b.defaultModTime.IsZero() {
		return time.Now()
	}

	return b.defaultModTime
}

func (b *Builder) Dir(path string, mode fs.FileMode) error {
	if !fs.ValidPath(path) {
		return fs.ErrInvalid
	}

	if path == "." {
		return fs.ErrExist
	}

	return b.addChild(b.root, path, b.defaultOwner, b.defaultGroup, mode, false)
}

type node struct {
	name         string
	owner, group uint32
	mode         fs.FileMode
	modTime      time.Time
	children     []*node
}

func (b *Builder) addChild(n *node, path string, owner, group uint32, mode fs.FileMode, isFile bool) error {
	first, rest := splitPath(path)

	o := &node{
		name:     first,
		owner:    b.defaultOwner,
		group:    b.defaultGroup,
		mode:     b.defaultMode,
		modTime:  b.nodeModTime(),
		children: make([]*node, 0),
	}

	p := n.insertSortedNode(o)

	if rest != "" {
		return b.addChild(p, rest, owner, group, mode, isFile)
	}

	if o != p {
		return fs.ErrExist
	}

	o.owner = owner
	o.group = group
	o.mode = mode

	if isFile {
		o.children = nil
	}

	return nil
}

func (n *node) insertSortedNode(i *node) *node {
	pos, exists := slices.BinarySearchFunc(n.children, i, func(a, b *node) int {
		return strings.Compare(a.name, b.name)
	})

	if exists {
		return n.children[pos]
	}

	n.children = slices.Insert(n.children, pos, i)

	return i
}

func splitPath(path string) (string, string) {
	pos := strings.IndexByte(path, '/')
	if pos == -1 {
		return "", path
	}

	return path[:pos], path[pos+1:]
}
