// +build !linux

package main

import "errors"

func mount(network, addr, uname, mntpt string) error {
	return errors.New("mount not supported")
}
