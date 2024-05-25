package squashfs

import (
	"compress/zlib"
	"fmt"
	"io"
	"math/bits"

	"vimagination.zapto.org/byteio"
)

const (
	CompressorGZIP Compressor = 1
	CompressorLZMA Compressor = 2
	CompressorLZO  Compressor = 3
	CompressorXZ   Compressor = 4
	CompressorLZ4  Compressor = 5
	CompressorZSTD Compressor = 6

	minimumWindowSize = 8
	maximumWindowSize = 15

	maxStrategy = 21

	maxAlgorithm = 4

	lzoDefaultAlgorithm        = 4
	lzoDefaultCompressionLevel = 8

	maxDictionarySize = 8192
)

type Compressor uint16

func (c Compressor) String() string {
	switch c {
	case CompressorGZIP:
		return "gzip"
	case CompressorLZMA:
		return "lzma"
	case CompressorLZO:
		return "lzo"
	case CompressorXZ:
		return "xz"
	case CompressorLZ4:
		return "lz4"
	case CompressorZSTD:
		return "zstd"
	}

	return "unknown"
}

func (c Compressor) decompress(r io.Reader) (io.Reader, error) {
	switch c {
	case CompressorGZIP:
		return zlib.NewReader(r)
	default:
		return nil, fmt.Errorf("%s: %w", c, ErrUnsupportedCompressor)
	}
}

type compressedWriter interface {
	io.Writer
	Reset(io.Writer)
	Flush() error
}

type CompressorOptions interface {
	getCompressedWriter() (compressedWriter, error)
	asCompressor() Compressor
	isDefault() bool
	writeTo(*byteio.StickyLittleEndianWriter)
}

func (c Compressor) parseOptions(hasOptionsFlag bool, ler *byteio.StickyLittleEndianReader) (CompressorOptions, error) {
	switch c {
	case CompressorGZIP:
		if hasOptionsFlag {
			return parseGZipOptions(ler)
		} else {
			return DefaultGzipOptions(), nil
		}
	case CompressorLZMA:
		return nil, ErrNoCompressorOptions
	case CompressorLZO:
		if hasOptionsFlag {
			return parseLZOOptions(ler)
		} else {
			return DefaultLZOOptions(), nil
		}
	case CompressorXZ:
		if hasOptionsFlag {
			return parseXZOptions(ler)
		} else {
			return DefaultXZOptions(), nil
		}
	case CompressorLZ4:
		return parseLZ4Options(ler)
	case CompressorZSTD:
		if hasOptionsFlag {
			return parseZStdOptions(ler)
		} else {
			return DefaultZStdOptions(), nil
		}
	}

	return nil, ErrInvalidCompressor
}

type GZipOptions struct {
	CompressionLevel uint32
	WindowSize       uint16
	Strategies       uint16
}

func parseGZipOptions(ler *byteio.StickyLittleEndianReader) (*GZipOptions, error) {
	compressionlevel := ler.ReadUint32()
	if compressionlevel == 0 || compressionlevel > zlib.BestCompression {
		return nil, ErrInvalidCompressionLevel
	}

	windowsize := ler.ReadUint16()
	if windowsize < minimumWindowSize || windowsize > maximumWindowSize {
		return nil, ErrInvalidWindowSize
	}

	strategies := ler.ReadUint16()
	if strategies > maxStrategy {
		return nil, ErrInvalidCompressionStrategies
	}

	return &GZipOptions{
		CompressionLevel: compressionlevel,
		WindowSize:       windowsize,
		Strategies:       strategies,
	}, nil
}

func DefaultGzipOptions() *GZipOptions {
	return &GZipOptions{
		CompressionLevel: zlib.BestCompression,
		WindowSize:       maximumWindowSize,
	}
}

func (g *GZipOptions) getCompressedWriter() (compressedWriter, error) {
	return zlib.NewWriterLevel(nil, int(g.CompressionLevel))
}

func (GZipOptions) asCompressor() Compressor {
	return CompressorGZIP
}

func (g *GZipOptions) isDefault() bool {
	return g.CompressionLevel == zlib.BestCompression && g.WindowSize == maximumWindowSize
}

func (g *GZipOptions) writeTo(w *byteio.StickyLittleEndianWriter) {
	w.WriteUint32(g.CompressionLevel)
	w.WriteUint16(g.WindowSize)
	w.WriteUint16(g.Strategies)
}

type LZMAOptions struct{}

func DefaultLZMAOptions() LZMAOptions {
	return LZMAOptions{}
}

func (LZMAOptions) getCompressedWriter() (compressedWriter, error) {
	return nil, ErrUnsupportedCompressor
}

func (LZMAOptions) asCompressor() Compressor {
	return CompressorLZMA
}

func (LZMAOptions) isDefault() bool {
	return true
}

func (LZMAOptions) writeTo(_ *byteio.StickyLittleEndianWriter) {}

type LZOOptions struct {
	Algorithm        uint32
	CompressionLevel uint32
}

func parseLZOOptions(ler *byteio.StickyLittleEndianReader) (*LZOOptions, error) {
	algorithm := ler.ReadUint32()
	if algorithm > maxAlgorithm {
		return nil, ErrInvalidCompressionAlgorithm
	}

	compressionlevel := ler.ReadUint32()
	if compressionlevel > zlib.BestCompression || algorithm != 4 && compressionlevel != 0 {
		return nil, ErrInvalidCompressionLevel
	}

	return &LZOOptions{
		Algorithm:        algorithm,
		CompressionLevel: compressionlevel,
	}, nil
}

func DefaultLZOOptions() *LZOOptions {
	return &LZOOptions{
		Algorithm:        lzoDefaultAlgorithm,
		CompressionLevel: lzoDefaultCompressionLevel,
	}
}

func (l *LZOOptions) isDefault() bool {
	return l.CompressionLevel == lzoDefaultCompressionLevel && l.Algorithm == lzoDefaultAlgorithm
}

func (LZOOptions) getCompressedWriter() (compressedWriter, error) {
	return nil, ErrUnsupportedCompressor
}

func (LZOOptions) asCompressor() Compressor {
	return CompressorLZO
}

func (l *LZOOptions) writeTo(w *byteio.StickyLittleEndianWriter) {
	w.WriteUint32(l.Algorithm)
	w.WriteUint32(l.CompressionLevel)
}

type XZOptions struct {
	DictionarySize uint32
	Filters        uint32
}

func parseXZOptions(ler *byteio.StickyLittleEndianReader) (*XZOptions, error) {
	dictionarysize := ler.ReadUint32()

	lead, trail := bits.LeadingZeros32(dictionarysize), bits.TrailingZeros32(dictionarysize)
	if dictionarysize < maxDictionarySize || 32-trail-lead > 2 {
		return nil, ErrInvalidDictionarySize
	}

	const maxFilters = 63

	filters := ler.ReadUint32()
	if filters > maxFilters {
		return nil, ErrInvalidFilters
	}

	return &XZOptions{
		DictionarySize: dictionarysize,
		Filters:        filters,
	}, nil
}

func DefaultXZOptions() *XZOptions {
	return &XZOptions{
		DictionarySize: maxDictionarySize,
	}
}

func (XZOptions) getCompressedWriter() (compressedWriter, error) {
	return nil, ErrUnsupportedCompressor
}

func (XZOptions) asCompressor() Compressor {
	return CompressorXZ
}

func (x *XZOptions) isDefault() bool {
	return x.DictionarySize == maxDictionarySize && x.Filters == 0
}

func (x *XZOptions) writeTo(w *byteio.StickyLittleEndianWriter) {
	w.WriteUint32(x.DictionarySize)
	w.WriteUint32(x.Filters)
}

type LZ4Options struct {
	Version uint32
	Flags   uint32
}

func parseLZ4Options(ler *byteio.StickyLittleEndianReader) (*LZ4Options, error) {
	if ler.ReadUint32() != 1 {
		return nil, ErrInvalidCompressorVersion
	}

	flags := ler.ReadUint32()
	if flags > 1 {
		return nil, ErrInvalidCompressorFlags
	}

	return &LZ4Options{
		Version: 1,
		Flags:   flags,
	}, nil
}

func (LZ4Options) getCompressedWriter() (compressedWriter, error) {
	return nil, ErrUnsupportedCompressor
}

func (LZ4Options) asCompressor() Compressor {
	return CompressorLZ4
}

func (LZ4Options) isDefault() bool {
	return false
}

func (l *LZ4Options) writeTo(w *byteio.StickyLittleEndianWriter) {
	w.WriteUint32(l.Version)
	w.WriteUint32(l.Flags)
}

type ZStdOptions struct {
	CompressionLevel uint32
}

func parseZStdOptions(ler *byteio.StickyLittleEndianReader) (*ZStdOptions, error) {
	const maxZStdCompressionLevel = 22

	compressionlevel := ler.ReadUint32()
	if compressionlevel == 0 || compressionlevel > maxZStdCompressionLevel {
		return nil, ErrInvalidCompressionLevel
	}

	return &ZStdOptions{
		CompressionLevel: compressionlevel,
	}, nil
}

func DefaultZStdOptions() *ZStdOptions {
	return &ZStdOptions{
		CompressionLevel: zlib.BestCompression,
	}
}

func (ZStdOptions) getCompressedWriter() (compressedWriter, error) {
	return nil, ErrUnsupportedCompressor
}

func (ZStdOptions) asCompressor() Compressor {
	return CompressorZSTD
}

func (z *ZStdOptions) isDefault() bool {
	return z.CompressionLevel == zlib.BestCompression
}

func (z *ZStdOptions) writeTo(w *byteio.StickyLittleEndianWriter) {
	w.WriteUint32(z.CompressionLevel)
}
