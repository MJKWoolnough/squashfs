package squashfs

import (
	"compress/zlib"
	"fmt"
	"io"
	"math/bits"

	"vimagination.zapto.org/byteio"
)

type Compressor uint16

const (
	CompressorGZIP Compressor = 1
	CompressorLZMA Compressor = 2
	CompressorLZO  Compressor = 3
	CompressorXZ   Compressor = 4
	CompressorLZ4  Compressor = 5
	CompressorZSTD Compressor = 6
)

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

type CompressorOptions interface {
	makeWriter(io.Writer) (io.WriteCloser, error)
	asCompressor() Compressor
}

func (c Compressor) parseOptions(hasOptionsFlag bool, ler *byteio.StickyLittleEndianReader) (CompressorOptions, error) {
	switch c {
	case CompressorGZIP:
		if hasOptionsFlag {
			return parseGZipOptions(ler)
		} else {
			return defaultGzipOptions(), nil
		}
	case CompressorLZMA:
		return nil, ErrNoCompressorOptions
	case CompressorLZO:
		if hasOptionsFlag {
			return parseLZOOptions(ler)
		} else {
			return defaultLZOOptions(), nil
		}
	case CompressorXZ:
		if hasOptionsFlag {
			return parseXZOptions(ler)
		} else {
			return defaultXZOptions(), nil
		}
	case CompressorLZ4:
		return parseLZ4Options(ler)
	case CompressorZSTD:
		if hasOptionsFlag {
			return parseZStdOptions(ler)
		} else {
			return defaultZStdOptions(), nil
		}
	}

	return nil, ErrInvalidCompressor
}

type GZipOptions struct {
	CompressionLevel uint32
	WindowSize       uint16
	Strategies       uint16
}

const (
	minimumWindowSize = 8
	maximumWindowSize = 15
)

func parseGZipOptions(ler *byteio.StickyLittleEndianReader) (*GZipOptions, error) {
	compressionlevel := ler.ReadUint32()
	if compressionlevel == 0 || compressionlevel > zlib.BestCompression {
		return nil, ErrInvalidCompressionLevel
	}

	windowsize := ler.ReadUint16()
	if windowsize < minimumWindowSize || windowsize > maximumWindowSize {
		return nil, ErrInvalidWindowSize
	}

	const maxStrategy = 21

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

func defaultGzipOptions() *GZipOptions {
	return &GZipOptions{
		CompressionLevel: zlib.BestCompression,
		WindowSize:       maximumWindowSize,
	}
}

func (g *GZipOptions) makeWriter(w io.Writer) (io.WriteCloser, error) {
	return zlib.NewWriterLevel(w, int(g.CompressionLevel))
}

func (GZipOptions) asCompressor() Compressor {
	return CompressorGZIP
}

type LZOOptions struct {
	Algorithm        uint32
	CompressionLevel uint32
}

func parseLZOOptions(ler *byteio.StickyLittleEndianReader) (*LZOOptions, error) {
	const maxAlgorithm = 4

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

func defaultLZOOptions() *LZOOptions {
	const (
		lzoDefaultAlgorithm        = 4
		lzoDefaultCompressionLevel = 8
	)

	return &LZOOptions{
		Algorithm:        lzoDefaultAlgorithm,
		CompressionLevel: lzoDefaultCompressionLevel,
	}
}

func (LZOOptions) makeWriter(w io.Writer) (io.WriteCloser, error) {
	return nil, ErrUnsupportedCompressor
}

func (LZOOptions) asCompressor() Compressor {
	return CompressorLZO
}

type XZOptions struct {
	DictionarySize uint32
	Filters        uint32
}

const maxDictionarySize = 8192

func parseXZOptions(ler *byteio.StickyLittleEndianReader) (*XZOptions, error) {
	dictionarysize := ler.ReadUint32()
	if lead, trail := bits.LeadingZeros32(dictionarysize), bits.TrailingZeros32(dictionarysize); dictionarysize < maxDictionarySize || 32-trail-lead > 2 {
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

func defaultXZOptions() *XZOptions {
	return &XZOptions{
		DictionarySize: maxDictionarySize,
	}
}

func (XZOptions) makeWriter(w io.Writer) (io.WriteCloser, error) {
	return nil, ErrUnsupportedCompressor
}

func (XZOptions) asCompressor() Compressor {
	return CompressorXZ
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

func (LZ4Options) makeWriter(w io.Writer) (io.WriteCloser, error) {
	return nil, ErrUnsupportedCompressor
}

func (LZ4Options) asCompressor() Compressor {
	return CompressorLZ4
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

func defaultZStdOptions() *ZStdOptions {
	return &ZStdOptions{
		CompressionLevel: zlib.BestCompression,
	}
}

func (ZStdOptions) makeWriter(w io.Writer) (io.WriteCloser, error) {
	return nil, ErrUnsupportedCompressor
}

func (ZStdOptions) asCompressor() Compressor {
	return CompressorZSTD
}
