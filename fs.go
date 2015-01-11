/*
Package ramfs implements a 9P2000 file server keeping all files in
memory.

A 9P2000 server is an agent that provides one or more hierarchical
file systems -- file trees -- that may be accessed by processes.
A server responds to requests by clients to navigate the hierarchy,
and to create, remove, read, and write files.

References:
  [intro]   http://plan9.bell-labs.com/magic/man2html/5/0intro
  [attach]  http://plan9.bell-labs.com/magic/man2html/5/attach
  [clunk]   http://plan9.bell-labs.com/magic/man2html/5/clunk
  [error]   http://plan9.bell-labs.com/magic/man2html/5/error
  [flush]   http://plan9.bell-labs.com/magic/man2html/5/flush
  [open]    http://plan9.bell-labs.com/magic/man2html/5/open
  [read]    http://plan9.bell-labs.com/magic/man2html/5/read
  [remove]  http://plan9.bell-labs.com/magic/man2html/5/remove
  [stat]    http://plan9.bell-labs.com/magic/man2html/5/stat
  [version] http://plan9.bell-labs.com/magic/man2html/5/version
  [walk]    http://plan9.bell-labs.com/magic/man2html/5/walk
*/
package ramfs

import (
	"net"
	"path"
	"strings"
	"sync"

	"9fans.net/go/plan9"
)

const maxPath = uint64(1<<64 - 1)

// RamFS constants and limits.
const (
	MSIZE = 128*1024 + plan9.IOHDRSZ // maximum message size
	// IOUNIT represents the maximum size that is guaranteed to be
	// transferred atomically.
	IOUNIT    = 128 * 1024
	BLOCKSIZE = 2 * 1024 * 1024 // maximum block size

	OREAD   = plan9.OREAD   // open for read
	OWRITE  = plan9.OWRITE  // open for write
	ORDWR   = plan9.ORDWR   // open for read/write
	OEXEC   = plan9.OEXEC   // read but check execute permission
	OTRUNC  = plan9.OTRUNC  // truncate file first
	ORCLOSE = plan9.ORCLOSE // remove on close
	OEXCL   = plan9.OEXCL   // exclusive use
	OAPPEND = plan9.OAPPEND // append only

	QTDIR    = plan9.QTDIR    // type bit for directories
	QTAPPEND = plan9.QTAPPEND // type bit for append only files
	QTEXCL   = plan9.QTEXCL   // type bit for exclusive use files
	QTAUTH   = plan9.QTAUTH   // type bit for authentication file
	QTTMP    = plan9.QTTMP    // type bit for non-backed-up file
	QTFILE   = plan9.QTFILE   // type bits for plain file

	DMDIR    = plan9.DMDIR    // mode bit for directories
	DMAPPEND = plan9.DMAPPEND // mode bit for append only files
	DMEXCL   = plan9.DMEXCL   // mode bit for exclusive use files
	DMAUTH   = plan9.DMAUTH   // mode bit for authentication file
	DMTMP    = plan9.DMTMP    // mode bit for non-backed-up file
	DMREAD   = plan9.DMREAD   // mode bit for read permission
	DMWRITE  = plan9.DMWRITE  // mode bit for write permission
	DMEXEC   = plan9.DMEXEC   // mode bit for execute permission
)

// LogFunc can be used to enable a trace of general debugging messages.
type LogFunc func(format string, v ...interface{})

// FS represents a a 9P2000 file server.
type FS struct {
	mu        sync.Mutex
	path      uint64
	pathmap   map[uint64]bool
	fidnew    chan (chan *Fid)
	root      *node
	group     *group
	hostowner string
	chatty    bool // not sync'd
	Log       LogFunc
}

// New starts a 9P2000 file server keeping all files in memory. The
// filesystem is entirely maintained in memory, no external storage is
// used. File data is allocated in 128 * 1024 byte blocks.
//
// The root of the filesystem is owned by the user who invoked ramfs and
// is created with Read, Write and Execute permissions for the owner and
// Read and Execute permissions for everyone else (0755). FS create the
// necessary directories and files in /adm/ctl, /adm/group and
// /<hostowner>.
func New(hostowner string) *FS {
	owner := hostowner
	if owner == "" {
		owner = "adm"
	}
	fs := &FS{
		path:      uint64(5),
		pathmap:   make(map[uint64]bool),
		fidnew:    make(chan (chan *Fid)),
		hostowner: owner,
	}
	fs.group = newGroup(fs, owner)

	root := newNode(fs, "/", owner, "adm", 0755|plan9.DMDIR, 0, nil)
	adm := newNode(fs, "adm", "adm", "adm", 0770|plan9.DMDIR, 1, nil)
	group := newNode(fs, "group", "adm", "adm", 0660, 2, fs.group)
	ctl := newNode(fs, "ctl", "adm", "adm", 0220, 3, newCtl(fs))

	root.children["adm"] = adm
	adm.children["group"] = group
	adm.children["ctl"] = ctl
	root.parent = root
	adm.parent = root
	group.parent = adm
	ctl.parent = adm
	if owner != "adm" {
		n := newNode(fs, owner, owner, owner, 0750|plan9.DMDIR, 4, nil)
		n.parent = root
		root.children[owner] = n
	}

	fs.root = root
	go fs.newFid(fs.fidnew)
	return fs
}

// Halt closes the filesystem, rendering it unusable for I/O.
func (fs *FS) Halt() error { return nil }

func (fs *FS) newPath() (uint64, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	for path := range fs.pathmap {
		delete(fs.pathmap, path)
		return path, nil
	}

	path := fs.path
	if fs.path == maxPath {
		return 0, perror("out of paths")
	}
	fs.path++
	return path, nil
}

func (fs *FS) delPath(path uint64) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.pathmap[path] = true
}

func (fs *FS) newFid(fidnew <-chan (chan *Fid)) {
	for ch := range fidnew {
		ch <- &Fid{
			num:  uint32(0),
			uid:  "none",
			node: fs.root,
		}
		close(ch)
	}
}

func (fs *FS) walk(name string) (*node, error) {
	root := fs.root
	path := split(name)
	if len(path) == 0 {
		return fs.root, nil
	}

	base := &node{}
	err := walk(root, path, func(n *node, path []string) error {
		if len(path) == 0 {
			base = n
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return base, nil
}

func (fs *FS) createHome(uid string) error {
	path, err := fs.newPath()
	if err != nil {
		return err
	}
	n := newNode(fs, uid, uid, uid, 0750|plan9.DMDIR, path, nil)
	fs.root.mu.Lock()
	fs.root.children[uid] = n
	fs.root.mu.Unlock()
	return nil
}

// Attach identifies the user and may select the file tree to access. As
// a result of the attach transaction, the client will have a connection
// to the root directory of the desired file tree, represented by Fid.
func (fs *FS) Attach(uname, aname string) (*Fid, error) {
	user, err := fs.group.Get(uname)
	if err != nil {
		user, _ = fs.group.Get("none")
	}
	uid := user.Name

	aname = path.Clean(aname)
	node, err := fs.walk(aname)
	if err != nil {
		return nil, err
	}
	return &Fid{uid: uid, node: node}, nil
}

// Create asks the file server to create a new file with the name
// supplied, in the directory represented by fid, and requires write
// permission in the directory. The owner of the file is the implied user
// id of the request, the group of the file is the same as dir, and the
// permissions are the value of
//   perm = (perm &^ 0666) | (dir.Mode & 0666)
// if a regular file is being created and
//   perm = (perm &^ 0777) | (dir.Mode & 0777)
// if a directory is being created.
//
// Finally, the newly created file is opened according to mode, and fid
// will represent the newly opened file. Directories are created by
// setting the DMDIR bit (0x80000000) in the perm.
//
// The names . and .. are special; it is illegal to create files with
// these names.
func (fs *FS) Create(name string, mode uint8, perm Perm) (*Fid, error) {
	user, err := fs.group.Get(fs.hostowner)
	if err != nil {
		panic(err) // can't happen
	}
	uid := user.Name

	name = path.Clean(name)
	dname, name := path.Dir(name), path.Base(name)
	dir, err := fs.walk(dname)
	if err != nil {
		return nil, err
	}
	defer dir.Close()

	node, err := dir.Create(uid, name, mode, plan9.Perm(perm))
	if err != nil {
		return nil, err
	}
	return &Fid{uid: uid, node: node}, nil
}

// Open asks the file server to check permissions and prepare a fid for
// I/O with subsequent read and write messages. The mode field determines
// the type of I/O: OREAD, OWRITE, ORDWR, and OEXEC mean read access,
// write access, read and write access, and execute access, to be checked
// against the permissions for the file.
//
// In addition, if mode has the OTRUNC bit set, the file is to be
// truncated, which requires write permission (if the file is
// append–only, and permission is granted, the open succeeds but the file
// will not be truncated); if the mode has the ORCLOSE bit set, the file
// is to be removed when the fid is clunked, which requires permission to
// remove the file from its directory.
//
// It is illegal to write a directory, truncate it, or attempt to remove
// it on close. If the file is marked for exclusive use, only one client
// can have the file open at any time.
func (fs *FS) Open(name string, mode uint8) (*Fid, error) {
	user, err := fs.group.Get(fs.hostowner)
	if err != nil {
		panic(err) // can't happen
	}
	uid := user.Name

	name = path.Clean(name)
	node, err := fs.walk(name)
	if err != nil {
		return nil, err
	}

	fid := &Fid{uid: uid, node: node}
	if err := fid.Open(mode); err != nil {
		return nil, err
	}
	return fid, nil
}

// Remove asks the file server both to remove the file represented by fid
// and to clunk the fid, even if the remove fails.
func (fs *FS) Remove(name string) error {
	user, err := fs.group.Get(fs.hostowner)
	if err != nil {
		panic(err) // can't happen
	}
	uid := user.Name

	name = path.Clean(name)
	node, err := fs.walk(name)
	if err != nil {
		return err
	}

	fid := &Fid{uid: uid, node: node}
	return fid.Remove()
}

// Listen listens on the given network address and then serves incoming
// requests.
func (fs *FS) Listen(network, addr string) error {
	work := make(chan *transaction)
	srv := &server{
		work:    work,
		fs:      fs,
		conn:    uint32(0),
		connmap: make(map[uint32]bool),
	}
	go srv.Listen()

	listener, err := net.Listen(network, addr)
	if err != nil {
		return err
	}

	for {
		rwc, err := listener.Accept()
		if err != nil {
			continue
		}
		connID, err := srv.newConn()
		if err != nil {
			rwc.Close()
			continue
		}

		go func(rwc net.Conn, id uint32) {
			defer srv.delConn(id)
			conn := &conn{
				rwc:    rwc,
				fidnew: fs.fidnew,
				work:   work,
				uid:    "none",
				fidmap: make(map[uint32]*Fid),
			}
			if fs.Log != nil {
				conn.log = fs.Log
			}
			conn.send(conn.recv())
		}(rwc, connID)
	}
}

func split(path string) []string {
	if len(path) == 0 || path == "/" || path == "." {
		return []string{}
	}

	if len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}
	return strings.Split(path, "/")
}

// Copied from http://goplan9.googlecode.com/hg/plan9/dir.go
//   http://godoc.org/code.google.com/p/goplan9/plan9#Perm

// Perm represents file/directory permissions.
type Perm uint32

type permChar struct {
	bit Perm
	c   int
}

var permChars = []permChar{
	permChar{plan9.DMDIR, 'd'},
	permChar{plan9.DMAPPEND, 'a'},
	permChar{plan9.DMAUTH, 'A'},
	permChar{plan9.DMDEVICE, 'D'},
	permChar{plan9.DMSOCKET, 'S'},
	permChar{plan9.DMNAMEDPIPE, 'P'},
	permChar{0, '-'},
	permChar{plan9.DMEXCL, 'l'},
	permChar{plan9.DMSYMLINK, 'L'},
	permChar{0, '-'},
	permChar{0400, 'r'},
	permChar{0, '-'},
	permChar{0200, 'w'},
	permChar{0, '-'},
	permChar{0100, 'x'},
	permChar{0, '-'},
	permChar{0040, 'r'},
	permChar{0, '-'},
	permChar{0020, 'w'},
	permChar{0, '-'},
	permChar{0010, 'x'},
	permChar{0, '-'},
	permChar{0004, 'r'},
	permChar{0, '-'},
	permChar{0002, 'w'},
	permChar{0, '-'},
	permChar{0001, 'x'},
	permChar{0, '-'},
}

func (p Perm) String() string {
	s := ""
	did := false
	for _, pc := range permChars {
		if p&pc.bit != 0 {
			did = true
			s += string(pc.c)
		}
		if pc.bit == 0 {
			if !did {
				s += string(pc.c)
			}
			did = false
		}
	}
	return s
}
