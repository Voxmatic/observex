//go:build linux
// +build linux

package main

import "syscall"

func statfs(path string, stat *syscall_Statfs) error {
	var s syscall.Statfs_t
	if err := syscall.Statfs(path, &s); err != nil {
		return err
	}
	stat.Bsize = s.Bsize
	stat.Blocks = s.Blocks
	stat.Bfree = s.Bfree
	stat.Bavail = s.Bavail
	return nil
}
