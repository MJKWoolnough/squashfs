package squashfs

import (
	"errors"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"

	"vimagination.zapto.org/memio"
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
		owner:   b.defaultOwner,
		group:   b.defaultGroup,
		modTime: b.nodeModTime(),
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

	n := &node{name: path.Base(p)}

	if o := b.getParent(b.root, p); o == nil {
		return nil, fs.ErrInvalid
	} else if n != o.insertSortedNode(n) {
		return nil, fs.ErrExist
	}

	n.mode = b.defaultMode | fs.ModePerm
	n.owner = b.defaultOwner
	n.group = b.defaultGroup

	for _, opt := range options {
		opt(n)
	}

	if n.modTime.IsZero() {
		n.modTime = b.nodeModTime()
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
	n, err := b.addNode(p, options...)
	if err != nil {
		return err
	}

	n.mode = fs.ModeSymlink | fs.ModePerm

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
		name:  first,
		owner: b.defaultOwner,
		group: b.defaultGroup,
		mode:  fs.ModeDir | b.defaultMode,
	})

	if !p.mode.IsDir() {
		return nil
	}

	if p.modTime.IsZero() {
		p.modTime = b.nodeModTime()
	}

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

type blockWriter struct {
	w            *io.OffsetWriter
	uncompressed []byte
	compressed   memio.LimitedBuffer
	compressor   compressedWriter
}

func newBlockWriter(w io.WriterAt, start int64, blockSize int, compressor compressedWriter) blockWriter {
	return blockWriter{
		w:            io.NewOffsetWriter(w, start),
		uncompressed: make([]byte, blockSize),
		compressed:   make(memio.LimitedBuffer, 0, blockSize),
		compressor:   compressor,
	}
}

func (b *blockWriter) Pos() int64 {
	pos, _ := b.w.Seek(0, io.SeekCurrent)

	return pos
}

func (b *blockWriter) writeFile(r io.Reader) ([]uint32, error) {
	var sizes []uint32

	for {
		_, err := io.ReadFull(r, b.uncompressed)
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, err
		}

		c := b.compressed

		b.compressor.Reset(&c)

		var toWrite []byte

		if _, err = b.compressor.Write(b.uncompressed); errors.Is(err, io.ErrShortWrite) {
			toWrite = b.uncompressed
		} else if err != nil {
			return nil, err
		} else {
			toWrite = c
		}

		if _, err = b.w.Write(toWrite); err != nil {
			return nil, err
		}

		sizes = append(sizes, uint32(len(toWrite)))
	}

	return sizes, nil
}
