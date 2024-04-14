package squashfs

import (
	"vimagination.zapto.org/byteio"
	"vimagination.zapto.org/errors"
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

type CompressorOptions any

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

func parseGZipOptions(ler *byteio.StickyLittleEndianReader) (*GZipOptions, error) {
	compressionlevel := ler.ReadUint32()
	if compressionlevel == 0 || compressionlevel > 9 {
		return nil, ErrInvalidCompressionLevel
	}

	windowsize := ler.ReadUint16()
	if windowsize < 8 || windowsize > 15 {
		return nil, ErrInvalidWindowSize
	}

	strategies := ler.ReadUint16()
	if strategies > 31 {
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
		CompressionLevel: 9,
		WindowSize:       15,
	}
}

func parseLZOOptions(ler *byteio.StickyLittleEndianReader) (any, error) {
	return nil, nil
}

func defaultLZOOptions() any {
	return nil
}

func parseXZOptions(ler *byteio.StickyLittleEndianReader) (any, error) {
	return nil, nil
}

func defaultXZOptions() any {
	return nil
}

func parseLZ4Options(ler *byteio.StickyLittleEndianReader) (any, error) {
	return nil, nil
}

func parseZStdOptions(ler *byteio.StickyLittleEndianReader) (any, error) {
	return nil, nil
}

func defaultZStdOptions() any {
	return nil
}

const (
	CompressorGZIP Compressor = 1
	CompressorLZMA Compressor = 2
	CompressorLZO  Compressor = 3
	CompressorXZ   Compressor = 4
	CompressorLZ4  Compressor = 5
	CompressorZSTD Compressor = 6
)

const (
	ErrInvalidCompressionLevel      = errors.Error("invalid compression level")
	ErrInvalidWindowSize            = errors.Error("invalid window size")
	ErrInvalidCompressionStrategies = errors.Error("invalid compression strategies")
	ErrNoCompressorOptions          = errors.Error("no compressor options should be supplied")
)
