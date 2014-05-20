package ramfs

import (
	"bytes"
	"io"
	"testing"
)

var writeTests = []struct {
	data   []byte
	offset int64
	blocks int
	size   uint64
}{
	// avoid fragments
	{[]byte("as"), 12, 1, 2},

	{[]byte("df"), 2, 1, 4},
	{[]byte("ghjk"), 4, 1, 8},
	{[]byte("xxxx"), 0, 1, 8},
	{[]byte("iiiittttq"), 8, 3, 17},
	{[]byte("s"), 0, 3, 17},

	// avoid fragments
	{[]byte("uuuu"), 800, 3, 21},
}
var producedResult = []byte("sxxxghjkiiiittttquuuu")

var readTests = []struct {
	result []byte
	offset int64
	size   int
	should int
}{
	{[]byte("sxxx"), 0, 4, 4},
	{[]byte("ghj"), 4, 3, 3},
	{[]byte("hjki"), 5, 4, 4},
	{[]byte("uu"), int64(len(producedResult) - 2), 10, 2},
	{producedResult, 0, len(producedResult), len(producedResult)},
}

func TestWriteRead(t *testing.T) {
	f := &file{
		block:     make(map[uint64][]byte),
		blockSize: uint64(8),
	}

	for i, test := range writeTests {
		n, err := f.WriteAt(test.data, test.offset)
		if err != nil {
			t.Fatalf("writeat 1:%d: %v", i, err)
		}
		if n != len(test.data) {
			t.Fatalf("write %d: expected len %d, got len %d", i, len(test.data), n)
		}
		if test.blocks != len(f.block) {
			t.Fatalf("write %d: expected blocks %d, got blocks %d", i, test.blocks, len(f.block))
		}
		if test.size != f.size {
			t.Fatalf("write %d: expected size %d, got size %d", i, test.size, f.size)
		}
	}

	// test io.EOF
	var data []byte
	_, err := f.ReadAt(data, 9999)
	if err != io.EOF {
		t.Fatalf("io.EOF: expected io.EOF, got %v", err)
	}

	for i, test := range readTests {
		data := make([]byte, test.size)
		n, err := f.ReadAt(data, test.offset)
		if err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		if n != test.should {
			t.Fatalf("read %d: expected len %d, got len %d", i, test.should, n)
		}
		if bytes.Compare(data[:n], test.result) != 0 {
			t.Fatalf("read %d: expected data %s, got data %s", i, test.result, data[:n])
		}
	}
}

func write(t *testing.T, r io.Reader, w io.WriterAt, offset int64) {
	data := make([]byte, BLOCKSIZE)
	for {
		n, err := r.Read(data)
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("read: %v", err)
		}

		m, err := w.WriteAt(data[0:n], offset)
		if err != nil {
			t.Fatalf("write: %v", err)
		}
		if m != n {
			t.Fatalf("short write: %v", err)
		}
		offset += int64(m)
	}
}

func read(t *testing.T, w io.Writer, r io.ReaderAt) {
	data := make([]byte, BLOCKSIZE)
	offset := int64(0)
	for {
		n, err := r.ReadAt(data, offset)
		if err != nil {
			// code.goole.com/p/goplan9/plan9/client#Fid returns
			// client.Error (EOF)
			if err == io.EOF || err.Error() == "EOF" {
				break
			}
			t.Fatalf("read: %v", err)
		}
		if n == 0 {
			break
		}

		offset += int64(n)
		if _, err := w.Write(data[0:n]); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
}

func TestLength(t *testing.T) {
	file := &file{
		block:     make(map[uint64][]byte),
		blockSize: uint64(32),
	}

	n, err := file.WriteAt([]byte("aaa"), 0)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	n, err = file.WriteAt([]byte("bbbbb"), 0)
	if err != nil {
		t.Fatalf("write %v", err)
	}
	if n != 5 {
		t.Fatalf("write: expected len 5, got len %d", n)
	}
	if file.Len() != 5 {
		t.Fatalf("length differ: expected 5, got %d", file.Len())
	}
}
