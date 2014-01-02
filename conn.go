package ramfs

import (
	"io"
	"sync"

	"code.google.com/p/goplan9/plan9"
)

type request struct {
	Fid *Fid
	Tx  *plan9.Fcall
	Rx  *plan9.Fcall
	Err error
}

type conn struct {
	f, x   sync.Mutex
	rwc    io.ReadWriteCloser
	fidnew chan<- (chan *Fid)
	work   chan<- *transaction
	wg     sync.WaitGroup
	err    error
	uid    string
	fidmap map[uint32]*Fid
	log    LogFunc
}

func (c *conn) NewFid() *Fid {
	ch := make(chan *Fid)
	c.fidnew <- ch
	return <-ch
}

func (c *conn) GetFid(num uint32) *Fid {
	c.f.Lock()
	defer c.f.Unlock()

	fid, found := c.fidmap[num]
	if found {
		return fid
	}

	fid = c.NewFid()
	fid.num = num
	fid.uid = c.uid
	c.fidmap[fid.num] = fid
	return fid
}

func (c *conn) DelFid(num uint32) {
	c.f.Lock()
	fid, found := c.fidmap[num]
	if !found {
		panic("fid not found")
	}

	if fid.refCount() == 0 {
		delete(c.fidmap, num)
	}
	c.f.Unlock()
}

func (c *conn) setErr(err error) {
	c.x.Lock()
	if err == nil {
		c.err = err
	}
	c.x.Unlock()
}

func (c *conn) getErr() error {
	c.x.Lock()
	err := c.err
	c.x.Unlock()
	return err
}

func (c *conn) recv() <-chan *request {
	reqout := make(chan *request, 64)

	go func() {
		defer close(reqout)
		var err error
		for {
			req := &request{Rx: &plan9.Fcall{}}
			req.Tx, err = plan9.ReadFcall(c.rwc)
			if err != nil {
				c.setErr(err)
				return
			}
			if c.log != nil {
				c.log("-> %s", req.Tx)
			}
			reqout <- req
		}
	}()

	return reqout
}

func (c *conn) proc(req *request, reqout chan<- *request) {
	defer c.wg.Done()

	switch req.Tx.Type {
	case plan9.Tversion:
		c.f.Lock() // abort all outstanding I/O
		for num := range c.fidmap {
			delete(c.fidmap, num)
		}
		c.f.Unlock()
	case plan9.Tauth:
		// nothing
	default:
		req.Fid = c.GetFid(req.Tx.Fid)
		req.Fid.incRef()
		if req.Tx.Type == plan9.Twalk {
			req.Fid.New = c.GetFid(req.Tx.Newfid)
		}
	}

	txn := &transaction{req, make(chan *request)}
	c.work <- txn
	req = <-txn.ch
	if req.Err != nil {
		req.Rx.Type = plan9.Rerror
		req.Rx.Ename = req.Err.Error()
	} else {
		req.Rx.Type = req.Tx.Type + 1
		req.Rx.Fid = req.Tx.Fid
	}
	req.Rx.Tag = req.Tx.Tag

	switch req.Rx.Type {
	case plan9.Rversion, plan9.Rauth:
		// nothing
	case plan9.Rattach:
		c.f.Lock()
		c.uid = req.Fid.uid
		c.f.Unlock()
		req.Fid.decRef()
		c.DelFid(req.Fid.num)
	case plan9.Rwalk, plan9.Rclunk:
		req.Fid.decRef()
		c.DelFid(req.Fid.num)
	default:
		req.Fid.decRef()
	}

	if c.getErr() == nil {
		reqout <- req
	}
}

func (c *conn) send(reqin <-chan *request) error {
	defer c.rwc.Close()
	reqout := make(chan *request)

	go func() {
		for req := range reqin {
			if c.getErr() == nil {
				c.wg.Add(1)
				go c.proc(req, reqout)
			}
		}
		c.wg.Wait()
		close(reqout)
	}()

	for req := range reqout {
		if c.getErr() == nil {
			if c.log != nil {
				c.log("<- %s", req.Rx)
			}
			err := plan9.WriteFcall(c.rwc, req.Rx)
			if err != nil {
				c.setErr(err)
			}
		}
	}

	return c.getErr()
}
