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

type ProcessEvent struct {
	Event db.ProcessEventType `json:"event"`
	Time  int64               `json:"time"`
}

type EventsResponse struct {
	Events []ProcessEvent `json:"events"`
}

func (srv *HttpServer) GetProcessEvents(w http.ResponseWriter, r *http.Request) {
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

	from := time.Unix(0, 0)
	to := time.Now().UTC()
	limit := 0x7fffffff

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

	if r.URL.Query().Get("limit") != "" {
		limit, err = strconv.Atoi(r.URL.Query().Get("limit"))
		if err != nil {
			rw.E(MessageCodeInvalidLimit, "Invalid limit", http.StatusBadRequest, "Could not convert limit to int")
			return
		}

	}

	stats, err := srv.ProcessManager.Queries.GetProcessEventsFromTo(r.Context(), db.GetProcessEventsFromToParams{
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
		Limit: int32(limit),
	})

	if err != nil {
		rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
		return
	}

	res := EventsResponse{
		Events: make([]ProcessEvent, len(stats)),
	}
	for i, s := range stats {
		res.Events[i] = ProcessEvent{
			Event: s.Event,
			Time:  s.CreatedAt.Time.Unix(),
		}
	}

	rw.MarshalAndRespond(res)
}
