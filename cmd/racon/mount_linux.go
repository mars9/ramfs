package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"
)

func mount(network, addr, uname, mntpt string) error {
	uid, err := user.Lookup(uname)
	if err != nil {
		return err
	}

	elem := strings.SplitN(addr, ":", 2)
	addr = elem[0]
	port := "564"
	if len(elem) == 2 {
		port = elem[1]
	}

	opts := fmt.Sprintf("%s,trans=%s,name=%s,uname=%s,uid=%s,gid=%s,dfltuid=%s,"+
		"dfltgid=%s,access=%s,port=%s,noextend,nodev",
		network, network, uid.Name, uid.Name, uid.Uid, uid.Gid, uid.Uid,
		uid.Gid, uid.Uid, port)

	// could use syscall.Mount here
	// syscall.Mount(addr, mntpt, "9p", 0, opts)

	sudo, err := exec.LookPath("sudo")
	if err != nil {
		sudo = ""
	}

	mount, err := exec.LookPath("mount")
	if err != nil {
		return err
	}

	cmd := exec.Command(sudo, mount, "-t", "9p", "-o", opts, addr, mntpt)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err = cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}
