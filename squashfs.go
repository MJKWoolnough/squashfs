// Package squashfs is a SquashFS reader and writer using fs.FS
package squashfs // import "vimagination.zapto.org/squashfs"

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
)

const defaultCacheSize = 1024

type SquashFS struct {
	superblock superblock
	reader     io.ReaderAt

	blockCache blockCache
}

func (s *SquashFS) Open(path string) (fs.File, error) {
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

func (s *SquashFS) open(path string) (fs.File, error) {
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

func (s *SquashFS) ReadFile(name string) ([]byte, error) {
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

func (s *SquashFS) readFile(name string) ([]byte, error) {
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

func (s *SquashFS) ReadDir(name string) ([]fs.DirEntry, error) {
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

func (s *SquashFS) readDir(name string) ([]fs.DirEntry, error) {
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

// Open reads the passed io.ReaderAt as a SquashFS image, returning a fs.FS
// implementation.
//
// The returned fs.FS, and any files opened from it will cease to work if the
// io.ReaderAt is closed.
func Open(r io.ReaderAt) (*SquashFS, error) {
	return OpenWithCacheSize(r, defaultCacheSize)
}

// OpenWithCacheSize acts like Open, but allows a custom cache size, which
// normally defaults to 1024.
func OpenWithCacheSize(r io.ReaderAt, cacheSize uint) (*SquashFS, error) {
	var sb superblock
	if err := sb.readFrom(io.NewSectionReader(r, 0, headerLength)); err != nil {
		return nil, fmt.Errorf("error reading superblock: %w", err)
	}

	return &SquashFS{
		superblock: sb,
		reader:     r,
		blockCache: newBlockCache(cacheSize),
	}, nil
}

func (s *SquashFS) Stat(path string) (fs.FileInfo, error) {
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

func (s *SquashFS) LStat(path string) (fs.FileInfo, error) {
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

func (s *SquashFS) Readlink(path string) (string, error) {
	fi, err := s.resolve(path, false)
	if err != nil {
		return "", &fs.PathError{
			Op:   "readlink",
			Path: path,
			Err:  err,
		}
	}

	sym, ok := fi.(symlinkStat)
	if !ok {
		return "", &fs.PathError{
			Op:   "readlink",
			Path: path,
			Err:  fs.ErrInvalid,
		}
	}

	return sym.targetPath, nil
}
