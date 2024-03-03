//go:build windows

package procsmanager

import (
	"os/exec"
	"strconv"
)

func killProcessTree(pid int) error {
	cmd := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid))
	return cmd.Run()
}
