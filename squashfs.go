// Package squashfs is a SquashFS reader and writer using fs.FS
package squashfs // import "vimagination.zapto.org/squashfs"

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
)

const defaultCacheSize = 1024

type squashfs struct {
	superblock superblock
	reader     io.ReaderAt

	blockCache blockCache
}

func (s *squashfs) Open(path string) (fs.File, error) {
	f, err := s.open(path)
	if err != nil {
		return nil, &fs.PathError{
			Op:   "open",
			Path: path,
			Err:  err,
		}
	}

	return f, nil
}

func (s *squashfs) open(path string) (fs.File, error) {
	f, err := s.resolve(path, true)
	if err != nil {
		return nil, err
	}

	switch f := f.(type) {
	case fileStat:
		return &file{
			squashfs: s,
			file:     f,
		}, nil
	case dirStat:
		return s.newDir(f)
	}

	return nil, fs.ErrInvalid
}

func (s *squashfs) ReadFile(name string) ([]byte, error) {
	d, err := s.readFile(name)
	if err != nil {
		return nil, &fs.PathError{
			Op:   "readfile",
			Path: name,
			Err:  err,
		}
	}

	return d, nil
}

func (s *squashfs) readFile(name string) ([]byte, error) {
	f, err := s.Open(name)
	if err != nil {
		return nil, err
	}

	ff, ok := f.(*file)
	if !ok {
		return nil, fs.ErrInvalid
	}

	buf := make([]byte, ff.file.fileSize)

	if _, err = ff.read(buf); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}

	return buf, nil
}

func (s *squashfs) ReadDir(name string) ([]fs.DirEntry, error) {
	de, err := s.readDir(name)
	if err != nil {
		return nil, &fs.PathError{
			Op:   "readdir",
			Path: name,
			Err:  err,
		}
	}

	return de, nil
}

func (s *squashfs) readDir(name string) ([]fs.DirEntry, error) {
	d, err := s.open(name)
	if err != nil {
		return nil, err
	}

	dd, ok := d.(*dir)
	if !ok {
		return nil, fs.ErrInvalid
	}

	return dd.ReadDir(-1)
}

type FS interface {
	fs.ReadFileFS
	fs.ReadDirFS
	fs.StatFS

	// Lstat returns a FileInfo describing the named file. If the file is a
	// symbolic link, the returned FileInfo describes the symbolic link.
	LStat(name string) (fs.FileInfo, error)
}

// Open reads the passed io.ReaderAt as a SquashFS image, returning a fs.FS
// implementation.
//
// The returned fs.FS, and any files opened from it will cease to work if the
// io.ReaderAt is closed.
func Open(r io.ReaderAt) (FS, error) {
	return OpenWithCacheSize(r, defaultCacheSize)
}

// OpenWithCacheSize acts like Open, but allows a custom cache size, which
// normally defaults to 1024.
func OpenWithCacheSize(r io.ReaderAt, cacheSize uint) (FS, error) {
	var sb superblock
	if err := sb.readFrom(io.NewSectionReader(r, 0, headerLength)); err != nil {
		return nil, fmt.Errorf("error reading superblock: %w", err)
	}

	return &squashfs{
		superblock: sb,
		reader:     r,
		blockCache: newBlockCache(cacheSize),
	}, nil
}

func (s *squashfs) Stat(path string) (fs.FileInfo, error) {
	fi, err := s.resolve(path, true)
	if err != nil {
		return nil, &fs.PathError{
			Op:   "stat",
			Path: path,
			Err:  err,
		}
	}

	return fi, nil
}

func (s *squashfs) LStat(path string) (fs.FileInfo, error) {
	fi, err := s.resolve(path, false)
	if err != nil {
		return nil, &fs.PathError{
			Op:   "lstat",
			Path: path,
			Err:  err,
		}
	}

	return fi, nil
}
