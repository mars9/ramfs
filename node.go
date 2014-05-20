package ramfs

import (
	"sync"
	"time"

	"code.google.com/p/goplan9/plan9"
)

var errPerm = perror("permission denied")

type node struct {
	mu       sync.RWMutex
	fs       *FS
	file     buffer
	dir      *plan9.Dir
	parent   *node
	children map[string]*node
	open     bool // used for OEXCL
	orclose  bool
}

func newNode(fs *FS, name, uid, gid string, perm plan9.Perm, path uint64, b buffer) *node {
	now := uint32(time.Now().Unix())
	n := &node{
		fs: fs,
		dir: &plan9.Dir{
			Qid: plan9.Qid{
				Type: uint8(perm >> 24),
				Vers: uint32(0),
				Path: path,
			},
			Mode:   perm,      // permission
			Atime:  now,       // last read time
			Mtime:  now,       // last write time
			Length: uint64(0), // node length
			Name:   name,      // last element of path
			Uid:    uid,       // owner name
			Gid:    gid,       // group name
			Muid:   uid,       // last modifier name
		},
	}
	if perm&plan9.DMDIR != 0 {
		n.children = make(map[string]*node)
	} else {
		n.file = b
	}
	return n
}

func (n *node) Create(uid, name string, mode uint8, perm plan9.Perm) (*node, error) {
	if name == "." || name == ".." {
		return nil, perror("illegal name")
	}

	if perm&plan9.DMDIR != 0 {
		perm = (perm &^ 0777) | (n.dir.Mode & 0777)
	} else {
		perm = (perm &^ 0666) | (n.dir.Mode & 0666)
	}

	n.mu.Lock()

	if n.dir.Mode&plan9.DMDIR == 0 {
		n.mu.Unlock()
		return nil, perror("not a directory")
	}
	if n.dir.Mode&plan9.DMEXCL != 0 && n.open {
		n.mu.Unlock()
		return nil, perror("exclusive use file already open")
	}

	path, err := n.fs.newPath()
	if err != nil {
		n.mu.Unlock()
		return nil, err
	}
	node := newNode(n.fs, name, uid, n.dir.Gid, perm, path, newFile(BLOCKSIZE))
	node.parent = n

	if f, found := n.children[name]; found {
		n.mu.Unlock()
		if err := f.Open(mode); err != nil {
			return nil, err
		}
		return f, nil

	}
	n.children[name] = node

	n.mu.Unlock()
	return node, nil
}

func (n *node) Open(mode uint8) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.dir.Mode&plan9.DMEXCL != 0 && n.open {
		return perror("exclusive use file already open")
	}
	if n.dir.Mode&plan9.DMEXCL != 0 && !n.open {
		n.open = true
	}
	if mode&plan9.ORCLOSE != 0 {
		n.orclose = true
	}
	return nil
}

func (n *node) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.dir.Mode&plan9.DMEXCL != 0 && n.open {
		n.open = false
	}
	if n.dir.Mode&plan9.DMDIR == 0 {
		n.file.Close()
	}
	if n.orclose {
		return n.remove()
	}
	return nil
}

func (n *node) remove() error {
	if n.dir.Mode&plan9.DMDIR != 0 && len(n.children) != 0 {
		return perror("directory not empty")
	}

	parent := n.parent
	parent.mu.Lock()
	name := n.dir.Name
	if _, found := parent.children[name]; !found {
		parent.mu.Unlock()
		return perror("file does not exist")
	}
	delete(parent.children, name)
	parent.mu.Unlock()

	n.fs.delPath(n.dir.Qid.Path)
	return nil
}

func (n *node) Remove() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.remove()
}

func (n *node) WriteAt(p []byte, offset int64) (int, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.dir.Mode&plan9.DMDIR != 0 {
		return 0, perror("is a directory")
	}
	if n.dir.Mode&plan9.DMAPPEND != 0 {
		n := n.file.Len()
		if n > uint64(1<<63-1) { // TODO
			return 0, perror("offset overflow")
		}
		offset = int64(n)
	}

	m, err := n.file.WriteAt(p, offset)
	if err != nil {
		return 0, err
	}

	now := uint32(time.Now().Unix())
	n.dir.Atime = now
	n.dir.Mtime = now
	n.dir.Length = n.file.Len()
	if n.dir.Mode&plan9.DMTMP == 0 {
		n.dir.Qid.Vers++
	}
	return m, nil
}

func (n *node) ReadAt(p []byte, offset int64) (int, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.dir.Mode&plan9.DMDIR != 0 {
		return 0, perror("is a directory")
	}

	m, err := n.file.ReadAt(p, offset)
	if err != nil {
		return 0, err
	}

	n.dir.Atime = uint32(time.Now().Unix())
	return m, nil
}

func (n *node) Readdir() ([]byte, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.dir.Mode&plan9.DMDIR == 0 {
		return nil, perror("not a directory")
	}

	var data []byte
	for _, f := range n.children {
		buf, err := f.dir.Bytes()
		if err != nil {
			return nil, err
		}
		data = append(data, buf...)
	}
	return data, nil
}

func (n *node) Stat() *plan9.Dir {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.dir
}

func (n *node) Wstat(uname string, dir *plan9.Dir) error {
	// To change mode, must be owner or group leader. Because of lack of
	// group file, leader=>group itself.
	if dir.Mode != 0xFFFFFFFF && dir.Mode != n.dir.Mode {
		if uname != n.dir.Uid && uname != n.dir.Gid {
			return perror("not owner")
		}
	}

	// To change name, must have write permission in parent and name must
	// be unique.
	parent := n.parent
	if dir.Name != "" && dir.Name != n.dir.Name {
		if !parent.HasPerm(uname, plan9.DMWRITE) {
			return errPerm
		}

		parent.mu.Lock()
		if _, found := parent.children[dir.Name]; found {
			parent.mu.Unlock()
			return perror("file exists")
		}
		parent.mu.Unlock()
	}

	// To change group, must be owner and member of new group
	if dir.Gid != "" && dir.Gid != n.dir.Gid {
		fgroup, err := n.fs.group.Get(n.dir.Gid)
		if err != nil {
			panic(err) // can't happen
		}
		if _, found := fgroup.Member[uname]; !found {
			return perror("not owner")
		}
	}

	// all ok; do it
	if dir.Mode != 0xFFFFFFFF && dir.Mode != n.dir.Mode {
		if dir.Mode&plan9.DMDIR != 0 {
			n.dir.Mode = (dir.Mode &^ 0777) | (n.dir.Mode & 0777)
		} else {
			n.dir.Mode = (dir.Mode &^ 0666) | (n.dir.Mode & 0666)
		}
	}
	if dir.Name != "" && dir.Name != n.dir.Name {
		parent.mu.Lock()
		delete(parent.children, n.dir.Name)

		n.mu.Lock()
		n.dir.Name = dir.Name
		n.mu.Unlock()

		parent.children[dir.Name] = n
		parent.mu.Unlock()
	}
	if dir.Gid != "" && dir.Gid != n.dir.Gid {
		n.mu.Lock()
		n.dir.Gid = dir.Gid
		n.mu.Unlock()
	}
	return nil
}

func (n *node) HasPerm(uname string, perm plan9.Perm) bool {
	other := plan9.Perm(7)
	perm &= other

	// other
	fperm := n.dir.Mode & other
	if uname == "none" && (fperm&perm) == perm {
		return true
	}

	if _, err := n.fs.group.Get(uname); err == nil {
		// user
		if n.dir.Uid == uname {
			user := plan9.Perm(6)
			fperm |= (n.dir.Mode >> user) & other
		}
		if (fperm & perm) == perm {
			return true
		}

		// group
		fgroup, err := n.fs.group.Get(n.dir.Gid)
		if err != nil {
			panic(err) // can't happen
		}
		if _, found := fgroup.Member[uname]; found {
			group := plan9.Perm(3)
			fperm |= (n.dir.Mode >> group) & other
		}
		return (fperm & perm) == perm
	}

	return false
}

type walkFunc func(root *node, path []string) error

func walk(root *node, path []string, fn walkFunc) error {
	if len(path) == 0 {
		return nil
	}

	node := root
	name, path := path[0], path[1:]
	if name == ".." {
		node = node.parent
	} else {
		n, found := node.children[name]
		if found {
			node = n
		} else {
			return perror("file does not exist")
		}
	}

	stat := node.Stat()
	if (stat.Type & plan9.QTDIR) > 0 {
		if (stat.Mode & plan9.DMEXEC) > 0 {
			return errPerm
		}
	}

	if err := fn(node, path); err != nil {
		return err
	}
	return walk(node, path, fn)
}
