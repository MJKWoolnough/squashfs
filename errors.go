package squashfs

import "errors"

var (
	ErrInvalidCompressor            = errors.New("invalid or unknown compressor")
	ErrInvalidCompressionLevel      = errors.New("invalid compression level")
	ErrInvalidWindowSize            = errors.New("invalid window size")
	ErrInvalidCompressionStrategies = errors.New("invalid compression strategies")
	ErrNoCompressorOptions          = errors.New("no compressor options should be supplied")
	ErrInvalidCompressionAlgorithm  = errors.New("invalid compression algorithm")
	ErrInvalidDictionarySize        = errors.New("invalid dictionary size")
	ErrInvalidFilters               = errors.New("invalid filters")
	ErrInvalidCompressorVersion     = errors.New("invalid compressor version")
	ErrInvalidCompressorFlags       = errors.New("invalid compressor flags")
	ErrUnsupportedCompressor        = errors.New("unsupported compressor")

	ErrInvalidPointer     = errors.New("invalid pointer")
	ErrInvalidBlockHeader = errors.New("invalid block header")
	ErrUnsupportedSeek    = errors.New("unsupported seek")

	ErrInvalidMagicNumber = errors.New("invalid magic number")
	ErrInvalidBlockSize   = errors.New("invalid block size")
	ErrInvalidVersion     = errors.New("invalid version")
)
