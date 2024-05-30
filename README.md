# squashfs
--
    import "vimagination.zapto.org/squashfs"

Package squashfs is a SquashFS reader and writer using fs.FS

## Usage

```go
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

	ErrInvalidMagicNumber = errors.New("invalid magic number")
	ErrInvalidBlockSize   = errors.New("invalid block size")
	ErrInvalidVersion     = errors.New("invalid version")
)
```

#### type Compressor

```go
type Compressor uint16
```


```go
const (
	CompressorGZIP Compressor = 1
	CompressorLZMA Compressor = 2
	CompressorLZO  Compressor = 3
	CompressorXZ   Compressor = 4
	CompressorLZ4  Compressor = 5
	CompressorZSTD Compressor = 6
)
```

#### func (Compressor) String

```go
func (c Compressor) String() string
```

#### type CompressorOptions

```go
type CompressorOptions interface {
	// contains filtered or unexported methods
}
```


#### type GZipOptions

```go
type GZipOptions struct {
	CompressionLevel uint32
	WindowSize       uint16
	Strategies       uint16
}
```


#### func  DefaultGzipOptions

```go
func DefaultGzipOptions() *GZipOptions
```

#### type LZ4Options

```go
type LZ4Options struct {
	Version uint32
	Flags   uint32
}
```


#### type LZMAOptions

```go
type LZMAOptions struct{}
```


#### func  DefaultLZMAOptions

```go
func DefaultLZMAOptions() LZMAOptions
```

#### type LZOOptions

```go
type LZOOptions struct {
	Algorithm        uint32
	CompressionLevel uint32
}
```


#### func  DefaultLZOOptions

```go
func DefaultLZOOptions() *LZOOptions
```

#### type SquashFS

```go
type SquashFS struct {
}
```

The SquashFS type implements many of the FS interfaces, such as: fs.FS
fs.ReadFileFS fs.ReadDirFS fs.StatFS

and has additional methods for dealing with symlinks.

#### func  Open

```go
func Open(r io.ReaderAt) (*SquashFS, error)
```
Open reads the passed io.ReaderAt as a SquashFS image, returning a fs.FS
implementation.

The returned fs.FS, and any files opened from it will cease to work if the
io.ReaderAt is closed.

#### func  OpenWithCacheSize

```go
func OpenWithCacheSize(r io.ReaderAt, cacheSize int) (*SquashFS, error)
```
OpenWithCacheSize acts like Open, but allows a custom cache size, which normally
defaults to 16MB.

#### func (*SquashFS) LStat

```go
func (s *SquashFS) LStat(path string) (fs.FileInfo, error)
```
Lstat returns a FileInfo describing the named file. If the file is a symbolic
link, the returned FileInfo describes the symbolic link.

#### func (*SquashFS) Open

```go
func (s *SquashFS) Open(path string) (fs.File, error)
```
Open opens the named file for reading.

#### func (*SquashFS) ReadDir

```go
func (s *SquashFS) ReadDir(name string) ([]fs.DirEntry, error)
```
ReadDir returns a sorted list of directory entries for the named directory.

#### func (*SquashFS) ReadFile

```go
func (s *SquashFS) ReadFile(name string) ([]byte, error)
```
ReadFile return the byte contents of the named file.

#### func (*SquashFS) Readlink

```go
func (s *SquashFS) Readlink(path string) (string, error)
```
Readlink returns the destination of the named symbolic link.

#### func (*SquashFS) Stat

```go
func (s *SquashFS) Stat(path string) (fs.FileInfo, error)
```
Stat returns a FileInfo describing the name file.

#### type Stats

```go
type Stats struct {
	Inodes     uint32
	ModTime    time.Time
	BlockSize  uint32
	FragCount  uint32
	Compressor Compressor
	Flags      uint16
	BytesUsed  uint64
}
```

Type Stats contains basic data about the SquashFS file, read from the
superblock.

#### func  ReadStats

```go
func ReadStats(r io.Reader) (*Stats, error)
```
ReadStats reads the superblock from the passed reader and returns useful stats.

#### type XZOptions

```go
type XZOptions struct {
	DictionarySize uint32
	Filters        uint32
}
```


#### func  DefaultXZOptions

```go
func DefaultXZOptions() *XZOptions
```

#### type ZStdOptions

```go
type ZStdOptions struct {
	CompressionLevel uint32
}
```


#### func  DefaultZStdOptions

```go
func DefaultZStdOptions() *ZStdOptions
```
