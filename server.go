package ramfs

import (
	"sync"

	"code.google.com/p/goplan9/plan9"
)

const maxConn = uint32(1<<32 - 1)

type transaction struct {
	req *request
	ch  chan *request
}

type server struct {
	mu      sync.Mutex
	work    <-chan *transaction
	conn    uint32
	connmap map[uint32]bool
	fs      *FS
}

func (s *server) newConn() (uint32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for conn := range s.connmap {
		delete(s.connmap, conn)
		return conn, nil
	}

	conn := s.conn
	if s.conn == maxConn {
		return 0, perror("max connection reached")
	}
	s.conn++
	return conn, nil
}

func (s *server) delConn(conn uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connmap[conn] = true
}

func (s *server) Version(fid *Fid, tx, rx *plan9.Fcall) error {
	rx.Version = plan9.VERSION9P
	if tx.Msize < plan9.IOHDRSZ {
		return perror("msize too small")
	}
	if tx.Msize > MSIZE {
		rx.Msize = MSIZE
	} else {
		rx.Msize = tx.Msize
	}
	//if tx.Version != plan9.VERSION9P {
	//	return perror("unknown 9P version")
	//}
	rx.Version = plan9.VERSION9P

	return nil
}

func (s *server) Auth(fid *Fid, tx, rx *plan9.Fcall) error {
	return perror("authentication not required")
}

func (s *server) Attach(fid *Fid, tx, rx *plan9.Fcall) error {
	if tx.Afid != plan9.NOFID {
		return perror("authentication not required")
	}

	root, err := s.fs.Attach(tx.Uname, tx.Aname)
	if err != nil {
		return err
	}
	fid.mu.Lock()
	fid.node = root.node
	fid.uid = root.uid
	fid.mu.Unlock()

	stat := root.node.Stat()
	rx.Qid = stat.Qid
	return nil
}

func (s *server) Clunk(fid *Fid, tx, rx *plan9.Fcall) error {
	fid.Close() // ignore errors
	return nil
}
func (s *server) Flush(fid *Fid, tx, rx *plan9.Fcall) error {
	return nil
}

func (s *server) Walk(fid *Fid, tx, rx *plan9.Fcall) error {
	wqids := make([]plan9.Qid, len(tx.Wname))
	i := 0
	err := fid.Walk(tx.Wname, func(f *Fid, p []string) error {
		wqids[i] = f.node.Stat().Qid
		i++
		return nil
	})
	if err != nil {
		return err
	}

	rx.Wqid = wqids
	return nil
}

func (s *server) Open(fid *Fid, tx, rx *plan9.Fcall) error {
	if err := fid.Open(tx.Mode); err != nil {
		return err
	}

	stat := fid.node.Stat()
	rx.Qid = stat.Qid
	rx.Iounit = IOUNIT
	return nil
}

func (s *server) Create(fid *Fid, tx, rx *plan9.Fcall) error {
	err := fid.Create(tx.Name, tx.Mode, Perm(tx.Perm))
	if err != nil {
		return err
	}

	stat := fid.node.Stat()
	rx.Qid = stat.Qid
	rx.Iounit = IOUNIT
	return nil
}

func (s *server) Read(fid *Fid, tx, rx *plan9.Fcall) error {
	stat := fid.node.Stat()
	if stat.Mode&plan9.DMDIR != 0 {
		if tx.Count > plan9.STATMAX {
			tx.Count = plan9.STATMAX
		}
	}
	data := make([]byte, tx.Count)

	n, err := fid.ReadAt(data, int64(tx.Offset))
	if err != nil {
		return err
	}

	rx.Count = uint32(n)
	rx.Data = data[:n]
	return nil
}

func (s *server) Write(fid *Fid, tx, rx *plan9.Fcall) error {
	n, err := fid.WriteAt(tx.Data, int64(tx.Offset))
	if err != nil {
		return err
	}

	rx.Count = uint32(n)
	return nil
}

func (s *server) Remove(fid *Fid, tx, rx *plan9.Fcall) error {
	fid.Remove() // ignore error
	return s.Clunk(fid, tx, rx)
}

func (s *server) Stat(fid *Fid, tx, rx *plan9.Fcall) error {
	stat := fid.node.Stat()
	data, err := stat.Bytes()
	if err != nil {
		return err
	}

	rx.Stat = data
	return nil
}

func (s *server) Wstat(fid *Fid, tx, rx *plan9.Fcall) error {
	return fid.Wstat(tx.Stat)
}

func (s *server) BadFcall(fid *Fid, tx, rx *plan9.Fcall) error {
	return perror("bad fcall")
}

func (s *server) Listen() {
	for txn := range s.work {
		go func(t *transaction) {
			req := t.req
			fn := s.BadFcall
			switch req.Tx.Type {
			case plan9.Tversion:
				fn = s.Version
			case plan9.Tauth:
				fn = s.Auth
			case plan9.Tattach:
				fn = s.Attach
			case plan9.Tclunk:
				fn = s.Clunk
			case plan9.Tflush:
				fn = s.Flush
			case plan9.Twalk:
				fn = s.Walk
			case plan9.Topen:
				fn = s.Open
			case plan9.Tcreate:
				fn = s.Create
			case plan9.Tread:
				fn = s.Read
			case plan9.Twrite:
				fn = s.Write
			case plan9.Tremove:
				fn = s.Remove
			case plan9.Tstat:
				fn = s.Stat
			case plan9.Twstat:
				fn = s.Wstat
			}
			req.Err = fn(req.Fid, req.Tx, req.Rx)
			t.ch <- req
			close(t.ch)
		}(txn)
	}
}
