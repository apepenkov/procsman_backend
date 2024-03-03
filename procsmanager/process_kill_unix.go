//go:build !windows

package procsmanager

import (
	"syscall"
)

func killProcessTree(pid int) error {
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		return err
	}
	return syscall.Kill(-pgid, syscall.SIGTERM) // Kill the entire process group
}
