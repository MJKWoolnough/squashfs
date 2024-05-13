package squashfs

import (
	"bytes"
	"compress/zlib"
	"io"
	"testing"
)

func compress(i int) io.ReadSeeker {
	var buf bytes.Buffer

	z := zlib.NewWriter(&buf)
	z.Write([]byte{byte(i)})
	z.Close()

	return bytes.NewReader(buf.Bytes())
}

func readBlock(f io.Reader) byte {
	var num [1]byte

	f.Read(num[:])

	return num[0]
}

func TestBlockCache(t *testing.T) {
	b := newBlockCache(10)

	for i := 0; i < 20; i++ {
		f, err := b.getBlock(int64(i%10), compress(i), CompressorGZIP)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		} else if num := readBlock(f); num != byte(i)%10 {
			t.Errorf("test %d.1: expecting to read byte %d, got %d", i+1, i%10, num)
		} else if i < 10 {
			if f, err := b.getBlock(int64(i), nil, CompressorGZIP); err != nil {
				t.Fatalf("unexpected error: %s", err)
			} else if num := readBlock(f); num != byte(i) {
				t.Errorf("test %d.1: expecting to read byte %d, got %d", i+1, i, num)
			}
		}
	}
}
