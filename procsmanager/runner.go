package procsmanager

import (
	"context"
	"errors"
	"fmt"
	"github.com/apepenkov/yalog"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"procsman_backend/db"
	"runtime"
	"strings"
	"sync"
	"time"
)

// clocksPerSecond will only be used on Unix systems.
var dummyWriter = NewDummyWriter()

func UtcNow() time.Time {
	return time.Now().UTC()
}

type ProcessLogger struct {
	Process    *ProcessRunner
	CurrentLog *db.Log
	FileWriter *os.File
	LastFlush  time.Time

	mu sync.Mutex
}

// flush writes the current buffer to the file and updates LastFlush
func (pl *ProcessLogger) flush() error {
	if pl.FileWriter == nil {
		return nil
	}
	err := pl.FileWriter.Sync()
	if err != nil {
		return err
	}
	pl.LastFlush = UtcNow()
	return nil
}

// closeFile closes the current file and sets FileWriter to nil.
// It only affects the file, not the database entry.
func (pl *ProcessLogger) closeFile() error {
	if pl.FileWriter == nil {
		return nil
	}
	if err := pl.flush(); err != nil {
		return err
	}

	if err := pl.FileWriter.Close(); err != nil {
		return err
	}

	pl.FileWriter = nil
	return nil
}

// newLog creates a new file for the process and sets FileWriter to the new file.
// It also adds a new entry to the database.
func (pl *ProcessLogger) newLog() error {
	if pl.FileWriter != nil {
		if err := pl.closeFile(); err != nil {
			return err
		}
	}
	var err error
	var tx pgx.Tx
	var queries *db.Queries
	tx, queries, err = pl.Process.Manager.OpenTx(context.Background())
	if err != nil {
		return err
	}

	defer func() {
		if tx != nil {
			if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				pl.Process.Manager.Logger.Error("Failed to rollback transaction", "error", err)
			}
		}
	}()

	newLogFolder := filepath.Join(pl.Process.Manager.Config.LogsFolder, fmt.Sprintf("%d", pl.Process.Process.ID))
	if err := os.MkdirAll(newLogFolder, 0755); err != nil {
		return err
	}
	newLogPath := filepath.Join(newLogFolder, fmt.Sprintf("%d.procLog", UtcNow().Unix()))

	var newLog db.Log
	newLog, err = queries.NewProcessLogFile(context.Background(), db.NewProcessLogFileParams{
		ProcessID: pgtype.Int4{Int32: pl.Process.Process.ID, Valid: true},
		Path:      newLogPath,
	})

	if err != nil {
		return err
	}

	pl.CurrentLog = &newLog
	pl.FileWriter, err = os.Create(newLogPath)
	if err != nil {
		return err
	}
	pl.LastFlush = UtcNow()

	if err := tx.Commit(context.Background()); err != nil {
		return err
	}
	return nil
}

// FinishLog closes the current file and sets FileWriter to nil.
// It also sets end_time in the database entry. After that neither the file, nor procLog entry should be used.
func (pl *ProcessLogger) FinishLog() error {
	//pl.mu.Lock()
	//defer pl.mu.Unlock()

	if pl.FileWriter == nil {
		return nil
	}
	if err := pl.closeFile(); err != nil {
		return err
	}
	var tx pgx.Tx
	var queries *db.Queries
	var err error
	tx, queries, err = pl.Process.Manager.OpenTx(context.Background())
	if err != nil {
		return err
	}

	defer func() {
		if tx != nil {
			if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				pl.Process.Manager.Logger.Error("Failed to rollback transaction", "error", err)
			}
		}
	}()

	if pl.CurrentLog == nil {
		return errors.New("tried to finish procLog, but no current procLog set")
	}

	err = queries.SetLogEndTime(context.Background(), db.SetLogEndTimeParams{
		ID: pl.CurrentLog.ID,
		EndTime: pgtype.Timestamp{
			Valid: true,
			Time:  UtcNow(),
		},
	})
	if err != nil {
		return err
	}

	if err := tx.Commit(context.Background()); err != nil {
		return err
	}
	tx = nil

	return nil
}

func (pl *ProcessLogger) FinishLogOnDelete() error {
	if pl.CurrentLog == nil {
		return nil
	}
	if pl.FileWriter != nil {
		if err := pl.closeFile(); err != nil {
			return err
		}
	}
	return os.RemoveAll(filepath.Dir(pl.CurrentLog.Path))
}

// retrieveCurrentLog finds the current procLog for the process and sets FileWriter to the file.
// If there is no current procLog, it creates a new one.
func (pl *ProcessLogger) retrieveCurrentLog() error {
	var err error
	var tx pgx.Tx
	var queries *db.Queries
	tx, queries, err = pl.Process.Manager.OpenTx(context.Background())
	if err != nil {
		return err
	}

	defer func() {
		if tx != nil {
			if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				pl.Process.Manager.Logger.Error("Failed to rollback transaction", "error", err)
			}
		}
	}()

	var lastLog db.Log
	newLog := false

	lastLog, err = queries.LastProcessLogFile(context.Background(), pgtype.Int4{Int32: pl.Process.Process.ID, Valid: true})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		newLog = true
		pl.CurrentLog = nil
	} else {
		if lastLog.StartTime.Time.Before(UtcNow().Add(-pl.Process.Manager.Config.LogFileTimespan)) {
			// close the procLog
			if !lastLog.EndTime.Valid {
				if err = queries.SetLogEndTime(context.Background(), db.SetLogEndTimeParams{
					ID: lastLog.ID,
					EndTime: pgtype.Timestamp{
						Valid: true,
						Time:  UtcNow(),
					},
				}); err != nil {
					return err
				}
			}

			newLog = true
		}
		pl.CurrentLog = &lastLog
	}
redo:
	if newLog {
		if err = pl.newLog(); err != nil {
			return err
		}
	} else {
		pl.CurrentLog = &lastLog
		pl.FileWriter, err = os.OpenFile(lastLog.Path, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				if err = pl.FinishLog(); err != nil {
					return err
				}
				newLog = true
				goto redo
			}
			return err
		}
	}
	if err = tx.Commit(context.Background()); err != nil {
		return err
	}
	tx = nil
	return nil
}

// shouldBeClosed checks if the current procLog should be closed.

func (pl *ProcessLogger) shouldBeClosed() bool {
	if pl.CurrentLog == nil {
		return false
	}
	if pl.CurrentLog.StartTime.Time.Before(UtcNow().Add(-pl.Process.Manager.Config.LogFileTimespan)) {
		return true
	}
	return false
}

// cycle checks if the current procLog is still valid and creates a new one if it's not.
// It also checks if the procLog file is too old and closes it if it is.
// It should be called periodically.
// It is also called in initialisation to set the current procLog.
func (pl *ProcessLogger) cycle() error {
	if !pl.Process.Process.Configuration.GetStoreLogs() {
		return nil
	}
	pl.mu.Lock()
	defer pl.mu.Unlock()

	if pl.CurrentLog == nil {
		return pl.retrieveCurrentLog()
	}

	if pl.shouldBeClosed() {
		//pl.mu.Unlock()
		err := pl.FinishLog()
		//pl.mu.Lock()
		if err != nil {
			return err
		}

		return pl.retrieveCurrentLog()
	}

	if UtcNow().After(pl.LastFlush.Add(pl.Process.Manager.Config.FlushInterval)) {
		return pl.flush()
	}

	return nil
}

// Write writes the given string to the current procLog file.

func (pl *ProcessLogger) Write(b []byte) (int, error) {
	if pl.FileWriter == nil {
		if err := pl.retrieveCurrentLog(); err != nil {
			return 0, err
		}
	}
	n, err := pl.FileWriter.Write(b)
	if err != nil {
		return n, err
	}
	return n, err
}

func (pl *ProcessLogger) Close() error {
	// noop, because we manage the file ourselves.
	return nil
}

type Signal int

// Stop 	stops the process 	and closes the procLog file. Status will be set to 							STOPPING -> STOPPED
// Start	starts the process, if it's stopped. Status will be set to 									STARTING -> RUNNING
// Restart 	stops the process, 	close the procLog file and start the process again. Status will be set to	STOPPING -> STOPPED -> STARTING -> RUNNING.
// Deleted 	stops the process, 	delete all logs. Process is expected to be deleted from the database by the caller.
// Refresh  re-fetches process configuration from the database and updates the process runner.
const (
	Stop Signal = iota
	Start
	Restart
	Deleted
	Refresh
)

func (s Signal) String() string {
	switch s {
	case Stop:
		return "Stop"
	case Start:
		return "Start"
	case Restart:
		return "Restart"
	case Deleted:
		return "Deleted"
	case Refresh:
		return "Refresh"
	default:
		return fmt.Sprintf("Unknown Signal (%d)", s)
	}
}

// Other status descriptions:
// CRASHED: The process has crashed and is not running. It will not be restarted.
// STOPPED_WILL_RESTART: The process has stopped and will be restarted.
// CRASHED_WILL_RESTART: The process has crashed and will be restarted.
// UNKNOWN: The process status is unknown. A default status for a new process. Should not be set manually.

type ProcessRunner struct {
	Manager *ProcessManager
	Process *db.Process
	Logger  *yalog.Logger

	SignalIn chan Signal

	StdIn chan string

	status  db.ProcessStatus
	procLog *ProcessLogger

	// stoppedByUser is set when a stop signal is received.
	// it will be set to false after .Wait()
	stoppedByUser bool
}

func NewProcessRunner(manager *ProcessManager, process *db.Process) *ProcessRunner {
	runner := &ProcessRunner{
		Manager:  manager,
		Process:  process,
		SignalIn: make(chan Signal, 2),
		StdIn:    make(chan string, 2),
		status:   process.Status,
		Logger:   manager.Logger.NewLogger(fmt.Sprintf("pr-%d", process.ID)),
	}
	runner.procLog = &ProcessLogger{
		Process: runner,
	}
	return runner
}

func (pr *ProcessRunner) SetStatus(status db.ProcessStatus) error {
	pr.Logger.Debugf("Setting status to %s\n", status)
	err := pr.Manager.Queries.SetProcessStatus(context.Background(), db.SetProcessStatusParams{
		ID:     pr.Process.ID,
		Status: status,
	})
	if err != nil {
		pr.Logger.Errorf("Failed to set status %v: %v\n", status, err)
		return err
	}
	pr.Process.Status = status
	pr.status = status
	return nil
}

func (pr *ProcessRunner) GetCmd() string {
	return pr.Process.ExecutablePath + " " + pr.Process.Arguments
}

func (pr *ProcessRunner) LogEvent(eventType db.ProcessEventType, extra []byte) error {
	notify := func(text string) {
		res := pr.Manager.Notifications.SendMessage(text)
		for _, r := range res {
			if r.Success {
				continue
			}
			pr.Logger.Warningf("Failed to send notification: %v\n", r.Error)
		}
	}
	switch eventType {
	case db.ProcessEventTypeSTART:
		if pr.Process.Configuration.GetNotifyOnStart() {
			go notify(fmt.Sprintf("Process %s has started", pr.Process.Name))
		}
	case db.ProcessEventTypeSTOP:
		if pr.Process.Configuration.GetNotifyOnStop() {
			go notify(fmt.Sprintf("Process %s has stopped", pr.Process.Name))
		}
	case db.ProcessEventTypeCRASH:
		if pr.Process.Configuration.GetNotifyOnCrash() {
			go notify(fmt.Sprintf("Process %s has crashed", pr.Process.Name))
		}
	case db.ProcessEventTypeFULLSTOP:
		if pr.Process.Configuration.GetNotifyOnStop() {
			go notify(fmt.Sprintf("Process %s has fully stopped", pr.Process.Name))
		}
	case db.ProcessEventTypeFULLCRASH:
		if pr.Process.Configuration.GetNotifyOnCrash() {
			go notify(fmt.Sprintf("Process %s has fully crashed", pr.Process.Name))
		}
	case db.ProcessEventTypeMANUALLYSTOPPED:
		if pr.Process.Configuration.GetNotifyOnStop() {
			go notify(fmt.Sprintf("Process %s has been manually stopped", pr.Process.Name))
		}
	case db.ProcessEventTypeRESTART:
		if pr.Process.Configuration.GetNotifyOnRestart() {
			go notify(fmt.Sprintf("Process %s has been restarted", pr.Process.Name))
		}

	}
	_, err := pr.Manager.Queries.InsertProcessEvent(context.Background(), db.InsertProcessEventParams{
		ProcessID:      pgtype.Int4{Int32: pr.Process.ID, Valid: true},
		Event:          eventType,
		AdditionalInfo: extra,
	})
	if err != nil {
		pr.Logger.Errorf("Failed to procLog event %v: %v\n", eventType, err)
	}
	return err
}

type UsageInfo struct {
	// TotalCpuUsage is a total CPU usage by the process.
	// MemUsage is a total memory usage in B.
	TotalCpuUsage time.Duration
	MemUsage      int64
	When          time.Time
}

// SubProcess is a wrapper for exec.Cmd and its stdin.

type SubProcess struct {
	Cmd       *exec.Cmd
	Stdin     io.WriteCloser
	LastUsage *UsageInfo
}

//goland:noinspection GoSnakeCaseUsage
type Win32_Process struct {
	Name            string
	ProcessID       uint32
	ParentProcessId uint32
	UserModeTime    uint64
	KernelModeTime  uint64
	PageFileUsage   uint32
}

func (s *SubProcess) getUsageInfo() (*UsageInfo, error) {
	if s.Cmd.ProcessState != nil && s.Cmd.ProcessState.Exited() || s.Cmd.Process == nil || s.Cmd.Process.Pid == 0 {
		return nil, errors.New("process has exited")
	}
	return s.getUsageInfoInner()
}

type UsageRecord struct {
	CpuUsage        time.Duration
	CpuUsagePercent float64
	MemUsage        int64
	When            time.Time
	Delta           time.Duration
}

// getUsageRecording saves current usage info in database.
// due to the CPU info being recorded by delta, we can't record the first usage info.
func (s *SubProcess) getUsageRecording() (*UsageRecord, error) {
	var err error
	if s.LastUsage == nil {
		s.LastUsage, err = s.getUsageInfo()
		if err != nil {
			return nil, err
		}
		return nil, nil
	}

	newUsage, err := s.getUsageInfo()
	if err != nil {
		return nil, err
	}

	cpuUsageDelta := newUsage.TotalCpuUsage - s.LastUsage.TotalCpuUsage
	timeDelta := newUsage.When.Sub(s.LastUsage.When)

	totalCpuNano := time.Duration(runtime.NumCPU()) * timeDelta

	s.LastUsage = newUsage

	return &UsageRecord{
		CpuUsage:        cpuUsageDelta,
		CpuUsagePercent: (float64(cpuUsageDelta) / float64(totalCpuNano)) * 100.0,
		MemUsage:        newUsage.MemUsage,
		When:            newUsage.When,
		Delta:           timeDelta,
	}, nil
}

func (s *SubProcess) Cleanup() {
	if s.Cmd != nil {
		if s.Cmd.Process != nil {
			_ = killProcessTree(s.Cmd.Process.Pid)
			_ = s.Cmd.Process.Kill()
			_ = s.Cmd.Process.Release()
		}
	}
	//if s.Stdin != nil {
	//	_ = s.Stdin.Close()
	//}
	return
}

//goland:noinspection GoBoolExpressions
func ResolvePath(source string) (string, error) {
	info, err := os.Stat(source)
	if err == nil {
		if info.IsDir() {
			return "", errors.New("source is a directory")
		}
		if runtime.GOOS != "windows" && info.Mode()&0111 == 0 {
			return "", errors.New("source is not executable")
		}
		return source, nil
	}
	fullPath, err := exec.LookPath(source)
	if err != nil {
		return "", err
	}
	info, err = os.Stat(fullPath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", errors.New("source is a directory")
	}
	if runtime.GOOS != "windows" && info.Mode()&0111 == 0 {
		return "", errors.New("source is not executable")
	}
	return fullPath, nil
}

func (pr *ProcessRunner) NewSubProcess(program string, argsStr string, envVars map[string]string, workingDirectory string) (*SubProcess, error) {
	allGood := false
	var proc *SubProcess
	defer func() {
		if !allGood {
			if proc != nil {
				proc.Cleanup()
			}
		}
	}()

	programPath, err := ResolvePath(program)
	if err != nil {
		return nil, err
	}

	args := strings.Fields(argsStr)

	cmd := exec.Command(programPath, args...)
	cmd.Env = os.Environ()
	cmd.Dir = workingDirectory
	if envVars != nil {
		for k, v := range envVars {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	//stdout, err := cmd.StdoutPipe()
	//if err != nil {
	//	return nil, err
	//}
	//stderr, err := cmd.StderrPipe()
	//if err != nil {
	//	return nil, err
	//}

	// Unfortunately, when using StdoutPipe and StderrPipe, we can't force pipes to flush, so we can't get the output in real time.
	// So we have to manage them ourselves (including closing them).

	if !pr.Process.Configuration.GetStoreLogs() {
		cmd.Stdout = dummyWriter
		cmd.Stderr = dummyWriter
	} else {
		cmd.Stdout = pr.procLog
		cmd.Stderr = pr.procLog
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	proc = &SubProcess{
		Cmd:   cmd,
		Stdin: stdin,
	}
	allGood = true
	return proc, nil
}

func (pr *ProcessRunner) StopRestartFrameSatisfied() bool {
	if pr.Process.Configuration.GetAutoRestartMaxRetriesFrame() == 0 {
		return true
	}

	events, err := pr.Manager.Queries.GetProcessEventsAfter(context.Background(), db.GetProcessEventsAfterParams{
		ProcessID: pgtype.Int4{Int32: pr.Process.ID, Valid: true},
		CreatedAt: pgtype.Timestamp{
			Time:  UtcNow().Add(-(time.Second * time.Duration(pr.Process.Configuration.GetAutoRestartMaxRetriesFrame()))),
			Valid: true,
		},
	})
	if err != nil {
		pr.Logger.Errorf("Failed to get process events: %v\n", err)
		return false
	}

	eventCount := 0
	for _, event := range events {
		if event.Event == db.ProcessEventTypeCRASH || event.Event == db.ProcessEventTypeSTOP {
			eventCount++
		}

	}

	return eventCount < pr.Process.Configuration.GetAutoRestartMaxRetries()
}

func (pr *ProcessRunner) RecordUsage(subprocess *SubProcess) (bool, error) {
	if !pr.Process.Configuration.GetRecordStats() {
		return false, nil
	}

	if subprocess.LastUsage != nil {
		passed := UtcNow().Sub(subprocess.LastUsage.When)
		if passed < pr.Manager.Config.ProcessStatsInterval {
			return false, nil
		}
	}

	record, err := subprocess.getUsageRecording()
	if err != nil {
		return false, err
	}
	if record == nil {
		return false, nil
	}

	roundedToThreeDecimals := float64(int(record.CpuUsagePercent*1000)) / 1000

	_, err = pr.Manager.Queries.InsertProcessStats(context.Background(), db.InsertProcessStatsParams{
		ProcessID: pgtype.Int4{
			Int32: pr.Process.ID,
			Valid: true,
		},
		CpuUsage:           record.CpuUsage.Nanoseconds(),
		CpuUsagePercentage: roundedToThreeDecimals,
		MemoryUsage:        record.MemUsage,
	})

	return err == nil, err
}

// Work will be called in goroutine to start the process, handle IO, restart if needed, procLog events etc.
// It contains a loop, that does not lock up thanks to timers, channels, etc.
// It should be called in a goroutine.
// procLog.cycle() will be called periodically to check if the procLog file is still valid and create a new one if it's not, if process is running.
func (pr *ProcessRunner) Work() {
	var subprocess *SubProcess
	var processErr error

	// Ensure resources are properly released on function exit.
	defer func() {
		if subprocess != nil {
			subprocess.Cleanup()
		}
		if pr.procLog != nil {
			_ = pr.procLog.FinishLog()
		}
	}()

	stopIfExists := func() {
		if subprocess != nil {
			subprocess.Cleanup()
		}
	}

	if pr.Process.Enabled {
		pr.SignalIn <- Start
	}

	_ = pr.SetStatus(db.ProcessStatusUNKNOWN)
	_ = pr.procLog.cycle()

	for {
		select {
		case signal := <-pr.SignalIn:
			pr.Logger.Debugf("Received signal: %s\n", signal)
			switch signal {
			case Start:
				if pr.status != db.ProcessStatusRUNNING && pr.status != db.ProcessStatusSTARTING {
					_ = pr.SetStatus(db.ProcessStatusSTARTING)
					stopIfExists()
					subprocess, processErr = pr.startProcess(true)
					if processErr != nil {
						pr.Logger.Errorf("Failed to start process: %v\n", processErr)
						_ = pr.SetStatus(db.ProcessStatusCRASHED)
						continue
					}
					_ = pr.SetStatus(db.ProcessStatusRUNNING)
				}

			case Stop, Deleted:
				if signal == Stop {
					_ = pr.SetStatus(db.ProcessStatusSTOPPING)
				}

				if subprocess != nil {
					subprocess.Cleanup()
					subprocess = nil
					pr.stoppedByUser = true
				}

				if signal == Deleted {
					if err := pr.procLog.FinishLogOnDelete(); err != nil {
						pr.Logger.Errorf("Error finishing procLog: %v\n", err)
					}
					_ = os.RemoveAll(filepath.Join(pr.Manager.Config.LogsFolder, fmt.Sprintf("%d", pr.Process.ID)))
					return
				} else {
					_ = pr.LogEvent(db.ProcessEventTypeMANUALLYSTOPPED, nil)
					_ = pr.SetStatus(db.ProcessStatusSTOPPED)
				}

			case Restart:
				_ = pr.SetStatus(db.ProcessStatusSTOPPING)
				_ = pr.LogEvent(db.ProcessEventTypeRESTART, nil)
				pr.stoppedByUser = true
				stopIfExists()
				sleepFor := pr.Process.Configuration.GetAutoRestartDelay()
				if sleepFor > 0 {
					pr.Logger.Debugf("Sleeping for %s before restarting\n", sleepFor)
					time.Sleep(sleepFor)
				}
				_ = pr.SetStatus(db.ProcessStatusSTARTING)
				subprocess, processErr = pr.startProcess(false)
				if processErr != nil {
					pr.Logger.Errorf("Failed to restart process: %v\n", processErr)
					_ = pr.SetStatus(db.ProcessStatusCRASHED)
					continue
				}
				_ = pr.SetStatus(db.ProcessStatusRUNNING)

			case Refresh:
				pr.stoppedByUser = true
				proc, err := pr.Manager.Queries.GetProcess(context.Background(), pr.Process.ID)
				if err != nil {
					pr.Logger.Errorf("Failed to refresh process: %v\n", err)
					return
				}
				pr.Process = &proc
				if pr.Process.Enabled {
					if err = pr.procLog.cycle(); err != nil {
						pr.Logger.Errorf("Error cycling procLog: %v\n", err)
					}
					pr.SignalIn <- Restart
				}
			}

		// Handle other cases like periodic procLog flushing or external shutdown signals.
		default:
			if pr.status == db.ProcessStatusSTOPPEDWILLRESTART || pr.status == db.ProcessStatusCRASHEDWILLRESTART {
				pr.Logger.Infof("Restarting process based on configuration\n")
				pr.SignalIn <- Restart
			}

			// Implement procLog cycling and flushing based on conditions
			if err := pr.procLog.cycle(); err != nil {
				pr.Logger.Errorf("Error cycling procLog: %v\n", err)
			}

			if subprocess != nil && pr.status == db.ProcessStatusRUNNING && pr.Process.Configuration.GetRecordStats() {
				if _, err := pr.RecordUsage(subprocess); err != nil {
					pr.Logger.Errorf("Error recording usage: %v\n", err)
				}
			}

			// Avoid busy waiting
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (pr *ProcessRunner) startProcess(reportStart bool) (*SubProcess, error) {
	pr.stoppedByUser = false
	cmdStr, argsStr := pr.Process.ExecutablePath, pr.Process.Arguments
	subprocess, err := pr.NewSubProcess(cmdStr, argsStr, pr.Process.Environment, pr.Process.WorkingDirectory)
	if err != nil {
		return nil, err
	}

	if err = subprocess.Cmd.Start(); err != nil {
		if subprocess != nil {
			subprocess.Cleanup()
		}
		return nil, err
	}

	// Start goroutines to handle subprocess stdout and stderr
	go pr.handleStdIn(subprocess.Stdin)

	if reportStart {
		_ = pr.LogEvent(db.ProcessEventTypeSTART, nil)
	}

	go pr.waitForProcessExit(subprocess)
	return subprocess, nil
}

func (pr *ProcessRunner) handleStdIn(stdin io.WriteCloser) {
	for input := range pr.StdIn {
		if _, err := stdin.Write([]byte(input + "\n")); err != nil {
			pr.Logger.Errorf("Error writing to stdin: %v\n", err)
			return
		}
	}
}

func (pr *ProcessRunner) waitForProcessExit(subprocess *SubProcess) {
	err := subprocess.Cmd.Wait()

	if pr.status == db.ProcessStatusSTOPPING {
		pr.Logger.Debugln("Process is in stopping state, not doing anything in waitForProcessExit")
		return
	}

	wasStoppedByUser := pr.stoppedByUser
	pr.stoppedByUser = false

	// Ensure thread-safe access to pr.status
	pr.Manager.Logger.Debugln("Process exited, checking status and deciding on auto-restart...")
	pr.procLog.flush()

	finish := func(isStop bool, tryRestart bool) {
		if isStop {
			if tryRestart && pr.Process.Configuration.GetAutoRestartOnStop() && pr.StopRestartFrameSatisfied() {
				_ = pr.SetStatus(db.ProcessStatusSTOPPEDWILLRESTART)
				_ = pr.LogEvent(db.ProcessEventTypeSTOP, nil)
			} else {
				_ = pr.SetStatus(db.ProcessStatusSTOPPED)
				_ = pr.LogEvent(db.ProcessEventTypeFULLSTOP, nil)
			}
		} else {
			if tryRestart && pr.Process.Configuration.GetAutoRestartOnCrash() && pr.StopRestartFrameSatisfied() {
				_ = pr.SetStatus(db.ProcessStatusCRASHEDWILLRESTART)
				_ = pr.LogEvent(db.ProcessEventTypeCRASH, nil)
			} else {
				_ = pr.SetStatus(db.ProcessStatusCRASHED)
				_ = pr.LogEvent(db.ProcessEventTypeFULLCRASH, nil)
			}
		}
	}

	if wasStoppedByUser {
		pr.Logger.Debugf("Process was stopped by user\n")
		finish(true, false)
		return
	}

	if err != nil {
		var exitError *exec.ExitError
		var syscallError *os.SyscallError
		if errors.As(err, &exitError) {
			// Process exited with a non-zero status (i.e., crashed)
			pr.Logger.Errorf("Process crashed: %v\n", err)
			finish(false, true)
		} else if errors.As(err, &syscallError) {
			// Most likely it was stopped by us (user)
			pr.Logger.Errorf("Process crashed (syscall): %v\n", err)
			finish(true, false)
		} else {
			pr.Logger.Errorf("Process crashed (unknown error): %v\n", err)
			finish(false, true)
		}
	} else {
		finish(true, true)
	}
}
