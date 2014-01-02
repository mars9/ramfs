package ramfs

import (
	"sync"

	"code.google.com/p/goplan9/plan9"
)

// The Fid type identifies a file on the file server. A new Fid is
// created when the user attaches to the file server, or when Walk-ing to
// a file. The Fid values are created automatically by the ramfs
// implementation.
type Fid struct {
	mu     sync.RWMutex
	num    uint32
	uid    string
	node   *node
	opened bool
	buf    []byte // used for Dirread
	ref    uint16
	New    *Fid
}

func (f *Fid) incRef() {
	f.mu.Lock()
	f.ref++
	f.mu.Unlock()
}

func (f *Fid) decRef() uint16 {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ref == 0 {
		return 0
	}
	f.ref--
	return f.ref
}

func (f *Fid) refCount() uint16 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.ref
}

func (f *Fid) isOpen() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.opened
}

type WalkFunc func(fid *Fid, path []string) error

func (f *Fid) Walk(name []string, fn WalkFunc) error {
	if len(name) > plan9.MAXWELEM {
		return perror("too many names in walk")
	}

	f.New.node = f.node
	return walk(f.node, name, func(n *node, p []string) error {
		f.New.node = n
		return fn(f.New, p)
	})
}

// Close informs the file server that the current file represented by fid
// is no longer needed by the client.
func (f *Fid) Close() error {
	if !f.isOpen() {
		return perror("file not open for I/O")
	}
	if f.node.dir.Mode&plan9.ORCLOSE != 0 {
		parent := f.node.parent
		if !f.node.HasPerm(f.uid, plan9.DMWRITE) {
			return errPerm
		}
		if !parent.HasPerm(f.uid, plan9.DMWRITE) {
			return errPerm
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.opened = false
	return f.node.Close()
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
func (f *Fid) Create(name string, mode uint8, perm Perm) error {
	if !f.node.HasPerm(f.uid, plan9.Perm(perm)) {
		return errPerm
	}

	node, err := f.node.Create(f.uid, name, mode, plan9.Perm(perm))
	if err != nil {
		return err
	}

	f.mu.Lock()
	f.node = node
	f.opened = true
	f.mu.Unlock()
	return nil
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
func (f *Fid) Open(mode uint8) error {
	if f.isOpen() {
		return perror("file already open for I/O")
	}

	perm := plan9.Perm(0)
	switch mode & 3 {
	case plan9.OREAD:
		perm = plan9.DMREAD
	case plan9.OWRITE:
		perm = plan9.DMWRITE
	case plan9.ORDWR:
		perm = plan9.DMREAD | plan9.DMWRITE
	}
	if (mode & plan9.OTRUNC) != 0 {
		perm |= plan9.DMWRITE
	}

	if !f.node.HasPerm(f.uid, plan9.Perm(perm)) {
		return errPerm
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.opened = true
	return f.node.Open(mode)
}

// Remove asks the file server both to remove the file represented by fid
// and to clunk the fid, even if the remove fails.
func (f *Fid) Remove() error {
	if !f.isOpen() {
		return perror("file not open for I/O")
	}

	parent := f.node.parent
	if !f.node.HasPerm(f.uid, plan9.DMWRITE) {
		return errPerm
	}
	if !parent.HasPerm(f.uid, plan9.DMWRITE) {
		return errPerm
	}

	if err := f.node.Remove(); err != nil {
		return err
	}
	return nil
}

// ReadAt asks for len(p) bytes of data from the file identified by fid,
// which must be opened for reading, starting offset bytes after the
// beginning of the file.
//
// For directories, ReadAt returns an integral number of directory
// entries exactly as in stat, one for each member of the directory.
func (f *Fid) ReadAt(p []byte, offset int64) (int, error) {
	if !f.isOpen() {
		return 0, perror("file not open for I/O")
	}

	stat := f.node.Stat()
	var err error
	if stat.Mode&plan9.DMDIR != 0 {
		if offset == 0 {
			f.buf, err = f.node.Readdir()
			if err != nil {
				return 0, err
			}
		}

		n := copy(p, f.buf)
		f.buf = f.buf[n:]
		return n, nil
	}
	return f.node.ReadAt(p, offset)
}

// WriteAt asks that len(p) bytes of data be recorded in the file
// identified by fid, which must be opened for writing, starting offset
// bytes after the beginning of the file. If the file is append–only, the
// data will be placed at the end of the file regardless of offset.
// Directories may not be written.
//
// WriteAt records the number of bytes actually written. It is usually an
// error if this is not the same as requested.
func (f *Fid) WriteAt(p []byte, offset int64) (int, error) {
	if !f.isOpen() {
		return 0, perror("file not open for I/O")
	}

	stat := f.node.Stat()
	if stat.Mode&plan9.DMDIR != 0 {
		return 0, perror("is a directory")
	}
	return f.node.WriteAt(p, offset)
}

// Stat inquires about the file identified by fid. The reply will contain
// a machine-independent directory entry
func (f *Fid) Stat() ([]byte, error) {
	return f.node.Stat().Bytes()
}

// Wstat can change some of the file status information. The name can be
// changed by anyone with write permission in the parent directory; it is
// an error to change the name to that of an existing file.
//
// The mode can be changed by the owner of the file or the group leader
// of the file's current group. The directory bit cannot be changed by a
// wstat; the other defined permission and mode bits can. The gid can be
// changed: by the owner if also a member of the new group; or by the
// group leader of the file's current group if also leader of the new
// group.
//
// Either all the changes in Wstat request happen, or none of them does:
// if the request succeeds, all changes were made; if it fails, none
// were.
func (f *Fid) Wstat(data []byte) error {
	stat, err := plan9.UnmarshalDir(data)
	if err != nil {
		return err
	}
	return f.node.Wstat(f.uid, stat)
}
