package db

import (
	"encoding/json"
	"github.com/jackc/pgx/v5/pgtype"
	"os"
	"time"
)

var DefaultConfiguration = Configuration{
	AutoRestartOnStop:  pgtype.Bool{Valid: true, Bool: true},
	AutoRestartOnCrash: pgtype.Bool{Valid: true, Bool: true},

	AutoRestartMaxRetries:      pgtype.Int4{Valid: true, Int32: 3},
	AutoRestartMaxRetriesFrame: pgtype.Int4{Valid: true, Int32: 60},
	AutoRestartDelay:           pgtype.Int4{Valid: true, Int32: 5000},

	NotifyOnStart:   pgtype.Bool{Valid: true, Bool: true},
	NotifyOnStop:    pgtype.Bool{Valid: true, Bool: true},
	NotifyOnCrash:   pgtype.Bool{Valid: true, Bool: true},
	NotifyOnRestart: pgtype.Bool{Valid: true, Bool: true},

	RecordStats: pgtype.Bool{Valid: true, Bool: true},
	StoreLogs:   pgtype.Bool{Valid: true, Bool: true},
}

type Configuration struct {
	AutoRestartOnStop  pgtype.Bool `json:"auto_restart_on_stop"`
	AutoRestartOnCrash pgtype.Bool `json:"auto_restart_on_crash"`

	// if AutoRestartMaxRetries happens within AutoRestartMaxRetriesFrame, the process will not be restarted
	AutoRestartMaxRetries      pgtype.Int4 `json:"auto_restart_max_retries"`
	AutoRestartMaxRetriesFrame pgtype.Int4 `json:"auto_restart_max_retries_frame"`
	AutoRestartDelay           pgtype.Int4 `json:"auto_restart_delay"`

	NotifyOnStart   pgtype.Bool `json:"notify_on_start"`
	NotifyOnStop    pgtype.Bool `json:"notify_on_stop"`
	NotifyOnCrash   pgtype.Bool `json:"notify_on_crash"`
	NotifyOnRestart pgtype.Bool `json:"notify_on_restart"`

	RecordStats pgtype.Bool `json:"record_stats"`
	StoreLogs   pgtype.Bool `json:"store_logs"`
}

// GetAutoRestartOnStop -> bool
// if true, the process will be restarted if it stops, if autorestart conditions are met
func (c *Configuration) GetAutoRestartOnStop() bool {
	if !c.AutoRestartOnStop.Valid {
		return DefaultConfiguration.AutoRestartOnStop.Bool
	}
	return c.AutoRestartOnStop.Bool
}

// GetAutoRestartOnCrash -> bool
// if true, the process will be restarted if it crashes, if autorestart conditions are met
func (c *Configuration) GetAutoRestartOnCrash() bool {
	if !c.AutoRestartOnCrash.Valid {
		return DefaultConfiguration.AutoRestartOnCrash.Bool
	}
	return c.AutoRestartOnCrash.Bool
}

// GetAutoRestartMaxRetries -> int
// the maximum number of retries to restart the process within AutoRestartMaxRetriesFrame
func (c *Configuration) GetAutoRestartMaxRetries() int {
	if !c.AutoRestartMaxRetries.Valid {
		return int(DefaultConfiguration.AutoRestartMaxRetries.Int32)
	}
	return int(c.AutoRestartMaxRetries.Int32)
}

// GetAutoRestartMaxRetriesFrame -> int (seconds)
// the maximum time frame to restart the process AutoRestartMaxRetries times
func (c *Configuration) GetAutoRestartMaxRetriesFrame() int {
	if !c.AutoRestartMaxRetriesFrame.Valid {
		return int(DefaultConfiguration.AutoRestartMaxRetriesFrame.Int32)
	}
	return int(c.AutoRestartMaxRetriesFrame.Int32)
}

// GetAutoRestartDelay -> time.Duration
// the delay between restarts
func (c *Configuration) GetAutoRestartDelay() time.Duration {
	if !c.AutoRestartDelay.Valid {
		return time.Duration(int(DefaultConfiguration.AutoRestartDelay.Int32)) * time.Millisecond
	}
	return time.Duration(int(c.AutoRestartDelay.Int32)) * time.Millisecond
}

func (c *Configuration) GetNotifyOnStart() bool {
	if !c.NotifyOnStart.Valid {
		return DefaultConfiguration.NotifyOnStart.Bool
	}
	return c.NotifyOnStart.Bool
}

func (c *Configuration) GetNotifyOnRestart() bool {
	if !c.NotifyOnRestart.Valid {
		return DefaultConfiguration.NotifyOnRestart.Bool
	}
	return c.NotifyOnRestart.Bool
}

func (c *Configuration) GetNotifyOnStop() bool {
	if !c.NotifyOnStop.Valid {
		return DefaultConfiguration.NotifyOnStop.Bool
	}
	return c.NotifyOnStop.Bool
}

func (c *Configuration) GetNotifyOnCrash() bool {
	if !c.NotifyOnCrash.Valid {
		return DefaultConfiguration.NotifyOnCrash.Bool
	}
	return c.NotifyOnCrash.Bool
}

func (c *Configuration) GetRecordStats() bool {
	if !c.RecordStats.Valid {
		return DefaultConfiguration.RecordStats.Bool
	}
	return c.RecordStats.Bool
}

func (c *Configuration) GetStoreLogs() bool {
	if !c.StoreLogs.Valid {
		return DefaultConfiguration.StoreLogs.Bool
	}
	return c.StoreLogs.Bool
}

func (c *Configuration) Equal(other Configuration) bool {
	return c.GetAutoRestartOnStop() == other.GetAutoRestartOnStop() &&
		c.GetAutoRestartOnCrash() == other.GetAutoRestartOnCrash() &&
		c.GetAutoRestartMaxRetries() == other.GetAutoRestartMaxRetries() &&
		c.GetAutoRestartMaxRetriesFrame() == other.GetAutoRestartMaxRetriesFrame() &&
		c.GetAutoRestartDelay() == other.GetAutoRestartDelay() &&
		c.GetNotifyOnStart() == other.GetNotifyOnStart() &&
		c.GetNotifyOnRestart() == other.GetNotifyOnRestart() &&
		c.GetNotifyOnStop() == other.GetNotifyOnStop() &&
		c.GetNotifyOnCrash() == other.GetNotifyOnCrash() &&
		c.GetRecordStats() == other.GetRecordStats() &&
		c.GetStoreLogs() == other.GetStoreLogs()
}

func init() {
	defFileName := "default_process_config.json"

	write := func() {
		marshalled, _ := json.MarshalIndent(DefaultConfiguration, "", "  ")
		_ = os.WriteFile(defFileName, marshalled, 0644)

	}
	stat, err := os.Stat(defFileName)
	if err != nil {
		if os.IsNotExist(err) {
			write()
			return
		}
		panic(err)

	} else if stat.IsDir() {
		panic("default_process_config.json is a directory")
	} else if stat.Size() == 0 {
		os.Remove(defFileName)
		write()
	} else {
		// read and unmarshal
		f, err := os.Open(defFileName)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		err = json.NewDecoder(f).Decode(&DefaultConfiguration)
		if err != nil {
			panic(err)
		}
	}

}
