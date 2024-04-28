package squashfs

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

const contentsA = "my contents"

func TestOpen(t *testing.T) {
	sqfs, err := buildSquashFS(
		t,
		dir("dirA", []child{
			file("fileA", contentsA),
		}),
	)
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

	a, err := sfs.Open(filepath.Join("/", "dirA", "fileA"))
	if err != nil {
		t.Fatalf("unexpected error opening file in squashfs FS: %s", err)
	}

	contents, err := io.ReadAll(a)
	if err != nil {
		t.Fatalf("unexpected error reading file in squashfs FS: %s", err)
	}

	if string(contents) != contentsA {
		t.Errorf("expected to read %q, got %q", contentsA, contents)
	}
}

func TestStat(t *testing.T) {
	sqfs, err := buildSquashFS(
		t,
		dir("dirA", []child{}, chmod(0o555)),
		dir("dirB", []child{
			file("fileA", contentsA),
		}),
	)
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

	stats, err := sfs.Stat("/")
	if err != nil {
		t.Fatalf("unexpected error stat'ing root: %s", err)
	}

	if !stats.IsDir() {
		t.Fatal("expecting stat for root dir to be a dir")
	}

	if m := stats.Mode(); m&0o777 != 0o777 {
		t.Fatalf("expecting perms 777, got %s", m)
	}

	stats, err = sfs.Stat("/dirA")
	if err != nil {
		t.Fatalf("unexpected error stat'ing dir: %s", err)
	}

	if !stats.IsDir() {
		t.Fatal("expecting stat for dir to be a dir")
	}

	if m := stats.Mode(); m&0o555 != 0o555 {
		t.Fatalf("expecting perms 555, got %s", m)
	}

	stats, err = sfs.Stat("/dirB/fileA")
	if err != nil {
		t.Fatalf("unexpected error stat'ing file: %s", err)
	}

	if stats.IsDir() {
		t.Fatal("expecting stat for file to be not a dir")
	}

	if size := stats.Size(); size != int64(len(contentsA)) {
		t.Fatalf("expecting size for file to be %d, got %d", len(contentsA), size)
	}
}
