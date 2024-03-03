package config

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	Db                   string        `json:"db"`
	LogsFolder           string        `json:"logs_folder"`
	LogFileTimespan      time.Duration `json:"log_file_timespan"`
	FlushInterval        time.Duration `json:"flush_interval"`
	ProcessStatsInterval time.Duration `json:"process_stats_interval"`
}

func (c *Config) Validate() error {
	if c.Db == "" {
		return errors.New("empty db")
	}
	if c.LogsFolder == "" {
		return errors.New("empty logs_folder")
	}
	// check that folder exists, writable and is a folder
	info, err := os.Stat(c.LogsFolder)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("logs_folder is not a folder")
	}
	if info.Mode()&0200 == 0 {
		return errors.New("logs_folder is not writable")
	}
	// it also must be executable for the user
	if info.Mode()&0100 == 0 {
		return errors.New("logs_folder is not executable")
	}

	// LogsFolder could have been a relative path, so we need to resolve it to an absolute path
	c.LogsFolder, err = filepath.Abs(c.LogsFolder)
	if err != nil {
		return err
	}

	c.LogFileTimespan = c.LogFileTimespan * time.Second
	c.FlushInterval = c.FlushInterval * time.Millisecond
	c.ProcessStatsInterval = c.ProcessStatsInterval * time.Second

	if c.LogFileTimespan < time.Minute {
		return errors.New("log_file_timespan must be at least 1 minute")
	}

	if c.FlushInterval < time.Millisecond*100 {
		return errors.New("flush_interval must be at least 100 milliseconds")
	}

	if c.ProcessStatsInterval < time.Second {
		return errors.New("process_stats_interval must be at least 1 second")
	}

	return nil
}
