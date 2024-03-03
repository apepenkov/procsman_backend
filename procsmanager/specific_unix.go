//go:build !windows

package procsmanager

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var CLK_TCK int64

func getChildPids(pid int) ([]int, error) {
	pidStr := strconv.Itoa(pid)

	cmd := exec.Command("pgrep", "-P", pidStr)
	output, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			if ee.ExitCode() == 1 {
				return make([]int, 0), nil
			}
		}
		return make([]int, 0), err
	}

	childPids := strings.Split(strings.TrimSpace(string(output)), "\n")
	var pids []int
	for _, cp := range childPids {
		cpid, err := strconv.Atoi(cp)
		if err != nil {
			continue
		}
		pids = append(pids, cpid)
	}

	return pids, nil
}

func killProcessTree(pid int) error {
	childPids, err := getChildPids(pid)
	if err != nil {
		return err
	}
	for _, cp := range childPids {
		err = killProcessTree(cp)
		if err != nil {
			fmt.Printf("Failed to kill child process %d: %v\n", cp, err)
		}
	}

	return syscall.Kill(pid, syscall.SIGKILL)
}

func getUsageInfoUnixRecursive(current *UsageInfo, pid int) error {
	if current == nil {
		return errors.New("current is nil")
	}
	childPids, err := getChildPids(pid)
	if err != nil {
		return err
	}

	for _, cp := range childPids {
		err = getUsageInfoUnixRecursive(current, cp)
		if err != nil {
			return err
		}
	}

	statBytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return err
	}
	fields := strings.Fields(string(statBytes))

	// Extract utime and stime from /proc/[pid]/stat
	utime, err := strconv.ParseInt(fields[13], 10, 64)
	if err != nil {
		return err
	}
	stime, err := strconv.ParseInt(fields[14], 10, 64)
	if err != nil {
		return err
	}

	// Compute total CPU time spent in user and kernel mode
	totalCpuTime := time.Duration(utime+stime) * time.Second / time.Duration(CLK_TCK)

	// Calculate memory usage from /proc/[pid]/statm
	memBytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/statm", pid))
	if err != nil {
		return err
	}
	memPages, err := strconv.ParseInt(strings.Fields(string(memBytes))[1], 10, 64)
	if err != nil {
		return err
	}

	// Update the UsageInfo struct
	current.TotalCpuUsage += totalCpuTime
	current.MemUsage += memPages * int64(os.Getpagesize())

	return nil
}

func (s *SubProcess) getUsageInfoInner() (*UsageInfo, error) {
	usageInfo := &UsageInfo{
		When: UtcNow(),
	}
	return usageInfo, getUsageInfoUnixRecursive(usageInfo, s.Cmd.Process.Pid)
}

func init() {
	cmd := exec.Command("getconf", "CLK_TCK")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		panic(err)
	}
	outStr := strings.TrimSpace(out.String())
	if outStr == "" {
		panic("getconf CLK_TCK returned empty string")
	}
	outStr = strings.Trim(outStr, "\n")
	CLK_TCK, err = strconv.ParseInt(outStr, 10, 64)
	if err != nil {
		panic(err)
	}
}
