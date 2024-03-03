package procsmanager

import (
	"context"
	"github.com/apepenkov/yalog"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"procsman_backend/config"
	"procsman_backend/db"
	"sync"
	"time"
)

type ProcessManager struct {
	Queries       *db.Queries
	Db            *pgxpool.Pool
	Logger        *yalog.Logger
	Config        *config.Config
	Notifications *config.NotificationsConfig

	runners      map[int32]*ProcessRunner
	runnersMutex sync.RWMutex
}

func NewProcessManager(cfg config.Config, logger *yalog.Logger) (*ProcessManager, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.Db)
	if err != nil {
		return nil, err
	}
	poolConfig.MaxConns = 40
	poolConfig.MaxConnLifetime = 5 * time.Minute
	poolConfig.MaxConnIdleTime = 5 * time.Minute
	poolConfig.HealthCheckPeriod = 5 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	d, err := pgxpool.NewWithConfig(ctx, poolConfig)
	cancel()
	if err != nil {
		return nil, err
	}
	notif, err := config.LoadOrCreateNotificationsConfig()
	if err != nil {
		return nil, err
	}
	pm := &ProcessManager{
		Queries:       db.New(d),
		Db:            d,
		Logger:        logger,
		Config:        &cfg,
		Notifications: notif,
	}
	processes, err := pm.Queries.GetProcesses(context.Background())
	if err != nil {
		return nil, err
	}
	for _, process := range processes {
		p := process
		go pm.AddRunner(&p).Work()

	}
	return pm, nil
}

func (pm *ProcessManager) Close() {
	pm.Db.Close()
}

func (pm *ProcessManager) OpenTx(ctx context.Context) (pgx.Tx, *db.Queries, error) {
	tx, err := pm.Db.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	return tx, pm.Queries.WithTx(tx), nil
}

func (pm *ProcessManager) AddRunner(process *db.Process) *ProcessRunner {
	pm.runnersMutex.Lock()
	defer pm.runnersMutex.Unlock()
	if pm.runners == nil {
		pm.runners = make(map[int32]*ProcessRunner)
	}
	pm.runners[process.ID] = NewProcessRunner(pm, process)
	return pm.runners[process.ID]
}

func (pm *ProcessManager) RemoveRunner(processID int32) {
	pm.runnersMutex.Lock()
	defer pm.runnersMutex.Unlock()
	delete(pm.runners, processID)
}

func (pm *ProcessManager) GetRunner(processID int32) *ProcessRunner {
	pm.runnersMutex.RLock()
	defer pm.runnersMutex.RUnlock()
	return pm.runners[processID]
}
