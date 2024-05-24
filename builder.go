package squashfs

import (
	"io"
	"io/fs"
	"path"
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

func (b *Builder) Dir(p string, options ...InodeOption) error {
	n, err := b.addNode(p, options...)
	if err != nil {
		return err
	}

	n.children = make([]*node, 0)
	n.mode |= fs.ModeDir

	return nil
}

func (b *Builder) addNode(p string, options ...InodeOption) (*node, error) {
	if !fs.ValidPath(p) {
		return nil, fs.ErrInvalid
	}

	if p == "." {
		return nil, fs.ErrExist
	}

	n := &node{
		name: path.Base(p),
	}

	if o := b.getParent(b.root, p); o == nil {
		return nil, fs.ErrInvalid
	} else if n != o.insertSortedNode(n) {
		return nil, fs.ErrExist
	}

	n.modTime = b.nodeModTime()
	n.mode = b.defaultMode | fs.ModePerm
	n.owner = b.defaultOwner
	n.group = b.defaultGroup

	for _, opt := range options {
		opt(n)
	}

	return n, nil
}

func (b *Builder) File(p string, r io.Reader, options ...InodeOption) error {
	_, err := b.addNode(p, options...)
	if err != nil {
		return err
	}

	return nil
}

func (b *Builder) Symlink(p, dest string, options ...InodeOption) error {
	_, err := b.addNode(p, options...)
	if err != nil {
		return err
	}

	return nil
}

type node struct {
	name         string
	owner, group uint32
	mode         fs.FileMode
	modTime      time.Time
	children     []*node
}

func (b *Builder) getParent(n *node, path string) *node {
	first, rest := splitPath(path)

	if first == "" {
		return n
	}

	p := n.insertSortedNode(&node{
		name:     first,
		owner:    b.defaultOwner,
		group:    b.defaultGroup,
		mode:     b.defaultMode,
		modTime:  b.nodeModTime(),
		children: make([]*node, 0),
	})

	if rest != "" {
		return b.getParent(p, rest)
	}

	return p
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
