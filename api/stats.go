package api

import (
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"net/http"
	"procsman_backend/db"
	"strconv"
	"time"
)

const MaxProcessStatsTimeFrame = time.Hour * 24 * 30

type StatsResponseCpu struct {
	RecordTime   int64   `json:"record_time"`
	UsagePercent float64 `json:"usage_percent"`
	UsageNs      int64   `json:"usage_ns"`
}

type StatsResponseMemory struct {
	RecordTime int64 `json:"record_time"`
	UsageBytes int64 `json:"usage_bytes"`
}

type StatsResponse struct {
	Cpu    []StatsResponseCpu    `json:"cpu"`
	Memory []StatsResponseMemory `json:"memory"`
}

func (srv *HttpServer) GetProcessStats(w http.ResponseWriter, r *http.Request) {
	rw := r.Context().Value(ContextKeyWrappedRequest).(*ReqWrapper)

	id := r.PathValue("id")
	if id == "" {
		rw.E(MessageCodeNoIdProvided, "no id provided", http.StatusBadRequest, "")
	}

	idInt, err := strconv.Atoi(id)
	if err != nil {
		rw.E(MessageCodeInvalidId, "Invalid id", http.StatusBadRequest, "Could not convert id to int")
		return
	}

	_, err = srv.ProcessManager.Queries.GetProcess(r.Context(), int32(idInt))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			rw.E(MessageCodeProcessNotFound, "Process not found", http.StatusNotFound, "Process not found")
			return
		}
		rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
	}

	from := time.Now().UTC().Add(-1 * time.Hour)
	to := time.Now().UTC()

	if r.URL.Query().Get("from") != "" {
		from, err = time.Parse(time.RFC3339, r.URL.Query().Get("from"))
		if err != nil {
			rw.E(MessageCodeInvalidTimeFrame, "Invalid time frame", http.StatusBadRequest, "Could not parse from time")
			return
		}
	}

	if r.URL.Query().Get("to") != "" {
		to, err = time.Parse(time.RFC3339, r.URL.Query().Get("to"))
		if err != nil {
			rw.E(MessageCodeInvalidTimeFrame, "Invalid time frame", http.StatusBadRequest, "Could not parse to time")
			return
		}
	}

	if to.Sub(from) > MaxProcessStatsTimeFrame {
		rw.E(MessageCodeInvalidTimeFrame, "Invalid time frame", http.StatusBadRequest, "Time frame too large")
		return
	}

	stats, err := srv.ProcessManager.Queries.GetProcessStatsFromTo(r.Context(), db.GetProcessStatsFromToParams{
		ProcessID: pgtype.Int4{
			Int32: int32(idInt),
			Valid: true,
		},
		CreatedAt: pgtype.Timestamp{
			Time:  from,
			Valid: true,
		},
		CreatedAt_2: pgtype.Timestamp{
			Time:  to,
			Valid: true,
		},
	})

	if err != nil {
		rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
		return
	}

	res := StatsResponse{
		Cpu:    make([]StatsResponseCpu, len(stats)),
		Memory: make([]StatsResponseMemory, len(stats)),
	}

	for i, stat := range stats {
		res.Cpu[i] = StatsResponseCpu{
			RecordTime:   stat.CreatedAt.Time.Unix(),
			UsagePercent: stat.CpuUsagePercentage,
			UsageNs:      stat.CpuUsage,
		}
		res.Memory[i] = StatsResponseMemory{
			RecordTime: stat.CreatedAt.Time.Unix(),
			UsageBytes: stat.MemoryUsage,
		}
	}

	rw.MarshalAndRespond(res)
}
