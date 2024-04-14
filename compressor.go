package squashfs

import "vimagination.zapto.org/byteio"

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
			return defaultGzipOptions()
		}
	case CompressorLZMA:
		return parseLZMAOptions(ler)
	case CompressorLZO:
		if hasOptionsFlag {
			return parseLZOOptions(ler)
		} else {
			return defaultLZOOptions()
		}
	case CompressorXZ:
		if hasOptionsFlag {
			return parseXZOptions(ler)
		} else {
			return defaultXZOptions()
		}
	case CompressorLZ4:
		return parseLZ4Options(ler)
	case CompressorZSTD:
		if hasOptionsFlag {
			return parseZStdOptions(ler)
		} else {
			return defaultZStdOptions()
		}
	}

	return nil, ErrInvalidCompressor
}

func parseGZipOptions(ler *byteio.StickyLittleEndianReader) (any, error) {
	return nil, nil
}

func defaultGzipOptions() (any, error) {
	return nil, nil
}

func parseLZMAOptions(ler *byteio.StickyLittleEndianReader) (any, error) {
	return nil, nil
}

func parseLZOOptions(ler *byteio.StickyLittleEndianReader) (any, error) {
	return nil, nil
}

func defaultLZOOptions() (any, error) {
	return nil, nil
}

func parseXZOptions(ler *byteio.StickyLittleEndianReader) (any, error) {
	return nil, nil
}

func defaultXZOptions() (any, error) {
	return nil, nil
}

func parseLZ4Options(ler *byteio.StickyLittleEndianReader) (any, error) {
	return nil, nil
}

func parseZStdOptions(ler *byteio.StickyLittleEndianReader) (any, error) {
	return nil, nil
}

func defaultZStdOptions() (any, error) {
	return nil, nil
}

const (
	CompressorGZIP Compressor = 1
	CompressorLZMA Compressor = 2
	CompressorLZO  Compressor = 3
	CompressorXZ   Compressor = 4
	CompressorLZ4  Compressor = 5
	CompressorZSTD Compressor = 6
)
