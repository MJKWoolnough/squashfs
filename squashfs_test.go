package squashfs

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

var (
	contentsA = "my contents"
	contentsB = strings.Repeat("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"+contentsA, 1024)
	contentsC = strings.Repeat("ABCDEFGHIJKLMNOP", 8192)
	contentsD = strings.Repeat("ZYXWVUTSRQPONMLK", 16384)
	contentsE = contentsA + contentsD

	timestamp = time.Unix(1234567, 0)
)

type testFn func(FS) error

func test(t *testing.T, tests []testFn, children ...child) {
	t.Helper()

	sqfs, err := buildSquashFS(t, children...)
	if err != nil {
		t.Fatalf("unexpected error creating squashfs file: %s", err)
	}

	f, err := os.Open(sqfs)
	if err != nil {
		t.Fatalf("unexpected error opening squashfs file: %s", err)
	}
	defer f.Close()

	sfs, err := Open(f)
	if err != nil {
		t.Fatalf("unexpected error opening squashfs reader: %s", err)
	}

	if err := fstest.TestFS(sfs, ".required"); err != nil {
		t.Fatal(err)
	}

	for n, test := range tests {
		if err := test(sfs); err != nil {
			t.Errorf("test %d: %s", n+1, err)
		}
	}
}

func readSqfsFile(sfs fs.FS, path, expectation string) error {
	a, err := sfs.Open(path)
	if err != nil {
		return fmt.Errorf("unexpected error opening file in squashfs FS: %w", err)
	}

	contents, err := io.ReadAll(a)
	if err != nil {
		return fmt.Errorf("unexpected error reading file in squashfs FS: %w", err)
	}

	if string(contents) != expectation {
		return fmt.Errorf("expected to read %q, got %q", expectation, contents)
	}

	return nil
}

func TestOpenRead(t *testing.T) {
	test(
		t,
		[]testFn{
			func(sfs FS) error {
				return readSqfsFile(sfs, filepath.Join("dirA", "fileA"), contentsA)
			},
			func(sfs FS) error {
				return readSqfsFile(sfs, filepath.Join("dirA", "fileB"), contentsB)
			},
			func(sfs FS) error {
				return readSqfsFile(sfs, filepath.Join("dirA", "fileC"), contentsC)
			},
			func(sfs FS) error {
				return readSqfsFile(sfs, filepath.Join("dirA", "fileD"), contentsD)
			},
			func(sfs FS) error {
				return readSqfsFile(sfs, filepath.Join("dirA", "fileE"), contentsE)
			},
		},
		dirData("dirA", []child{
			fileData("fileA", contentsA),
			fileData("fileB", contentsB),
			fileData("fileC", contentsC),
			fileData("fileD", contentsD),
			fileData("fileE", contentsE),
		}),
	)
}

var offsetReadTests = [...]struct {
	Start, Length int64
	Expectation   string
}{
	{
		0, 10,
		contentsE[:10],
	},
	{
		0, 100,
		contentsE[:100],
	},
	{
		0, 1000,
		contentsE[:1000],
	},
	{
		100, 10,
		contentsE[100:110],
	},
	{
		100, 100,
		contentsE[100:200],
	},
	{
		100, 1000,
		contentsE[100:1100],
	},
	{
		0, 1 << 15,
		contentsE[:1<<15],
	},
	{
		1, 1 << 15,
		contentsE[1 : 1+1<<15],
	},
	{
		int64(len(contentsE)) - 1000, 1000,
		contentsE[len(contentsE)-1000:],
	},
}

func TestOpenReadAt(t *testing.T) {
	var buf [1 << 15]byte

	test(
		t,
		[]testFn{
			func(sfs FS) error {
				a, err := sfs.Open("dirA/fileE")
				if err != nil {
					return fmt.Errorf("unexpected error opening file in squashfs FS: %w", err)
				}

				r, ok := a.(io.ReaderAt)
				if !ok {
					return fmt.Errorf("didn't get io.ReaderAt")
				}

				for n, test := range offsetReadTests {
					m, err := r.ReadAt(buf[:test.Length], test.Start)
					if err != nil {
						return fmt.Errorf("test %d: %w", n+1, err)
					}

					if out := string(buf[:m]); out != test.Expectation {
						return fmt.Errorf("test %d: expecting to read %q (%d), got %q (%d)", n+1, test.Expectation, len(test.Expectation), out, m)
					}
				}

				return nil
			},
		},
		dirData("dirA", []child{
			fileData("fileE", contentsE),
		}),
	)
}

func TestSeek(t *testing.T) {
	var buf [1 << 15]byte

	test(
		t,
		[]testFn{
			func(sfs FS) error {
				a, err := sfs.Open("dirA/fileE")
				if err != nil {
					return fmt.Errorf("unexpected error opening file in squashfs FS: %w", err)
				}

				r, ok := a.(io.ReadSeeker)
				if !ok {
					return fmt.Errorf("didn't get io.ReaderAt")
				}

				for n, test := range offsetReadTests {
					for s, seek := range [...]struct {
						Offset int64
						Whence int
					}{
						{
							test.Start,
							io.SeekStart,
						},
						{
							-test.Length,
							io.SeekCurrent,
						},
						{
							test.Start - int64(len(contentsE)),
							io.SeekEnd,
						},
					} {
						p, err := r.Seek(seek.Offset, seek.Whence)
						if err != nil {
							return fmt.Errorf("test %d.%d: %w", n+1, s+1, err)
						}

						if p != test.Start {
							return fmt.Errorf("test %d.%d: expecting to be at byte %d, actually at %d", n+1, s+1, test.Start, p)
						}

						m, err := r.Read(buf[:test.Length])
						if err != nil {
							return fmt.Errorf("test %d.%d: %w", n+1, s+1, err)
						}

						if out := string(buf[:m]); out != test.Expectation {
							return fmt.Errorf("test %d.%d: expecting to read %q (%d), got %q (%d)", s+1, n+1, test.Expectation, len(test.Expectation), out, m)
						}
					}
				}

				return nil
			},
		},
		dirData("dirA", []child{
			fileData("fileE", contentsE),
		}),
	)
}

func TestStat(t *testing.T) {
	test(
		t,
		[]testFn{
			func(sfs FS) error {
				stats, err := sfs.Stat(".")
				if err != nil {
					return fmt.Errorf("unexpected error stat'ing root: %w", err)
				}

				if !stats.IsDir() {
					return fmt.Errorf("expecting stat for root dir to be a dir")
				}

				if m := stats.Mode(); m&0o777 != 0o777 {
					return fmt.Errorf("expecting perms 777, got %s", m)
				}

				return nil
			},
			func(sfs FS) error {
				stats, err := sfs.Stat("dirA")
				if err != nil {
					return fmt.Errorf("unexpected error stat'ing dir: %w", err)
				}

				if !stats.IsDir() {
					return fmt.Errorf("expecting stat for dir to be a dir")
				}

				if m := stats.Mode(); m&0o555 != 0o555 {
					return fmt.Errorf("expecting perms 555, got %s", m)
				}

				return nil
			},
			func(sfs FS) error {
				stats, err := sfs.Stat("dirB/fileA")
				if err != nil {
					return fmt.Errorf("unexpected error stat'ing file: %w", err)
				}

				if stats.IsDir() {
					return fmt.Errorf("expecting stat for file to be not a dir")
				}

				if size := stats.Size(); size != int64(len(contentsA)) {
					return fmt.Errorf("expecting size for file to be %d, got %d", len(contentsA), size)
				}

				if m := stats.Mode(); m != 0o600 {
					return fmt.Errorf("expecting perms %s, got %s", fs.FileMode(0o777), m)
				}

				if time := stats.ModTime(); time.Sub(timestamp) != 0 {
					return fmt.Errorf("expecting modtime %s, got %s", timestamp, time)
				}

				return nil
			},
			func(sfs FS) error {
				stats, err := sfs.Stat("dirC/fileC")
				if err != nil {
					return fmt.Errorf("unexpected error stat'ing file: %w", err)
				}

				if m := stats.Mode(); m != 0o123 {
					return fmt.Errorf("expecting perms %s, got %s", fs.FileMode(0o123), m)
				}

				return nil
			},
			func(sfs FS) error {
				stats, err := sfs.Stat("dirD/fileD")
				if err != nil {
					return fmt.Errorf("unexpected error stat'ing file: %w", err)
				}

				if m := stats.Mode(); m != 0o123 {
					return fmt.Errorf("expecting perms %s, got %s", fs.FileMode(0o123), m)
				}

				return nil
			},
			func(sfs FS) error {
				stats, err := sfs.Stat("dirD/fileE")
				if err != nil {
					return fmt.Errorf("unexpected error stat'ing file: %w", err)
				}

				if m := stats.Mode(); m != 0o123 {
					return fmt.Errorf("expecting perms %s, got %s", fs.FileMode(0o123), m)
				}

				return nil
			},
			func(sfs FS) error {
				stats, err := sfs.LStat("dirD/fileE")
				if err != nil {
					return fmt.Errorf("unexpected error stat'ing file: %w", err)
				}

				expected := fs.ModeSymlink | fs.ModePerm

				if m := stats.Mode(); m != expected {
					return fmt.Errorf("expecting perms %s, got %s", expected, m)
				}

				return nil
			},
			func(sfs FS) error {
				stats, err := sfs.Stat("dirE/fileB")
				if err != nil {
					return fmt.Errorf("unexpected error stat'ing file: %w", err)
				}

				if m := stats.Mode(); m != 0o123 {
					return fmt.Errorf("expecting perms %s, got %s", fs.FileMode(0o123), m)
				}

				return nil
			},
			func(sfs FS) error {
				stats, err := sfs.LStat("dirE/fileB")
				if err != nil {
					return fmt.Errorf("unexpected error stat'ing file: %w", err)
				}

				if m := stats.Mode(); m != 0o123 {
					return fmt.Errorf("expecting perms %s, got %s", fs.FileMode(0o123), m)
				}

				return nil
			},
		},
		dirData("dirA", []child{}, chmod(0o555)),
		dirData("dirB", []child{
			fileData("fileA", contentsA, chmod(0o600), modtime(timestamp)),
		}),
		dirData("dirC", []child{
			fileData("fileB", contentsA, chmod(0o123)),
			symlink("fileC", "fileB", chmod(0o321)),
		}),
		dirData("dirD", []child{
			symlink("fileD", "../dirC/fileB", chmod(0o231)),
			symlink("fileE", "/dirC/fileB", chmod(0o132)),
		}),
		symlink("dirE", "dirC"),
	)
}
