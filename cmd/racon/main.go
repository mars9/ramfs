package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"time"

	"code.google.com/p/goplan9/plan9"
	"code.google.com/p/goplan9/plan9/client"
	"code.google.com/p/snappy-go/snappy"
)

const (
	MSIZE  = 128*1024 + plan9.IOHDRSZ
	IOUNIT = 128 * 1024
)

var (
	addr    = flag.String("addr", "localhost:5640", "service network address")
	network = flag.String("net", "tcp", "connect on the named network")
	mkdir   = flag.Bool("d", false, "make directories")
	long    = flag.Bool("l", false, "use a long listing format")
	uname   = flag.String("uname", os.Getenv("USER"), "username (default: $USER)")
	aname   = flag.String("aname", "", "attach to the file system named aname")
	comp    = flag.Bool("snappy", false, "use snappy en-/decompression")
)

const usageMsg = `
9P2000 client that can access a single file on a 9P2000 server. It can
be useful for manual interaction with a 9P2000 server or for accessing
simple 9P2000 services from within scripts.
`

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [options] cmd [option] args...\n", os.Args[0])
	fmt.Fprint(os.Stderr, usageMsg)
	fmt.Fprintf(os.Stderr, "\nOptions:\n")
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\nCommands:\n")

	max := 0
	names := make([]string, 0)
	help := make(map[string]string)
	for n, c := range cmds {
		name := n
		switch c.arg {
		case 0:
			if len(c.text) != 0 {
				name = fmt.Sprintf("%s %s", n, c.text)
			} else {
				name = fmt.Sprintf("%s", n)
			}
		case 1:
			if len(c.text) != 0 {
				name = fmt.Sprintf("%s %s file", n, c.text)
			} else {
				name = fmt.Sprintf("%s file", n)
			}
		case 2:
			name = fmt.Sprintf("%s %s file", n, c.text)
		case 3:
			if len(c.text) != 0 {
				name = fmt.Sprintf("%s %s file...", n, c.text)
			} else {
				name = fmt.Sprintf("%s file...", n)
			}
		case 4:
			name = fmt.Sprintf("%s %s file...", n, c.text)
		}
		n := len(name)
		if n > max {
			max = n
		}
		names = append(names, name)
		help[name] = c.help
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(os.Stderr, "  %-*s - %s\n", max, n, help[n])
	}
	os.Exit(2)
}

func main() {
	flag.Usage = usage
	flag.Parse()
	if flag.NArg() < 1 {
		flag.Usage()
	}

	name := flag.Arg(0)
	os.Args = flag.Args()
	flag.Parse()
	args := flag.Args()
	xprint := func(code int, format string, a ...interface{}) {
		fmt.Fprintf(os.Stderr, format, a...)
		os.Exit(code)
	}

	cmd, found := cmds[name]
	if !found {
		fmt.Fprintf(os.Stderr, "unknown command %s\n", name)
		os.Exit(2)
	}
	switch cmd.arg {
	case 0, 1, 2:
		if len(args) != cmd.arg {
			switch cmd.arg {
			case 0:
				xprint(2, "%s takes no arguments\n", name)
			case 1:
				xprint(2, "%s requires 1 argument\n", name)
			default:
				xprint(2, "%s requires %d arguments\n", name, cmd.arg)
			}
		}
	case 3:
		if len(args) == 0 {
			xprint(2, "%s requires at least 1 argument\n", name)
		}
	case 4:
		if len(args) == 0 || len(args) == 1 {
			xprint(2, "%s requires at least 2 arguments\n", name)
		}
	}

	if *network == "unix" {
		ns := client.Namespace()
		*addr = fmt.Sprintf("%s%s%s", ns, string(os.PathSeparator), *addr)
	}
	conn, err := client.Dial(*network, *addr)
	if err != nil {
		xprint(1, "%s\n", err.Error())
	}
	defer conn.Close()

	fsys, err := conn.Attach(nil, *uname, "")
	if err != nil {
		xprint(1, "mount: %v\n", err)
	}

	cmd.fn(fsys, args)
	os.Exit(0)
}

type cmd struct {
	fn   func(*client.Fsys, []string)
	arg  int
	text string
	help string
}

var cmds = map[string]cmd{
	"noop":   cmd{noop, 0, "", "send attach request"},
	"create": cmd{create, 3, "[-d]", "make directories or files"},
	"write":  cmd{write, 1, "", "read stdin and write contents to file"},
	"read":   cmd{read, 3, "", "write the contents of file to stdout"},
	"ls":     cmd{readdir, 1, "[-l]", "list contents of directory of file"},
	"stat":   cmd{stat, 3, "", "write status information to stdout"},
	"chgrp":  cmd{chgrp, 4, "group", "change file group"},
	"chmod":  cmd{chmod, 4, "mode", "change file modes"},
	//"rename": cmd{rename, 2, "name", "rename file"},
}

func noop(fs *client.Fsys, args []string) {}

func create(fs *client.Fsys, args []string) {
	var err error
	for _, name := range args {
		fid := &client.Fid{}
		if *mkdir {
			fid, err = fs.Create(name, plan9.OREAD, 0775|plan9.DMDIR)
		} else {
			fid, err = fs.Create(name, plan9.OREAD, 0664)
		}
		fid.Close()

		if err != nil {
			fmt.Fprintf(os.Stderr, "create %s: %v\n", name, err)
		}
	}
}

func write(fs *client.Fsys, args []string) {
	name := args[0]
	data := make([]byte, IOUNIT)
	buf := []byte{}
	offset := int64(0)
	f, err := fs.Open(name, plan9.OWRITE)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", name, err)
		os.Exit(1)
	}
	defer f.Close()

	for {
		n, err := os.Stdin.Read(data)
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Fprintf(os.Stderr, "read stdin: %v\n", err)
			os.Exit(1)
		}

		if *comp {
			buf, err = snappy.Encode(buf, data[0:n])
			if err != nil {
				fmt.Fprintf(os.Stderr, "compress %s: %v", name, err)
				os.Exit(1)
			}
		} else {
			buf = data[0:n]
		}

		n = len(buf)
		m, err := f.WriteAt(buf, offset)
		if err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", name, err)
			os.Exit(1)
		}
		if m != n {
			fmt.Fprintf(os.Stderr, "short write %s: %v\n", name, err)
			os.Exit(1)
		}
		offset += int64(m)
	}
}

func read(fs *client.Fsys, args []string) {
	data := make([]byte, IOUNIT)
	buf := []byte{}

	for _, name := range args {
		f, err := fs.Open(name, plan9.OREAD)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open %s: %v\n", name, err)
			continue
		}

		offset := int64(0)
		for {
			n, err := f.ReadAt(data, offset)
			if err != nil {
				if err == io.EOF {
					break
				}
				fmt.Fprintf(os.Stderr, "read %s: %v\n", name, err)
				f.Close()
				continue
			}

			offset += int64(n)

			if *comp {
				buf, err = snappy.Decode(buf, data[0:n])
				if err != nil {
					fmt.Fprintf(os.Stderr, "decompress %s: %v", name, err)
					os.Exit(1)
				}
			} else {
				buf = data[0:n]
			}

			if _, err = os.Stdout.Write(buf); err != nil {
				fmt.Fprintf(os.Stderr, "write stdout: %v", err)
				os.Exit(1)
			}
		}
		f.Close()
	}
}

func stat(fs *client.Fsys, args []string) {
	for _, name := range args {
		d, err := fs.Stat(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "stat %s: %v\n", name, err)
			continue
		}
		fmt.Printf("%s\n", d)
	}
}

func chgrp(fs *client.Fsys, args []string) {
	for _, name := range args[1:] {
		d, err := fs.Stat(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "stat %s: %v\n", name, err)
			return
		}
		d.Gid = args[0]
		if err = fs.Wstat(name, d); err != nil {
			fmt.Fprintf(os.Stderr, "wstat %s: %v\n", name, err)
			return
		}
	}
}

func chmod(fs *client.Fsys, args []string) {
	mode, err := strconv.ParseInt(args[0], 8, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "chmod: %v\n", err)
		return
	}
	for _, name := range args[1:] {
		d, err := fs.Stat(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "stat %s: %v\n", name, err)
			return
		}
		d.Mode = plan9.Perm(mode)
		if err = fs.Wstat(name, d); err != nil {
			fmt.Fprintf(os.Stderr, "wstat %s: %v\n", name, err)
			return
		}
	}
}

// TODO
//func rename(fs *client.Fsys, args []string) {}

func timeStamp(dtime uint32) string {
	now := time.Now()
	mtime := time.Unix(int64(dtime), 0)
	layout := "Jan 06 15:04"
	if mtime.Year() < now.Year() {
		layout = "Jan 06 2006"
	}
	return mtime.Format(layout)
}

type byName []*plan9.Dir

func (p byName) Len() int           { return len(p) }
func (p byName) Less(i, j int) bool { return p[i].Name < p[j].Name }
func (p byName) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func readdir(fs *client.Fsys, args []string) {
	name := args[0]
	fi, err := fs.Stat(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stat %s: %v\n", name, err)
		os.Exit(1)
	}

	f, err := fs.Open(name, plan9.OREAD)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", name, err)
		os.Exit(1)
	}
	defer f.Close()

	if fi.Mode&plan9.DMDIR != 0 {
		dirs, err := f.Dirreadall()
		if err != nil {
			fmt.Fprintf(os.Stderr, "dirread %s: %v\n", name, err)
			os.Exit(1)
		}
		sort.Sort(byName(dirs))

		lengths := make([]string, len(dirs))
		vers := make([]string, len(dirs))
		maxLen := 0
		maxVers := 0
		maxUid := 0
		maxGid := 0
		for i, d := range dirs {
			n := len(d.Uid)
			if n > maxUid {
				maxUid = n
			}
			n = len(d.Gid)
			if n > maxGid {
				maxGid = n
			}
			lengths[i] = fmt.Sprintf("%d", d.Length)
			n = len(lengths[i])
			if n > maxLen {
				maxLen = n
			}
			vers[i] = fmt.Sprintf("%d", d.Qid.Vers)
			n = len(vers[i])
			if n > maxVers {
				maxVers = n
			}
		}

		for i, d := range dirs {
			if d.Mode&plan9.DMDIR != 0 {
				d.Name = d.Name + "/"
			}
			if *long {
				fmt.Printf("%s %-*s %-*s %-*s %-*s %s  %s\n",
					d.Mode,
					maxVers, vers[i],
					maxUid, d.Uid,
					maxGid, d.Gid,
					maxLen, lengths[i],
					timeStamp(d.Mtime),
					d.Name,
				)
			} else {
				fmt.Printf("%s\n", d.Name)
			}
		}
	} else {
		d, err := fs.Stat(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "stat %s: %v\n", name, err)
			os.Exit(1)
		}
		if *long {
			length := fmt.Sprintf("%d", d.Length)
			vers := fmt.Sprintf("%d", d.Qid.Vers)
			fmt.Printf("%s %-*s %-*s %-*s %-*s %s  %s\n",
				d.Mode,
				len(vers), vers,
				len(d.Uid), d.Uid,
				len(d.Gid), d.Gid,
				len(length), length,
				timeStamp(d.Mtime),
				d.Name,
			)
		} else {
			fmt.Printf("%s\n", d.Name)
		}
	}
}
