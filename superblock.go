package squashfs

import (
	"bytes"
	"fmt"
	"io"
	"math/bits"
	"time"

	"vimagination.zapto.org/byteio"
)

const (
	headerLength           = 104
	magic                  = 0x73717368 // hsqs
	versionMajor           = 4
	versionMinor           = 0
	flagCompressionOptions = 0x400
)

type superblock struct {
	Stats
	IDCount            uint16
	RootInode          uint64
	IDTable            uint64
	XattrTable         uint64
	InodeTable         uint64
	DirTable           uint64
	FragTable          uint64
	ExportTable        uint64
	CompressionOptions CompressorOptions
}

func (s *superblock) readFrom(r io.Reader) error {
	var buf [headerLength]byte

	_, err := io.ReadFull(r, buf[:])
	if err != nil {
		return err
	}

	ler := byteio.StickyLittleEndianReader{Reader: bytes.NewBuffer(buf[:])}

	if ler.ReadUint32() != magic {
		return ErrInvalidMagicNumber
	}

	if err = s.readSuperBlockDetails(&ler); err != nil {
		return err
	}

	s.CompressionOptions, err = s.Compressor.parseOptions(s.Flags&flagCompressionOptions != 0, &ler)

	return err
}

func (s *superblock) readSuperBlockDetails(ler *byteio.StickyLittleEndianReader) error {
	s.Inodes = ler.ReadUint32()
	s.ModTime = time.Unix(int64(ler.ReadUint32()), 0)
	s.BlockSize = ler.ReadUint32()
	s.FragCount = ler.ReadUint32()
	s.Compressor = Compressor(ler.ReadUint16())

	if 1<<ler.ReadUint16() != s.BlockSize {
		return ErrInvalidBlockSize
	}

	s.Flags = ler.ReadUint16()
	s.IDCount = ler.ReadUint16()

	if ler.ReadUint16() != versionMajor || ler.ReadUint16() != versionMinor {
		return ErrInvalidVersion
	}

	s.RootInode = ler.ReadUint64()
	s.BytesUsed = ler.ReadUint64()
	s.IDTable = ler.ReadUint64()
	s.XattrTable = ler.ReadUint64()
	s.InodeTable = ler.ReadUint64()
	s.DirTable = ler.ReadUint64()
	s.FragTable = ler.ReadUint64()
	s.ExportTable = ler.ReadUint64()

	return nil
}

func (s *superblock) writeTo(w io.Writer) error {
	if s.ModTime.IsZero() {
		s.ModTime = time.Now()
	}

	lew := byteio.StickyLittleEndianWriter{Writer: w}

	lew.WriteUint32(magic)
	lew.WriteUint32(s.Inodes)
	lew.WriteUint32(uint32(s.ModTime.Unix()))
	lew.WriteUint32(s.BlockSize)
	lew.WriteUint32(s.FragCount)
	lew.WriteUint16(uint16(s.Compressor))
	lew.WriteUint16(uint16(bits.TrailingZeros32(s.BlockSize)))
	lew.WriteUint16(s.Flags)
	lew.WriteUint16(s.IDCount)
	lew.WriteUint16(versionMajor)
	lew.WriteUint16(versionMinor)
	lew.WriteUint64(s.RootInode)
	lew.WriteUint64(s.BytesUsed)
	lew.WriteUint64(s.IDTable)
	lew.WriteUint64(s.XattrTable)
	lew.WriteUint64(s.InodeTable)
	lew.WriteUint64(s.DirTable)
	lew.WriteUint64(s.FragTable)
	lew.WriteUint64(s.ExportTable)

	s.CompressionOptions.writeTo(&lew)

	return lew.Err
}

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
	var sb superblock
	if err := sb.readFrom(r); err != nil {
		return nil, fmt.Errorf("error reading superblock: %w", err)
	}

	return &sb.Stats, nil
}
