package api

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"procsman_backend/db"
	"procsman_backend/procsmanager"
	"runtime"
	"strconv"
)

type GetProcessesResponse struct {
	Processes []db.Process `json:"processes"`
}

func (srv *HttpServer) GetProcesses(w http.ResponseWriter, r *http.Request) {
	rw := r.Context().Value(ContextKeyWrappedRequest).(*ReqWrapper)

	processes, err := srv.ProcessManager.Queries.GetProcesses(r.Context())
	if err != nil {
		rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
		return
	}
	if processes == nil {
		processes = make([]db.Process, 0)
	}

	rw.MarshalAndRespond(GetProcessesResponse{
		Processes: processes,
	})
}

func (srv *HttpServer) GetProcess(w http.ResponseWriter, r *http.Request) {
	rw := r.Context().Value(ContextKeyWrappedRequest).(*ReqWrapper)

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
		rw.E(MessageCodeErrorGettingProcess, "Error getting process", http.StatusInternalServerError, "Error getting process")
		return
	}
	rw.MarshalAndRespond(process)
}

type AddProcessRequest struct {
	Name  string      `json:"name"`
	Group pgtype.Int4 `json:"group"`

	CreateNewGroup bool                      `json:"create_new_group"`
	NewGroup       CreateProcessGroupRequest `json:"new_group"`

	Color          pgtype.Text       `json:"color"`
	Enabled        bool              `json:"enabled"`
	ExecutablePath string            `json:"executable_path"`
	Arguments      string            `json:"arguments"`
	WorkingDir     string            `json:"working_dir"`
	Environment    map[string]string `json:"environment"`
	Config         db.Configuration  `json:"config"`

	group *db.ProcessGroup
}

//goland:noinspection GoBoolExpressions
func CheckPath(source string) (string, *Error) {
	info, err := os.Stat(source)
	if err == nil {
		if info.IsDir() {
			return "", MakeE(MessageCodeExecutableNotFile, "executable_path is not a file", http.StatusBadRequest, "executable_path is not a file")
		}
		if runtime.GOOS != "windows" && info.Mode()&0111 == 0 {
			return "", MakeE(MessageCodeExecutableNotExecutable, "executable_path is not executable", http.StatusBadRequest, "executable_path is not executable")
		}
		return source, nil
	}
	fullPath, err := exec.LookPath(source)
	if err != nil {
		return "", MakeE(MessageCodeExecutableNotFound, "executable_path does not exist", http.StatusBadRequest, "executable_path does not exist")
	}
	info, err = os.Stat(fullPath)
	if err != nil {
		return "", MakeE(MessageCodeExecutableNotFound, "executable_path does not exist", http.StatusBadRequest, "executable_path does not exist")
	}
	if info.IsDir() {
		return "", MakeE(MessageCodeExecutableNotFile, "executable_path is not a file", http.StatusBadRequest, "executable_path is not a file")
	}
	if runtime.GOOS != "windows" && info.Mode()&0111 == 0 {
		return "", MakeE(MessageCodeExecutableNotExecutable, "executable_path is not executable", http.StatusBadRequest, "executable_path is not executable")
	}
	return fullPath, nil
}

func (a *AddProcessRequest) Validate(ctx context.Context, srv *HttpServer) *Error {
	if a.Name == "" {
		return MakeE(MessageCodeNameRequired, "name is required", http.StatusBadRequest, "name is required")
	}

	if a.ExecutablePath == "" {
		return MakeE(MessageCodeExecutableRequired, "executable_path is required", http.StatusBadRequest, "executable_path is required")
	}

	realPath, err := CheckPath(a.ExecutablePath)
	if err != nil {
		return err
	}

	if a.WorkingDir != "" {
		info, err := os.Stat(a.WorkingDir)
		if err != nil {
			return MakeE(MessageCodeWdNotFound, "working_dir does not exist", http.StatusBadRequest, "working_dir does not exist")
		}
		if !info.IsDir() {
			return MakeE(MessageCodeWdNotDir, "working_dir is not a directory", http.StatusBadRequest, "working_dir is not a directory")
		}
	} else {
		a.WorkingDir = filepath.Dir(realPath)
	}

	if a.Group.Valid {
		group, getErr := srv.ProcessManager.Queries.GetProcessGroup(ctx, a.Group.Int32)
		if getErr != nil {
			return MakeE(MessageCodeInvalidGroup, "invalid group", http.StatusBadRequest, "invalid group")
		}
		a.group = &group
	} else if a.CreateNewGroup {
		if err = a.NewGroup.Validate(ctx, srv); err != nil {
			return err
		}
	}
	if a.Environment == nil {
		a.Environment = make(map[string]string)
	}

	//if a.Color == nil {
	//	a.Color = &db.Color{}
	//}

	return nil
}

func (srv *HttpServer) AddProcess(w http.ResponseWriter, r *http.Request) {
	rw := r.Context().Value(ContextKeyWrappedRequest).(*ReqWrapper)
	req := r.Context().Value(ContextKeyUnmarshalledJson).(*AddProcessRequest)

	tx, queries, err := srv.ProcessManager.OpenTx(r.Context())
	if err != nil {
		rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
		return
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback(r.Context())
			rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
			return
		}
	}()

	var groupID pgtype.Int4
	if req.group != nil {
		groupID.Int32 = req.group.ID
		groupID.Valid = true
	} else if req.CreateNewGroup {
		var group db.ProcessGroup
		group, err = queries.CreateProcessGroup(r.Context(), db.CreateProcessGroupParams{
			Name:  req.NewGroup.Name,
			Color: req.NewGroup.Color,
		})
		if err != nil {
			rw.E(MessageCodeCouldNotCreateGroup, "Could not create process group", http.StatusInternalServerError, err.Error())
			return
		}
		groupID.Int32 = group.ID
		groupID.Valid = true
	}

	var created db.Process
	created, err = queries.CreateProcess(r.Context(), db.CreateProcessParams{
		Name:             req.Name,
		ProcessGroupID:   groupID,
		Color:            req.Color,
		ExecutablePath:   req.ExecutablePath,
		Arguments:        req.Arguments,
		WorkingDirectory: req.WorkingDir,
		Environment:      req.Environment,
		Configuration:    req.Config,
		Enabled:          req.Enabled,
	})
	if err != nil {
		rw.E(MessageCodeCouldNotCreateProcess, "Could not create process", http.StatusInternalServerError, err.Error())
		return
	}

	go srv.ProcessManager.AddRunner(&created).Work()
	rw.MarshalAndRespond(created)
}

func (srv *HttpServer) DeleteProcess(w http.ResponseWriter, r *http.Request) {
	rw := r.Context().Value(ContextKeyWrappedRequest).(*ReqWrapper)

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

	_, err = srv.ProcessManager.Queries.GetProcess(r.Context(), int32(idInt))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			rw.E(MessageCodeProcessNotFound, "Process not found", http.StatusNotFound, "Process not found")
			return
		}
		rw.E(MessageCodeErrorGettingProcess, "Error getting process", http.StatusInternalServerError, err.Error())
		return
	}

	err = srv.ProcessManager.Queries.DeleteProcess(r.Context(), int32(idInt))
	if err != nil {
		rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, fmt.Sprintf("Error deleting process: %s", err.Error()))
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

type ProcessStatusChange struct {
	Enabled bool `json:"enabled"`
}

type WhatToDo int

const (
	Start WhatToDo = iota
	Stop
	Restart
)

func (srv *HttpServer) startStopProcess(w http.ResponseWriter, r *http.Request, what WhatToDo) {
	rw := r.Context().Value(ContextKeyWrappedRequest).(*ReqWrapper)

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

	_, err = srv.ProcessManager.Queries.GetProcess(r.Context(), int32(idInt))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			rw.E(MessageCodeProcessNotFound, "Process not found", http.StatusNotFound, "Process not found")
			return
		}
		rw.E(MessageCodeErrorGettingProcess, "Error getting process", http.StatusInternalServerError, err.Error())
		return
	}

	runner := srv.ProcessManager.GetRunner(int32(idInt))

	switch what {
	case Start:
		runner.SignalIn <- procsmanager.Start
	case Stop:
		runner.SignalIn <- procsmanager.Stop
	case Restart:
		runner.SignalIn <- procsmanager.Restart
	}
	rw.MarshalAndRespond(ProcessStatusChange{Enabled: what == Start || what == Restart})
}

func (srv *HttpServer) StartProcess(w http.ResponseWriter, r *http.Request) {
	srv.startStopProcess(w, r, Start)
}

func (srv *HttpServer) StopProcess(w http.ResponseWriter, r *http.Request) {
	srv.startStopProcess(w, r, Stop)
}

func (srv *HttpServer) RestartProcess(writer http.ResponseWriter, request *http.Request) {
	srv.startStopProcess(writer, request, Restart)
}

type UpdateProcessRequest struct {
	Name  string      `json:"name"`
	Group pgtype.Int4 `json:"group_id"`

	CreateNewGroup bool                      `json:"create_new_group"`
	NewGroup       CreateProcessGroupRequest `json:"new_group"`

	Color          pgtype.Text       `json:"color"`
	Enabled        bool              `json:"enabled"`
	ExecutablePath string            `json:"executable_path"`
	Arguments      string            `json:"arguments"`
	WorkingDir     string            `json:"working_dir"`
	Environment    map[string]string `json:"environment"`
	Config         db.Configuration  `json:"config"`

	group *db.ProcessGroup
}

func (u *UpdateProcessRequest) Validate(ctx context.Context, srv *HttpServer) *Error {
	if u.Name == "" {
		return MakeE(MessageCodeNameRequired, "name is required", http.StatusBadRequest, "name is required")
	}

	if u.ExecutablePath == "" {
		return MakeE(MessageCodeExecutableRequired, "executable_path is required", http.StatusBadRequest, "executable_path is required")
	}
	realPath, err := CheckPath(u.ExecutablePath)
	if err != nil {
		return err
	}

	if u.WorkingDir != "" {
		info, err := os.Stat(u.WorkingDir)
		if err != nil {
			return MakeE(MessageCodeWdNotFound, "working_dir does not exist", http.StatusBadRequest, "working_dir does not exist")
		}
		if !info.IsDir() {
			return MakeE(MessageCodeWdNotDir, "working_dir is not a directory", http.StatusBadRequest, "working_dir is not a directory")
		}
	} else if u.CreateNewGroup {
		if err = u.NewGroup.Validate(ctx, srv); err != nil {
			return err
		}
	} else {
		u.WorkingDir = filepath.Dir(realPath)
	}

	if u.Group.Valid {
		group, err := srv.ProcessManager.Queries.GetProcessGroup(ctx, u.Group.Int32)
		if err != nil {
			return MakeE(MessageCodeInvalidGroup, "invalid group", http.StatusBadRequest, "invalid group")
		}
		u.group = &group
	} else if u.CreateNewGroup {
		if validateGroupErr := u.NewGroup.Validate(ctx, srv); validateGroupErr != nil {
			return validateGroupErr
		}

	}

	if u.Environment == nil {
		u.Environment = make(map[string]string)
	}

	//if u.Color == nil {
	//	u.Color = &db.Color{}
	//}

	return nil
}

func (srv *HttpServer) UpdateProcess(w http.ResponseWriter, r *http.Request) {
	rw := r.Context().Value(ContextKeyWrappedRequest).(*ReqWrapper)
	req := r.Context().Value(ContextKeyUnmarshalledJson).(*UpdateProcessRequest)

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
	var tx pgx.Tx
	var queries *db.Queries
	tx, queries, err = srv.ProcessManager.OpenTx(r.Context())
	if err != nil {
		rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
		return
	}

	_, err = queries.GetProcess(r.Context(), int32(idInt))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			rw.E(MessageCodeProcessNotFound, "Process not found", http.StatusNotFound, "Process not found")
			return
		}
		rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback(r.Context())
			rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
			return
		}
	}()

	if req.group != nil {
		_, err = queries.GetProcessGroup(r.Context(), req.Group.Int32)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				rw.E(MessageCodeGroupNotFound, "Group not found", http.StatusNotFound, "Group not found")
				return
			}
			rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
			return
		}
	} else if req.CreateNewGroup {
		var group db.ProcessGroup
		group, err = queries.CreateProcessGroup(r.Context(), db.CreateProcessGroupParams{
			Name:  req.NewGroup.Name,
			Color: req.NewGroup.Color,
		})
		if err != nil {
			rw.E(MessageCodeCouldNotCreateGroup, "Could not create process group", http.StatusInternalServerError, err.Error())
			return
		}
		req.Group.Int32 = group.ID
		req.Group.Valid = true
	}

	var existingProcess db.Process
	existingProcess, err = queries.GetProcess(r.Context(), int32(idInt))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			rw.E(MessageCodeProcessNotFound, "Process not found", http.StatusNotFound, "Process not found")
			return
		}
		rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
		return

	}

	needsRestart := false
	if existingProcess.Enabled != req.Enabled || req.ExecutablePath != existingProcess.ExecutablePath || req.Arguments != existingProcess.Arguments || req.WorkingDir != existingProcess.WorkingDirectory || !maps.Equal(req.Environment, existingProcess.Environment) || req.Config != existingProcess.Configuration {
		needsRestart = true
	}
	var process db.Process
	process, err = queries.UpdateProcess(r.Context(), db.UpdateProcessParams{
		ID:               int32(idInt),
		Name:             req.Name,
		ProcessGroupID:   req.Group,
		Color:            req.Color,
		ExecutablePath:   req.ExecutablePath,
		Arguments:        req.Arguments,
		WorkingDirectory: req.WorkingDir,
		Environment:      req.Environment,
		Configuration:    req.Config,
		Enabled:          req.Enabled,
	})
	if err != nil {
		rw.E(MessageCodeCouldNotCreateProcess, "Could not edit process", http.StatusInternalServerError, err.Error())
		return
	}
	runner := srv.ProcessManager.GetRunner(int32(idInt))
	if needsRestart {
		runner.SignalIn <- procsmanager.Refresh
	}

	rw.MarshalAndRespond(process)
}

func (srv *HttpServer) GetDefaultConfiguration(w http.ResponseWriter, r *http.Request) {
	rw := r.Context().Value(ContextKeyWrappedRequest).(*ReqWrapper)

	rw.MarshalAndRespond(db.DefaultConfiguration)
}
