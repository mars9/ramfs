package ramfs

import (
	"bytes"
	"testing"

	"9fans.net/go/plan9"
)

func writeTest(t *testing.T, file *node) {
	data := []byte("hello world")
	size1, err := file.WriteAt(data, int64(0))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if size1 != len(data) {
		t.Fatalf("write: short write")
	}

	data = []byte("planet go")
	size2, err := file.WriteAt(data, 6)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if size2 != len(data) {
		t.Fatalf("write: short write")
	}

	result := []byte("hello planet go")
	buf := make([]byte, len(result))
	n, err := file.ReadAt(buf, int64(0))
	if err != nil {
		t.Fatalf("read 1: %v", err)
	}
	if n != len(buf) {
		t.Fatalf("read 1: short read")
	}
	if bytes.Compare(buf, result) != 0 {
		t.Fatalf("read 1: expected %q, got %q", result, buf)
	}
}

func TestCreateOpenClose(t *testing.T) {
	fs := New("adm")
	root := newNode(fs, "/", "adm", "adm", 0775|plan9.DMDIR, 0, nil)
	dir, err := root.Create("adm", "dir", plan9.ORDWR, 0775|plan9.DMDIR)
	if err != nil {
		t.Fatalf("create dir: %v", err)
	}
	if err := dir.Open(plan9.ORDWR); err != nil {
		t.Fatalf("open dir: %v", err)
	}

	file, err := dir.Create("adm", "file", plan9.ORDWR, 0664)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	if err := file.Open(plan9.ORDWR); err != nil {
		t.Fatalf("open file: %v", err)
	}

	writeTest(t, file)

	if err := file.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}
	if err := dir.Close(); err != nil {
		t.Fatalf("close dir: %v", err)
	}
}

func TestRemove(t *testing.T) {
	fs := New("adm")
	root := newNode(fs, "/", "adm", "adm", 0775|plan9.DMDIR, 0, nil)
	dir, err := root.Create("adm", "dir", plan9.ORDWR, 0775|plan9.DMDIR)
	if err != nil {
		t.Fatalf("create dir: %v", err)
	}
	if err := dir.Open(plan9.ORDWR); err != nil {
		t.Fatalf("open dir: %v", err)
	}
	defer dir.Close()

	if err := dir.Remove(); err != nil {
		t.Fatalf("remove dir: %v", err)
	}
}

func TestExlusiveMode(t *testing.T) {
	fs := New("adm")
	file := newNode(fs, "file", "adm", "adm", 0664|plan9.DMEXCL, 0, newFile(BLOCKSIZE))
	if err := file.Open(plan9.OWRITE); err != nil {
		t.Fatalf("open file: %v", err)
	}
	if err := file.Open(plan9.OWRITE); err == nil {
		t.Fatalf("open expected ErrExcl, got nil error")
	}
	if err := file.Open(plan9.OWRITE); err == nil {
		t.Fatalf("open expected ErrExcl, got nil error")
	}

	writeTest(t, file)

	if err := file.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}
}
