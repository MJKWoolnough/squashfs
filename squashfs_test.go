package squashfs

import (
	"errors"
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

type testFn func(*SquashFS) error

func test(t *testing.T, runFSTest bool, tests []testFn, children ...child) {
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

	if runFSTest {
		if err := fstest.TestFS(sfs, ".required"); err != nil {
			t.Fatal(err)
		}
	}

	for n, test := range tests {
		if err := test(sfs); err != nil {
			t.Errorf("test %d: %s", n+1, err)
		}
	}
}

func readSqfsFile(sfs fs.FS, path, expectation string) error {
	f, err := sfs.Open(path)
	if err != nil {
		return fmt.Errorf("unexpected error opening file in squashfs FS: %w", err)
	}

	contents, err := io.ReadAll(f)
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
		true,
		[]testFn{
			func(sfs *SquashFS) error {
				return readSqfsFile(sfs, filepath.Join("dirA", "fileA"), contentsA)
			},
			func(sfs *SquashFS) error {
				return readSqfsFile(sfs, filepath.Join("dirA", "fileB"), contentsB)
			},
			func(sfs *SquashFS) error {
				return readSqfsFile(sfs, filepath.Join("dirA", "fileC"), contentsC)
			},
			func(sfs *SquashFS) error {
				return readSqfsFile(sfs, filepath.Join("dirA", "fileD"), contentsD)
			},
			func(sfs *SquashFS) error {
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
		true,
		[]testFn{
			func(sfs *SquashFS) error {
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
		true,
		[]testFn{
			func(sfs *SquashFS) error {
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
	const (
		symD = "../dirC/fileB"
		symE = "/dirC/fileB"
	)

	test(
		t,
		false,
		[]testFn{
			func(sfs *SquashFS) error {
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
			func(sfs *SquashFS) error {
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
			func(sfs *SquashFS) error {
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
			func(sfs *SquashFS) error {
				stats, err := sfs.Stat("dirC/fileC")
				if err != nil {
					return fmt.Errorf("unexpected error stat'ing file: %w", err)
				}

				if m := stats.Mode(); m != 0o123 {
					return fmt.Errorf("expecting perms %s, got %s", fs.FileMode(0o123), m)
				}

				return nil
			},
			func(sfs *SquashFS) error {
				stats, err := sfs.Stat("dirD/fileD")
				if err != nil {
					return fmt.Errorf("unexpected error stat'ing file: %w", err)
				}

				if m := stats.Mode(); m != 0o123 {
					return fmt.Errorf("expecting perms %s, got %s", fs.FileMode(0o123), m)
				}

				return nil
			},
			func(sfs *SquashFS) error {
				stats, err := sfs.Stat("dirD/fileE")
				if err != nil {
					return fmt.Errorf("unexpected error stat'ing file: %w", err)
				}

				if m := stats.Mode(); m != 0o123 {
					return fmt.Errorf("expecting perms %s, got %s", fs.FileMode(0o123), m)
				}

				return nil
			},
			func(sfs *SquashFS) error {
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
			func(sfs *SquashFS) error {
				stats, err := sfs.Stat("dirE/fileB")
				if err != nil {
					return fmt.Errorf("unexpected error stat'ing file: %w", err)
				}

				if m := stats.Mode(); m != 0o123 {
					return fmt.Errorf("expecting perms %s, got %s", fs.FileMode(0o123), m)
				}

				return nil
			},
			func(sfs *SquashFS) error {
				stats, err := sfs.LStat("dirE/fileB")
				if err != nil {
					return fmt.Errorf("unexpected error stat'ing file: %w", err)
				}

				if m := stats.Mode(); m != 0o123 {
					return fmt.Errorf("expecting perms %s, got %s", fs.FileMode(0o123), m)
				}

				return nil
			},
			func(sfs *SquashFS) error {
				sym, err := sfs.Readlink("dirD/fileD")
				if err != nil {
					return fmt.Errorf("unexpected error readlink'ing file: %w", err)
				}

				if sym != symD {
					return fmt.Errorf("expecting symlink dest %q, got %q", symD, sym)
				}

				return nil
			},
			func(sfs *SquashFS) error {
				sym, err := sfs.Readlink("dirD/fileE")
				if err != nil {
					return fmt.Errorf("unexpected error readlink'ing file: %w", err)
				}

				if sym != symE {
					return fmt.Errorf("expecting symlink dest %q, got %q", symE, sym)
				}

				return nil
			},
			func(sfs *SquashFS) error {
				stats, err := sfs.Stat("dirA")
				if err != nil {
					return fmt.Errorf("unexpected error stat'ing file: %w", err)
				}

				dir, ok := stats.(dirStat)
				if !ok {
					return fmt.Errorf("expecting dir type, got: %t", stats)
				}

				if dir.uid != 0 || dir.gid != 0 {
					return fmt.Errorf("expecting uid %d and gid %d, got %d and %d", 0, 0, dir.uid, dir.gid)
				}

				return nil
			},
			func(sfs *SquashFS) error {
				stats, err := sfs.Stat("dirB/fileA")
				if err != nil {
					return fmt.Errorf("unexpected error stat'ing file: %w", err)
				}

				file, ok := stats.(fileStat)
				if !ok {
					return fmt.Errorf("expecting dir type, got: %t", stats)
				}

				if file.uid != 1000 || file.gid != 1000 {
					return fmt.Errorf("expecting uid %d and gid %d, got %d and %d", 1000, 1000, file.uid, file.gid)
				}

				return nil
			},
			func(sfs *SquashFS) error {
				stats, err := sfs.Stat("dirC/fileB")
				if err != nil {
					return fmt.Errorf("unexpected error stat'ing file: %w", err)
				}

				file, ok := stats.(fileStat)
				if !ok {
					return fmt.Errorf("expecting dir type, got: %t", stats)
				}

				if file.uid != 123 || file.gid != 456 {
					return fmt.Errorf("expecting uid %d and gid %d, got %d and %d", 123, 456, file.uid, file.gid)
				}

				return nil
			},
		},
		dirData("dirA", []child{}, chmod(0o555)),
		dirData("dirB", []child{
			fileData("fileA", contentsA, chmod(0o600), modtime(timestamp), owner(1000, 1000)),
		}),
		dirData("dirC", []child{
			fileData("fileB", contentsA, chmod(0o123), owner(123, 456)),
			symlink("fileC", "fileB", chmod(0o321)),
		}),
		dirData("dirD", []child{
			symlink("fileD", symD, chmod(0o231)),
			symlink("fileE", symE, chmod(0o132)),
		}),
		symlink("dirE", "dirC"),
	)
}

func TestReadDir(t *testing.T) {
	test(
		t,
		true,
		[]testFn{
			func(sfs *SquashFS) error {
				entries, err := sfs.ReadDir("dirA")
				if err != nil {
					return err
				}

				if len(entries) != 0 {
					return fmt.Errorf("expecting no entries, got %v", entries)
				}

				return nil
			},
			func(sfs *SquashFS) error {
				entries, err := sfs.ReadDir("dirB")
				if err != nil {
					return err
				}

				if len(entries) != 1 {
					return fmt.Errorf("expecting 1 entry, got %d", len(entries))
				}

				if name := entries[0].Name(); name != "childA" {
					return fmt.Errorf("expecting entry to be %q, got %q", "childA", name)
				}

				return nil
			},
			func(sfs *SquashFS) error {
				entries, err := sfs.ReadDir("dirC")
				if err != nil {
					return err
				}

				if len(entries) != 3 {
					return fmt.Errorf("expecting 3 entries, got %d", len(entries))
				}

				for n, child := range [...]string{"childA", "childB", "childC"} {
					if name := entries[n].Name(); name != child {
						return fmt.Errorf("expecting entry %d to be %q, got %q", n+1, child, name)
					}
				}

				return nil
			},
		},
		dirData("dirA", []child{}),
		dirData("dirB", []child{
			fileData("childA", ""),
		}),
		dirData("dirC", []child{
			dirData("childA", []child{}),
			fileData("childB", ""),
			symlink("childC", "childB"),
		}, chmod(0o432)),
	)
}

func TestReadlink(t *testing.T) {
	test(
		t,
		false,
		[]testFn{
			func(sfs *SquashFS) error {
				sym, err := sfs.ReadLink("childA")
				if !errors.Is(err, fs.ErrInvalid) {
					return fmt.Errorf("expecting error fs.ErrInvalid, got %s", err)
				} else if sym != "" {
					return fmt.Errorf("expecting an empty string, got %q", sym)
				}

				return nil
			},
			func(sfs *SquashFS) error {
				const expected = "childA"

				sym, err := sfs.ReadLink("symB")
				if err != nil {
					return fmt.Errorf("got unexpected error: %s", err)
				} else if sym != expected {
					return fmt.Errorf("expecting an %s, got %q", expected, sym)
				}

				return nil
			},
			func(sfs *SquashFS) error {
				const expected = "/not/exist"

				sym, err := sfs.ReadLink("symC")
				if err != nil {
					return fmt.Errorf("got unexpected error: %s", err)
				} else if sym != expected {
					return fmt.Errorf("expecting an %s, got %q", expected, sym)
				}

				return nil
			},
			func(sfs *SquashFS) error {
				const expected = "symE"

				sym, err := sfs.ReadLink("dirD/symE")
				if err != nil {
					return fmt.Errorf("got unexpected error: %s", err)
				} else if sym != expected {
					return fmt.Errorf("expecting an %s, got %q", expected, sym)
				}

				return nil
			},
		},
		fileData("childA", ""),
		symlink("symB", "childA"),
		symlink("symC", "/not/exist"),
		dirData("dirD", []child{
			symlink("symE", "symE"),
		}),
	)
}
