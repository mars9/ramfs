package ramfs

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"code.google.com/p/goplan9/plan9"
	"code.google.com/p/goplan9/plan9/client"
)

var testServerAddr = "localhost:15640"

func init() {
	go func() {
		New("").Listen("tcp", testServerAddr)
	}()
}

func newFsys(t *testing.T, uid string) (*client.Conn, *client.Fsys) {
	c, err := client.Dial("tcp", testServerAddr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	fs, err := c.Attach(nil, uid, "")
	if err != nil {
		t.Fatalf("attach: %v", err)
	}

	return c, fs
}

func TestAttach(t *testing.T) {
	c, err := client.Dial("tcp", testServerAddr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	}()

	_, err = c.Attach(nil, "adm", "/")
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	// TODO: fid ref count
	//	fs, err = c.Attach(nil, "adm", "/xxx/yyyy")
	//	if err == nil {
	//		t.Fatalf("expected attach error")
	//	}
}

func TestFileServerInit(t *testing.T) {
	c, fs := newFsys(t, "adm")
	defer c.Close()

	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("/dir%d", i)
		dir, err := fs.Create(name, plan9.OREAD, 0775|plan9.DMDIR)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}

		for j := 0; j < 10; j++ {
			name = fmt.Sprintf("/dir%d/sub%d", i, j)
			dir1, err := fs.Create(name, plan9.OREAD, 0775|plan9.DMDIR)
			if err != nil {
				t.Fatalf("create %s: %v", name, err)
			}
			if err = dir1.Close(); err != nil {
				t.Fatal(err)
			}

			name = fmt.Sprintf("/dir%d/file%d", i, j)
			file1, err := fs.Create(name, plan9.ORDWR, 0664)
			if err != nil {
				t.Fatalf("create %s: %v", name, err)
			}
			if err = file1.Close(); err != nil {
				t.Fatal(err)
			}
		}

		name = fmt.Sprintf("/file%d", i)
		file, err := fs.Create(name, plan9.ORDWR, 0664)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if err = file.Close(); err != nil {
			t.Fatal(err)
		}
		if err = dir.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFileServerReadWrite(t *testing.T) {
	c, fs := newFsys(t, "adm")
	defer c.Close()

	file, err := fs.Open("/file1", plan9.OREAD|plan9.OWRITE)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() {
		if err = file.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	}()

	osfile, err := os.Open("node.go")
	if err != nil {
		t.Fatal(err)
	}
	defer osfile.Close()

	buf := bytes.NewBuffer(nil)
	write(t, osfile, file, 0)
	read(t, buf, file)

	data, err := ioutil.ReadFile("node.go")
	if err != nil {
		t.Fatalf("os read: %v", err)
	}
	if bytes.Compare(data, buf.Bytes()) != 0 {
		t.Fatalf("files differ")
	}
}

func TestFileServerWstat(t *testing.T) {
	c, fs := newFsys(t, "adm")
	defer c.Close()

	stat, err := fs.Stat("/file1")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	stat.Name = "nfile"
	stat.Mode = 0660
	if err = fs.Wstat("/file1", stat); err != nil {
		t.Fatalf("wstat: %v", err)
	}

	stat1, err := fs.Stat("/nfile")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if stat1.Name != stat.Name {
		t.Fatalf("expected Name %s, got %s", stat.Name, stat1.Name)
	}
	adjustedMode := plan9.Perm(0644)
	if stat1.Mode != adjustedMode {
		t.Fatalf("expected Mode %s, got %s", adjustedMode, stat1.Mode)
	}

	// existing file
	stat.Name = "file2"
	if err = fs.Wstat("/nfile", stat); err == nil {
		t.Fatalf("wstat: %v", err)
	}
}

func TestWalk(t *testing.T) {
	fs := New("")

	dirA := newNode(fs, "a", "", "", 0775|plan9.DMDIR, 0, nil)
	dirB := newNode(fs, "b", "", "", 0775|plan9.DMDIR, 1, nil)
	dirC := newNode(fs, "c", "", "", 0775|plan9.DMDIR, 2, nil)
	dirD := newNode(fs, "d", "", "", 0775|plan9.DMDIR, 3, nil)
	fileA := newNode(fs, "fa", "", "", 0664, 4, nil)
	fileB := newNode(fs, "fb", "", "", 0664, 5, nil)

	dirA.children["b"] = dirB
	dirA.parent = fs.root

	dirB.children["c"] = dirC
	dirB.children["d"] = dirD
	dirB.parent = dirA

	dirC.children["fa"] = fileA
	dirC.children["fb"] = fileB
	dirC.parent = dirB

	dirD.parent = dirB

	fileA.parent = dirC
	fileB.parent = dirC

	fs.root.children["a"] = dirA

	if _, err := fs.walk("/a/b/c/fa"); err != nil {
		t.Fatalf("walk: %v", err)
	}
	if _, err := fs.walk("/a/b/c/x"); err == nil {
		t.Fatalf("walk: got nil error, expected ErrNotExist")
	}
	if _, err := fs.walk("/"); err != nil {
		t.Fatalf("walk: %v", err)
	}
	if _, err := fs.walk("/a/../a/b/../b"); err != nil {
		t.Fatalf("walk: %v", err)
	}
}
