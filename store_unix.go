//go:build linux || darwin || freebsd

package main

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func lockDirectory(path string) (*os.File, error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		f.Close()
		return nil, e
	}
	return f, nil
}
func replaceFile(from, to string) error {
	d, e := os.Open(filepath.Dir(to))
	if e != nil {
		return e
	}
	defer d.Close()
	if e = os.Rename(from, to); e != nil {
		return e
	}
	return d.Sync()
}
