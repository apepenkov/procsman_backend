//go:build windows

package procsmanager

import (
	"errors"
	"fmt"
	"github.com/StackExchange/wmi"
	"os/exec"
	"strconv"
	"time"
)

func killProcessTree(pid int) error {
	cmd := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid))
	return cmd.Run()
}

//goland:noinspection SqlResolve,SqlDialectInspection,SqlType
func (s *SubProcess) getUsageInfoInner() (*UsageInfo, error) {
	var rootProcesses []Win32_Process

	// Fetch root process
	query := fmt.Sprintf("SELECT Name, ProcessID, ParentProcessId, UserModeTime, KernelModeTime, PageFileUsage FROM Win32_Process WHERE ProcessID "+"= %d", s.Cmd.Process.Pid)
	err := wmi.Query(query, &rootProcesses)
	if err != nil {
		return nil, err
	}
	if len(rootProcesses) != 1 {
		return nil, errors.New("failed to fetch root process")
	}

	rootProcess := rootProcesses[0]

	// Initialize slice to hold all related processes including the root
	var allProcesses []Win32_Process
	allProcesses = append(allProcesses, rootProcess)

	// Recursive function to fetch child processes
	var fetchChildProcesses func(parentPID uint32) error

	fetchChildProcesses = func(parentPID uint32) error {
		var childProcesses []Win32_Process
		childQuery := fmt.Sprintf("SELECT Name, ProcessID, ParentProcessId, UserModeTime, KernelModeTime, PageFileUsage FROM Win32_Process WHERE ParentProcessId "+"= %d", parentPID)
		childErr := wmi.Query(childQuery, &childProcesses)
		if childErr != nil {
			return childErr
		}

		for _, childProcess := range childProcesses {
			allProcesses = append(allProcesses, childProcess)
			childErr = fetchChildProcesses(childProcess.ProcessID) // Recursively fetch children of this child
			if childErr != nil {
				return childErr
			}
		}
		return nil
	}

	// Start recursion from root process
	err = fetchChildProcesses(rootProcess.ProcessID)
	if err != nil {
		return nil, err
	}

	// Aggregate CPU and memory usage of all processes
	var totalUserModeTime, totalKernelModeTime, totalPageFileUsage uint64
	for _, process := range allProcesses {
		totalUserModeTime += uint64(process.UserModeTime)
		totalKernelModeTime += uint64(process.KernelModeTime)
		totalPageFileUsage += uint64(process.PageFileUsage)
	}

	totalCpuUsageDuration := time.Duration(totalUserModeTime+totalKernelModeTime) * (time.Nanosecond * 100)

	return &UsageInfo{
		TotalCpuUsage: totalCpuUsageDuration,
		MemUsage:      int64(totalPageFileUsage) * 1024,
		When:          UtcNow(),
	}, nil
}
