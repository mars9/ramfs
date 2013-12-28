package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/mars9/ramfs"
)

const usageMsg = `
Ramfs starts a 9P2000 file server keeping all files in memory. The
filesystem is entirely maintained in memory, no external storage is
used. File data is allocated in 2 MiB blocks.

The root of the filesystem is owned by the user who invoked ramfs and
is created with Read, Write and Execute permissions for the owner and
Read and Execute permissions for everyone else (0755). Ramfs create
the necessary directories and files in /adm/ctl, /adm/group and
/<hostowner>.
`

func main() {
	addr := flag.String("addr", "localhost:5640", "service listen address")
	network := flag.String("net", "tcp", "stream-oriented network")
	owner := flag.String("hostowner", os.Getenv("USER"), "hostowner (default: $USER)")
	chatty := flag.Bool("D", false, "print each 9P2000 message to stdout")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", os.Args[0])
		fmt.Fprint(os.Stderr, usageMsg)
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
		os.Exit(2)
	}
	flag.Parse()

	fs := ramfs.New(*owner)
	if *chatty {
		log.SetFlags(log.Ldate | log.Lmicroseconds)
		fs.Log = log.Printf
	}

	if err := fs.Listen(*network, *addr); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", os.Args[0], err)
		os.Exit(1)
	}
	os.Exit(0)
}
