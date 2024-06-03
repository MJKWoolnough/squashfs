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
	"vimagination.zapto.org/rwcount"
)

const (
	padTo   = 1 << 12
	noTable = 0xffffffffffffffff
)

var zeroPad [1]byte

type Builder struct {
	writer     io.WriterAt
	superblock superblock

	defaultMode    fs.FileMode
	defaultOwner   uint32
	defaultGroup   uint32
	defaultModTime time.Time

	blockWriter    blockWriter
	inodeTable     metadataWriter
	fragmentBuffer memio.Buffer
	fragmentTable  metadataWriter
	idTable        metadataWriter

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
			ExportTable:        noTable,
		},
	}

	for _, o := range options {
		if err := o(b); err != nil {
			return nil, err
		}
	}

	b.fragmentBuffer = make(memio.Buffer, 0, b.superblock.BlockSize)

	blockStart := int64(headerLength)

	if b.superblock.Flags&flagCompressionOptions == 0 {
		blockStart -= compressionOptionsLength
	}

	c, err := b.superblock.CompressionOptions.getCompressedWriter()
	if err != nil {
		return nil, err
	}

	b.blockWriter = newBlockWriter(w, blockStart, b.superblock.BlockSize, c)

	for _, table := range [...]*metadataWriter{
		&b.inodeTable,
		&b.fragmentTable,
		&b.idTable,
	} {
		*table = newMetadataWriter(c)
	}

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

	d := &dirNode{
		entry: entry{
			name: path.Base(p),
		},
		commonStat: b.commonStat(options...),
	}

	for _, opt := range options {
		opt(&d.commonStat)
	}

	if err := b.addNode(p, d); err != nil {
		return err
	}

	return nil
}

func (b *Builder) commonStat(options ...InodeOption) commonStat {
	c := commonStat{
		perms: uint16(b.defaultMode),
		uid:   b.defaultOwner,
		gid:   b.defaultGroup,
		mtime: b.defaultModTime,
	}

	for _, opt := range options {
		opt(&c)
	}

	return c
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

	start := uint64(b.blockWriter.Pos())

	sr := rwcount.Reader{Reader: r}

	sizes, err := b.blockWriter.WriteFile(&sr)
	if err != nil {
		return err
	}

	if err := b.addNode(p, entry{
		name:     path.Base(p),
		metadata: uint32(b.inodeTable.Pos()),
	}); err != nil {
		return err
	}

	fragIndex, blockOffset, err := b.writePossibleFragment(sr.Count)
	if err != nil {
		return err
	}

	return b.writeInode(fileStat{
		commonStat:  b.commonStat(options...),
		blocksStart: start,
		fileSize:    uint64(sr.Count),
		blockSizes:  sizes,
		fragIndex:   fragIndex,
		blockOffset: blockOffset,
	})
}

type inodeWriter interface {
	writeTo(*byteio.StickyLittleEndianWriter)
}

func (b *Builder) writeInode(inode inodeWriter) error {
	b.superblock.Inodes++

	lew := byteio.StickyLittleEndianWriter{Writer: &b.inodeTable}

	inode.writeTo(&lew)

	return lew.Err
}

func (b *Builder) writePossibleFragment(totalSize int64) (uint32, uint32, error) {
	fragmentLength := uint64(totalSize) % uint64(b.superblock.BlockSize)

	if fragmentLength == 0 {
		return fieldDisabled, 0, nil
	}

	fragment := b.blockWriter.uncompressed[:fragmentLength]

	if len(fragment) > cap(b.fragmentBuffer)-len(b.fragmentBuffer) {
		if err := b.writeFragments(); err != nil {
			return 0, 0, err
		}
	}

	fragIndex := uint32(b.fragmentTable.Pos())
	blockOffset := uint32(len(b.fragmentBuffer))

	b.fragmentBuffer = append(b.fragmentBuffer, fragment...)

	return fragIndex, blockOffset, nil
}

func (b *Builder) writeFragments() error {
	fragPos := uint64(b.blockWriter.Pos())

	n, err := b.blockWriter.WriteFragments(b.fragmentBuffer)
	if err != nil {
		return err
	}

	lew := byteio.LittleEndianWriter{Writer: &b.fragmentTable}
	if _, err := lew.WriteUint64(fragPos); err != nil {
		return err
	}

	if _, err := lew.WriteUint32(uint32(n)); err != nil {
		return err
	}

	if _, err := lew.WriteUint32(0); err != nil {
		return err
	}

	b.fragmentBuffer = b.fragmentBuffer[:0]
	b.superblock.FragCount++

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

	return b.writeInode(symlinkStat{
		commonStat: b.commonStat(options...),
		linkCount:  1,
		targetPath: dest,
	})
}

func (b *Builder) Close() error {
	if err := b.writeFragments(); err != nil {
		return err
	}

	dirTable := newMetadataWriter(b.blockWriter.compressor)

	b.walkTree(&dirTable)

	t := tableWriter{
		w:   b.writer,
		pos: b.blockWriter.Pos(),
	}

	t.WriteTable(&b.superblock.IDTable, b.idTable.buf)
	t.WriteTable(&b.superblock.DirTable, dirTable.buf)
	t.WriteTable(&b.superblock.FragTable, b.fragmentTable.buf)
	t.WriteTable(&b.superblock.IDTable, b.idTable.buf)

	if t.err != nil {
		return t.err
	}

	if diff := t.pos % padTo; diff != 0 {
		if _, err := b.writer.WriteAt(zeroPad[:], t.pos+diff); err != nil {
			return err
		}
	}

	return b.superblock.writeTo(io.NewOffsetWriter(b.writer, 0))
}

type tableWriter struct {
	w   io.WriterAt
	pos int64
	err error
}

func (t *tableWriter) WriteTable(tablePos *uint64, p []byte) {
	if t.err != nil {
		return
	}

	if len(p) == 0 {
		*tablePos = noTable

		return
	}

	*tablePos = uint64(t.pos)

	n, err := t.w.WriteAt(p, t.pos)

	t.pos += int64(n)

	if err != nil {
		t.err = err
	}
}

func (b *Builder) walkTree(_dirTable *metadataWriter) error {
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
	commonStat commonStat
	inode      uint64
	children   []childNode
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
	ow := io.NewOffsetWriter(w, 0)

	ow.Seek(start, io.SeekStart)

	return blockWriter{
		w:            ow,
		uncompressed: make(memio.LimitedBuffer, blockSize),
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
		if _, err := io.ReadFull(r, b.uncompressed); errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return sizes, nil
		} else if err != nil {
			return nil, err
		}

		n, err := b.w.Write(b.compressIfSmaller(b.uncompressed))
		if err != nil {
			return nil, err
		}

		sizes = append(sizes, uint32(n))
	}
}

func (b *blockWriter) WriteFragments(fragments []byte) (int, error) {
	return b.w.Write(b.compressIfSmaller(fragments))
}

func (b *blockWriter) compressIfSmaller(data []byte) []byte {
	c := b.compressed

	b.compressor.Reset(&c)

	if _, err := b.compressor.Write(data); !errors.Is(err, io.ErrShortWrite) {
		return c
	}

	return data
}

type metadataWriter struct {
	buf          memio.Buffer
	uncompressed memio.LimitedBuffer
	compressed   memio.LimitedBuffer
	compressor   compressedWriter
}

func newMetadataWriter(compressor compressedWriter) metadataWriter {
	return metadataWriter{
		uncompressed: make(memio.LimitedBuffer, 0, blockSize),
		compressed:   make(memio.LimitedBuffer, 0, blockSize),
		compressor:   compressor,
	}
}

func (m *metadataWriter) Pos() int {
	return len(m.buf)<<16 | len(m.uncompressed)
}

func (m *metadataWriter) Write(data []byte) (int, error) {
	l := len(data)

	for len(data) > 0 {
		n, _ := m.uncompressed.Write(data)

		data = data[n:]

		if len(m.uncompressed) != cap(m.uncompressed) {
			continue
		}

		if err := m.Flush(); err != nil {
			return l, err
		}
	}

	return l, nil
}

func (m *metadataWriter) Flush() error {
	lew := byteio.LittleEndianWriter{Writer: &m.buf}
	data := m.compressedOrUncompressed()
	header := uint16(len(data))

	if header == 0 {
		return nil
	}

	if &data[0] == &m.uncompressed[0] {
		header |= metadataBlockCompressedMask
	}

	m.uncompressed = m.uncompressed[:0]
	m.compressed = m.compressed[:0]

	if _, err := lew.WriteUint16(header); err != nil {
		return err
	}

	if _, err := m.buf.Write(data); err != nil {
		return err
	}

	return nil
}

func (m *metadataWriter) compressedOrUncompressed() memio.LimitedBuffer {
	c := m.compressed

	m.compressor.Reset(&c)

	if _, err := m.compressor.Write(m.uncompressed); !errors.Is(err, io.ErrShortWrite) {
		return c
	}

	return m.uncompressed
}
