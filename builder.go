package squashfs

import (
	"errors"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"vimagination.zapto.org/byteio"
	"vimagination.zapto.org/memio"
)

type Builder struct {
	writer     io.WriterAt
	superblock superblock

	defaultMode    fs.FileMode
	defaultOwner   uint32
	defaultGroup   uint32
	defaultModTime time.Time

	blockWriter blockWriter

	mu   sync.Mutex
	root *dirNode
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

	blockStart := int64(headerLength)

	if b.superblock.Flags&flagCompressionOptions == 0 {
		blockStart -= compressionOptionsLength
	}

	c, err := b.superblock.CompressionOptions.getCompressedWriter()
	if err != nil {
		return nil, err
	}

	b.blockWriter = newBlockWriter(w, blockStart, b.superblock.BlockSize, c)

	b.root = &dirNode{}

	return b, nil
}

func (b *Builder) nodeModTime() time.Time {
	if b.defaultModTime.IsZero() {
		return time.Now()
	}

	return b.defaultModTime
}

func (b *Builder) Dir(p string, options ...InodeOption) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.addNode(p, &dirNode{
		entry: entry{
			name: path.Base(p),
		},
	}); err != nil {
		return err
	}

	return nil
}

func (b *Builder) addNode(p string, c childNode) error {
	if !fs.ValidPath(p) {
		return fs.ErrInvalid
	}

	if p == "." {
		return fs.ErrExist
	}

	if o := b.getParent(b.root, p); o == nil {
		return fs.ErrInvalid
	} else if c != o.insertSortedNode(c) {
		return fs.ErrExist
	}

	return nil
}

func (b *Builder) File(p string, r io.Reader, options ...InodeOption) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	_, err := b.blockWriter.WriteFile(r)
	if err != nil {
		return err
	}

	if err := b.addNode(p, entry{
		name: path.Base(p),
	}); err != nil {
		return err
	}

	return nil
}

func (b *Builder) Symlink(p, dest string, options ...InodeOption) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.addNode(p, entry{
		name: path.Base(p),
	}); err != nil {
		return err
	}

	return nil
}

type childNode interface {
	Name() string
	AsDir() *dirNode
}

type entry struct {
	name     string
	metadata uint32
	typ      uint16
}

func (e entry) Name() string {
	return e.name
}

func (e entry) AsDir() *dirNode {
	return nil
}

type dirNode struct {
	entry
	inode    uint64
	children []childNode
}

func (d *dirNode) AsDir() *dirNode {
	return d
}

func (b *Builder) getParent(n *dirNode, path string) *dirNode {
	first, rest := splitPath(path)

	if first == "" {
		return n
	}

	p := n.insertSortedNode(&dirNode{
		entry: entry{
			name: first,
		},
	})

	d := p.AsDir()

	if d == nil {
		return nil
	}

	if rest != "" {
		return b.getParent(d, rest)
	}

	return d
}

func (n *dirNode) insertSortedNode(i childNode) childNode {
	pos, exists := slices.BinarySearchFunc(n.children, i, func(a, b childNode) int {
		return strings.Compare(a.Name(), b.Name())
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
	uncompressed memio.LimitedBuffer
	compressed   memio.LimitedBuffer
	compressor   compressedWriter
}

func newBlockWriter(w io.WriterAt, start int64, blockSize uint32, compressor compressedWriter) blockWriter {
	return blockWriter{
		w:            io.NewOffsetWriter(w, start),
		uncompressed: make(memio.LimitedBuffer, blockSize),
		compressed:   make(memio.LimitedBuffer, 0, blockSize),
		compressor:   compressor,
	}
}

func newMetadataWriter(w io.WriterAt, start int64, compressor compressedWriter) blockWriter {
	return blockWriter{
		w:            io.NewOffsetWriter(w, start),
		uncompressed: make(memio.LimitedBuffer, 0, blockSize),
		compressed:   make(memio.LimitedBuffer, 0, blockSize),
		compressor:   compressor,
	}
}

func (b *blockWriter) Pos() int64 {
	pos, _ := b.w.Seek(0, io.SeekCurrent)

	return pos
}

func (b *blockWriter) WriteFile(r io.Reader) ([]uint32, error) {
	var sizes []uint32

	for {
		if _, err := io.ReadFull(r, b.uncompressed); errors.Is(err, io.EOF) {
			break
		} else if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, err
		}

		c := b.compressed

		b.compressor.Reset(&c)

		n, err := b.w.Write(b.compressedOrUncompressed())
		if err != nil {
			return nil, err
		}

		sizes = append(sizes, uint32(n))
	}

	return sizes, nil
}

func (b *blockWriter) compressedOrUncompressed() memio.LimitedBuffer {
	if _, err := b.compressor.Write(b.uncompressed); !errors.Is(err, io.ErrShortWrite) {
		return b.compressed
	}

	return b.uncompressed
}

func (b *blockWriter) WriteMetadata(data []byte) error {
	for len(data) > 0 {
		n, _ := b.uncompressed.Write(data)

		data = data[n:]

		if len(b.uncompressed) != cap(b.uncompressed) {
			continue
		}

		if err := b.Flush(); err != nil {
			return err
		}
	}

	return nil
}

func (b *blockWriter) Flush() error {
	lew := byteio.LittleEndianWriter{Writer: b.w}
	data := b.compressedOrUncompressed()
	header := uint16(len(data))

	if header == 0 {
		return nil
	}

	if &data[0] == &b.uncompressed[0] {
		header |= metadataBlockCompressedMask
	}

	b.uncompressed = b.uncompressed[:0]
	b.compressed = b.compressed[:0]

	if _, err := lew.WriteUint16(header); err != nil {
		return err
	}

	if _, err := b.w.Write(data); err != nil {
		return err
	}

	return nil
}
