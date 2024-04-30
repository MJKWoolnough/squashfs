package squashfs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

const contentsA = "my contents"

type testFn func(FS) error

func test(t *testing.T, tests []testFn, children ...child) {
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

	for n, test := range tests {
		if err := test(sfs); err != nil {
			t.Errorf("test %d: %s", n+1, err)
		}
	}
}

func TestOpen(t *testing.T) {
	test(
		t,
		[]testFn{
			func(sfs FS) error {
				a, err := sfs.Open(filepath.Join("/", "dirA", "fileA"))
				if err != nil {
					return fmt.Errorf("unexpected error opening file in squashfs FS: %w", err)
				}

				contents, err := io.ReadAll(a)
				if err != nil {
					return fmt.Errorf("unexpected error reading file in squashfs FS: %w", err)
				}

				if string(contents) != contentsA {
					return fmt.Errorf("expected to read %q, got %q", contentsA, contents)
				}

				return nil
			},
		},
		dir("dirA", []child{
			fileData("fileA", contentsA),
		}),
	)
}

func TestStat(t *testing.T) {
	test(
		t,
		[]testFn{
			func(sfs FS) error {
				stats, err := sfs.Stat("/")
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
				stats, err := sfs.Stat("/dirA")
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
				stats, err := sfs.Stat("/dirB/fileA")
				if err != nil {
					return fmt.Errorf("unexpected error stat'ing file: %w", err)
				}

				if stats.IsDir() {
					return fmt.Errorf("expecting stat for file to be not a dir")
				}

				if size := stats.Size(); size != int64(len(contentsA)) {
					return fmt.Errorf("expecting size for file to be %d, got %d", len(contentsA), size)
				}

				return nil
			},
		},
		dir("dirA", []child{}, chmod(0o555)),
		dir("dirB", []child{
			fileData("fileA", contentsA),
		}),
	)
}
