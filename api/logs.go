package api

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"procsman_backend/db"
	"strconv"
	"time"
)

const WriteRepeatedThreshold = 20

// copyAndMarkRepeatedLines reads from src and writes to dst, marking repeated lines.
func copyAndMarkRepeatedLines(dst io.Writer, src io.Reader) (int64, error) {
	reader := bufio.NewReader(src)
	writer := bufio.NewWriter(dst)

	var totalWritten int64
	lastLine := ""
	currentLine := ""
	count := 1
	isFirstLine := true

	for {
		chunk, isPrefix, err := reader.ReadLine()
		if err != nil && err != io.EOF {
			return 0, err
		}
		if err == io.EOF {
			break
		}

		currentLine += string(chunk)
		if isPrefix {
			continue
		}

		if isFirstLine {
			lastLine = currentLine
			isFirstLine = false
			currentLine = ""
			continue
		}

		if currentLine == lastLine {
			count++
		} else {
			if count > WriteRepeatedThreshold {
				if _, err := writer.WriteString(fmt.Sprintf("%s\n{Last line repeated %d times}\n", lastLine, count)); err != nil {
					return 0, err
				}
			} else {
				for i := 0; i < count; i++ {
					if _, err := writer.WriteString(lastLine + "\n"); err != nil {
						return 0, err
					}
				}
			}
			lastLine = currentLine
			count = 1
		}

		currentLine = ""
	}

	// After reading all chunks, check if there is a line that needs to be written out
	if count > WriteRepeatedThreshold {
		if _, err := writer.WriteString(fmt.Sprintf("{Last line repeated %d times}\n", count)); err != nil {
			return 0, err
		}
	} else {
		for i := 0; i < count; i++ {
			if _, err := writer.WriteString(lastLine + "\n"); err != nil {
				return 0, err
			}
		}
	}

	if err := writer.Flush(); err != nil {
		return 0, err
	}

	return totalWritten, nil
}

type LogPiece struct {
	From    int64  `json:"from"`
	To      int64  `json:"to"`
	Text    string `json:"text"`
	Missing bool   `json:"missing"`
}

type LogsResponse struct {
	Logs []LogPiece `json:"logs"`
}

func (srv *HttpServer) GetProcessLogs(w http.ResponseWriter, r *http.Request) {
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

	from := time.Now().UTC().Add(-24 * time.Hour)
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

	logs, err := srv.ProcessManager.Queries.GetLogFilesFromTo(r.Context(), db.GetLogFilesFromToParams{
		ProcessID: pgtype.Int4{
			Int32: int32(idInt),
			Valid: true,
		},
		StartTime: pgtype.Timestamp{
			Time:  from.UTC(),
			Valid: true,
		},
		EndTime: pgtype.Timestamp{
			Time:  to.UTC(),
			Valid: true,
		},
	})

	if err != nil {
		rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
		return
	}

	res := LogsResponse{
		Logs: make([]LogPiece, len(logs)),
	}

	for i, log := range logs {
		res.Logs[i].From = log.StartTime.Time.Unix()
		if log.EndTime.Valid {
			res.Logs[i].To = log.EndTime.Time.Unix()
		}

		f, err := os.OpenFile(log.Path, os.O_RDONLY, 0)
		if err != nil {
			res.Logs[i].Missing = true
		} else {
			// create an empty Writer
			logsWriter := new(bytes.Buffer)
			_, err = copyAndMarkRepeatedLines(logsWriter, f)
			if err != nil {
				res.Logs[i].Missing = true
			}
			res.Logs[i].Text = logsWriter.String()
			if len(res.Logs[i].Text) == 1 {
				res.Logs[i].Text = ""
			}
		}

	}

	rw.MarshalAndRespond(res)
}

type StdInRequest struct {
	Text string `json:"text"`
}

func (s *StdInRequest) Validate(ctx context.Context, srv *HttpServer) *Error {
	if s.Text == "" {
		return MakeE(MessageCodeTextRequired, "Text required", http.StatusBadRequest, "")
	}
	return nil
}

func (srv *HttpServer) PostStdin(w http.ResponseWriter, r *http.Request) {
	rw := r.Context().Value(ContextKeyWrappedRequest).(*ReqWrapper)
	req := r.Context().Value(ContextKeyUnmarshalledJson).(*StdInRequest)

	id := r.PathValue("id")
	if id == "" {
		rw.E(MessageCodeNoIdProvided, "No id provided", http.StatusBadRequest, "No id provided")
		return
	}
	idInt, err := strconv.Atoi(id)
	if err != nil {
		rw.E(MessageCodeInvalidId, "Invalid id", http.StatusBadRequest, "Invalid id")
		return
	}

	process, err := srv.ProcessManager.Queries.GetProcess(r.Context(), int32(idInt))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			rw.E(MessageCodeProcessNotFound, "Process not found", http.StatusNotFound, "Process not found")
			return
		}
		rw.E(MessageCodeErrorGettingProcess, "Error getting process", http.StatusInternalServerError, err.Error())
		return
	}

	if !process.Enabled || process.Status != db.ProcessStatusRUNNING {
		rw.E(MessageCodeProcessNotRunning, "process is not running", http.StatusBadRequest, "")
		return
	}

	runner := srv.ProcessManager.GetRunner(int32(idInt))
	if runner == nil {
		srv.Logger.Errorf("Process %d's runner is nil\n", idInt)
		rw.E(MessageCodeInternalError, "internal server error", http.StatusInternalServerError, "runner is nil")
		return
	}

	runner.StdIn <- req.Text
	_ = rw.WriteHeader(http.StatusAccepted)
}

func (srv *HttpServer) ExportLogsAsZip(w http.ResponseWriter, r *http.Request) {
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

	from := time.Now().UTC().Add(-24 * time.Hour)
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

	logs, err := srv.ProcessManager.Queries.GetLogFilesFromTo(r.Context(), db.GetLogFilesFromToParams{
		ProcessID: pgtype.Int4{
			Int32: int32(idInt),
			Valid: true,
		},
		StartTime: pgtype.Timestamp{
			Time:  from.UTC(),
			Valid: true,
		},
		EndTime: pgtype.Timestamp{
			Time:  to.UTC(),
			Valid: true,
		},
	})

	if err != nil {
		rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
		return
	}

	tmpFile, err := os.CreateTemp("", "logs-*.zip")
	if err != nil {
		rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
		return
	}
	defer os.Remove(tmpFile.Name())

	zipWriter := zip.NewWriter(tmpFile)
	zipWriterClosed := false
	defer func() {
		if !zipWriterClosed {
			_ = zipWriter.Close()
		}
	}()

	wasError := false

	for _, logR := range logs {
		func(log db.Log) { // Pass log as a parameter to ensure it's correctly captured
			f, err := os.Open(log.Path)
			if err != nil {
				if os.IsNotExist(err) {
					// this sucks, but we still want to return whatever logs we have.
					// however this should only happen if the log file was deleted while we were reading it,
					// or if it was deleted manually.
					srv.Logger.Warningf("Log file %s does not exist\n", log.Path)
					return
				}
				rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
				wasError = true
				return
			}
			defer f.Close()

			zipFCreated, err := zipWriter.Create(filepath.Base(log.Path))
			if err != nil {
				rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
				wasError = true
				return
			}
			_, err = copyAndMarkRepeatedLines(zipFCreated, f)
			if err != nil {
				rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
				wasError = true
				return
			}

		}(logR) // Call the anonymous function with log as argument

		if wasError {
			return
		}
	}

	if err != nil {
		rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
		return
	}

	zipWriter.Close()
	zipWriterClosed = true

	_, err = tmpFile.Seek(0, 0)
	if err != nil {
		rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Disposition", "attachment; filename=logs.zip")
	w.Header().Set("Content-Type", "application/zip")
	_, err = io.Copy(w, tmpFile)
	if err != nil {
		rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
		return
	}
}
