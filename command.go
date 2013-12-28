package ramfs

import (
	"bytes"
	"io"
	"sync"
)

type member map[string]bool

type user struct {
	Name   string
	Leader string
	Member member
}

func (u user) Bytes() []byte {
	member := ""
	for m, _ := range u.Member {
		member += (m + ",")
	}
	if member != "" {
		member = member[:len(member)-1]
	}

	uid := u.Name
	return []byte(uid + ":" + uid + ":" + u.Leader + ":" + member)
}

type groupmap map[string]user

func (g groupmap) UserAdd(uid string) error {
	if _, found := g[uid]; found {
		return perror("user " + uid + " exists")
	}
	g[uid] = user{uid, uid, member{}}
	return nil
}

func (g groupmap) GroupAdd(uid string, gid ...string) error {
	if _, found := g[uid]; !found {
		return perror("user " + uid + " not found")
	}
	for _, groupId := range gid {
		if _, found := g[groupId]; !found {
			return perror("group " + groupId + " not found")
		}
	}
	for _, groupId := range gid {
		if groupId == uid {
			continue
		}
		g[groupId].Member[uid] = true
	}
	return nil
}

func (g groupmap) Exist(uid string) bool {
	_, found := g[uid]
	return found
}

func (g groupmap) Bytes() []byte {
	buf := make([][]byte, len(g))
	i := 0
	n := 0
	for _, user := range g {
		buf[i] = user.Bytes()
		n += len(buf[i])
		i++
	}
	data := make([]byte, n+len(g))
	i = 0
	n = 0
	for _, b := range buf {
		n += copy(data[i:i+len(b)], b)
		i = i + len(b)
		n += copy(data[i:i+1], []byte("\n"))
		i++
	}
	return data[:n]
}

type command struct {
	Name string
	Args []string
}

type group struct {
	mu       sync.RWMutex
	groupmap groupmap
}

func newGroup(owner string) *group {
	return &group{groupmap: groupmap{
		"adm":  user{"adm", "adm", member{owner: true}},
		"none": user{"none", "none", member{}},
		owner:  user{owner, owner, member{}},
	}}
}

func (f *group) Get(uid string) (user, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	user, found := f.groupmap[uid]
	if !found {
		return user, perror("user " + uid + " not found")
	}
	return user, nil
}

func (f *group) ReadAt(p []byte, offset int64) (int, error) {
	if offset < 0 {
		return 0, perror("negative offset")
	}

	f.mu.RLock()
	data, err := marshal(f.groupmap)
	if err != nil {
		f.mu.RUnlock()
		return 0, err
	}
	f.mu.RUnlock()

	if offset > int64(len(data)) {
		return 0, io.EOF
	}
	return copy(p, data[offset:]), nil
}

func (f *group) WriteAt(p []byte, offset int64) (int, error) {
	var err error
	cmd := command{}
	if err = unmarshal(p, &cmd); err != nil {
		return 0, err
	}
	if cmd.Name != "uname" {
		return 0, perror("invalid command " + cmd.Name)
	}
	if len(cmd.Args) != 2 {
		return 0, perror("uname requires 2 arguments")
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case len(cmd.Args[1]) > 1 && cmd.Args[1][0] == '+':
		err = f.groupmap.GroupAdd(cmd.Args[0], cmd.Args[1][1:])
	case cmd.Args[0] == cmd.Args[1]:
		err = f.groupmap.UserAdd(cmd.Args[0])
	case len(cmd.Args[1]) > 1 && cmd.Args[1][0] == ':':
		err = f.groupmap.UserAdd(cmd.Args[0])
	default:
		err = perror("invalid command")
	}
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (f *group) Len() uint64  { return uint64(0) }
func (f *group) Close() error { return nil }

type ctl struct {
	fs *FS
}

func newCtl(fs *FS) *ctl { return &ctl{fs: fs} }

func (f *ctl) ReadAt(p []byte, offset int64) (int, error) {
	return 0, perror("reading ctl file")
}

func (f *ctl) WriteAt(p []byte, offset int64) (int, error) {
	var err error
	cmd := command{}
	if err = unmarshal(p, &cmd); err != nil {
		return 0, err
	}

	switch cmd.Name {
	case "listen":
		if len(cmd.Args) != 2 {
			return 0, perror("listen requires 2 arguments")
		}
		go f.fs.Listen(cmd.Args[0], cmd.Args[1])
	default:
		return 0, perror("invalid command " + cmd.Name)
	}
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (f *ctl) Len() uint64  { return uint64(0) }
func (f *ctl) Close() error { return nil }

var (
	userSep   = []byte(":")
	memberSep = []byte(",")
	groupSep  = []byte("\n")
)

func unmarshal(data []byte, v interface{}) error {
	if _, ok := v.(*command); ok {
		bad := func(b byte) bool {
			switch b {
			case ' ', '\t', '\n', '\r':
				return true
			default:
				return false
			}
		}
		nelem := true
		args := make([][]byte, 64)
		n := 0
		m := 0
		i := 0
		for _, c := range data {
			switch {
			case bad(c) && nelem:
				continue
			case bad(c) && !nelem:
				args[i] = args[i][0:m]
				nelem = true
				continue
			}
			if nelem {
				if n >= 64 {
					return perror("too many arguments")
				}
				args[n] = make([]byte, 64)
				nelem = false
				i = n
				n++
				m = 0
			}
			if m == 64 {
				return perror("argument too long")
			}
			args[i][m] = c
			m++
		}
		if !nelem {
			args[i] = args[i][0:m]
		}
		if n == 0 {
			return perror("command name missing")
		}

		v.(*command).Name = string(args[0])
		v.(*command).Args = make([]string, n-1)
		if n > 1 {
			for i, a := range args[1:n] {
				v.(*command).Args[i] = string(a)
			}
		}
		return nil
	}

	if groupmap, ok := v.(groupmap); ok {
		groups := bytes.Split(data, groupSep)
		for _, g := range groups {
			if len(g) == 0 {
				continue
			}
			elem := make([][]byte, 4)
			elem = bytes.SplitN(g, userSep, 4)

			member := member{}
			if len(elem) == 4 {
				mem := bytes.Split(elem[3], memberSep)
				for _, m := range mem {
					member[string(m)] = true
				}
			}
			groupmap[string(elem[0])] = user{
				string(elem[1]),
				string(elem[2]),
				member,
			}
		}
		return nil
	}
	panic("unsupported type")
}

func marshal(v interface{}) ([]byte, error) {
	if user, ok := v.(user); ok {
		return user.Bytes(), nil
	}
	if groupmap, ok := v.(groupmap); ok {
		return groupmap.Bytes(), nil
	}
	panic("unsupported type")
}
