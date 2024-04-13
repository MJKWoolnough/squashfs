package squashfs

import (
	"archive/tar"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

var checkSQFSTar = func(_ *testing.T) {}

func TestMain(m *testing.M) {
	_, err := exec.LookPath("sqfstar")
	if err != nil {
		checkSQFSTar = (*testing.T).SkipNow
	}

	os.Exit(m.Run())
}

type option func(*tar.Header)

func modtime(t time.Time) option {
	return func(h *tar.Header) {
		h.ModTime = t
	}
}

func chmod(perms fs.FileMode) option {
	return func(h *tar.Header) {
		h.Mode = int64(perms)
	}
}

type directory struct {
	tar.Header
	children []child
}

func (d *directory) add(parent *directory) {
	parent.children = append(parent.children, d)
}

func (d *directory) writeTo(w *tar.Writer, path string) error {
	d.Header.Name = filepath.Join(path, d.Header.Name)

	if err := w.WriteHeader(&d.Header); err != nil {
		return err
	}

	for _, child := range d.children {
		if err := child.writeTo(w, d.Header.Name); err != nil {
			return err
		}
	}

	return nil
}

type data struct {
	tar.Header
	contents string
}

func (d *data) add(parent *directory) {
	parent.children = append(parent.children, d)
}

func (d *data) writeTo(w *tar.Writer, path string) error {
	d.Header.Name = filepath.Join(path, d.Header.Name)

	if err := w.WriteHeader(&d.Header); err != nil {
		return err
	}

	_, err := io.WriteString(w, d.contents)

	return err
}

type child interface {
	add(*directory)
	writeTo(*tar.Writer, string) error
}

func buildSquashFS(t *testing.T, children ...child) (string, error) {
	t.Helper()

	pr, pw := io.Pipe()
	ch := make(chan error, 1)

	go func() {
		w := tar.NewWriter(pw)

		for _, child := range children {
			if err := child.writeTo(w, "/"); err != nil {
				ch <- err
			}
		}

		w.Close()
		pw.Close()

		close(ch)
	}()

	tmp := t.TempDir()

	sqfs := filepath.Join(tmp, "out.sqfs")

	cmd := exec.Command("sqfstar", sqfs)
	cmd.Stdin = pr

	if err := cmd.Run(); err != nil {
		return "", err
	}

	pr.Close()

	if err := <-ch; err != nil {
		return "", err
	}

	return sqfs, nil
}

func dir(name string, children []child, opts ...option) *directory {
	dir := &directory{
		Header: tar.Header{
			Name:     name,
			Typeflag: tar.TypeDir,
			Mode:     0o555,
			ModTime:  time.Now(),
			Format:   tar.FormatGNU,
		},
		children: children,
	}

	for _, opt := range opts {
		opt(&dir.Header)
	}

	return dir
}

func file(name string, contents string, opts ...option) *data {
	file := &data{
		Header: tar.Header{
			Name:     name,
			Typeflag: tar.TypeReg,
			Mode:     0o555,
			ModTime:  time.Now(),
			Size:     int64(len(contents)),
			Format:   tar.FormatGNU,
		},
		contents: contents,
	}

	for _, opt := range opts {
		opt(&file.Header)
	}

	return file
}

func TestGetStats(t *testing.T) {
	checkSQFSTar(t)

	sqfs, err := buildSquashFS(
		t,
		dir("dirA", []child{
			file("fileA", "my contents"),
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

	_, err = GetStats(f)
	if err != nil {
		t.Fatalf("unexpected error reading squashfs file: %s", err)
	}
}
