package api

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"net/http"
	"procsman_backend/db"
	"strconv"
)

type CreateProcessGroupRequest struct {
	Name  string      `json:"name"`
	Color pgtype.Text `json:"color"`

	Config db.Configuration `json:"config"`
}

func (r *CreateProcessGroupRequest) Validate(ctx context.Context, srv *HttpServer) *Error {
	if r.Name == "" {
		return MakeE(MessageCodeNameRequired, "name is required", http.StatusBadRequest, "name is required")
	}

	exists, err := srv.ProcessManager.Queries.GroupExistsByName(ctx, r.Name)
	if err != nil {
		return MakeE(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
	}
	if exists {
		return MakeE(MessageCodeGroupAlreadyExists, "group already exists", http.StatusBadRequest, "group already exists")
	}

	return nil
}
func (srv *HttpServer) CreateGroup(w http.ResponseWriter, r *http.Request) {
	rw := r.Context().Value(ContextKeyWrappedRequest).(*ReqWrapper)
	req := r.Context().Value(ContextKeyUnmarshalledJson).(*CreateProcessGroupRequest)

	group, err := srv.ProcessManager.Queries.CreateProcessGroup(r.Context(), db.CreateProcessGroupParams{
		Name:                 req.Name,
		Color:                req.Color,
		ScriptsConfiguration: req.Config,
	})
	if err != nil {
		rw.E(MessageCodeCouldNotCreateGroup, "Could not create process group", http.StatusInternalServerError, err.Error())
		return
	}

	rw.MarshalAndRespond(group)
}

func (srv *HttpServer) GetGroups(w http.ResponseWriter, r *http.Request) {
	rw := r.Context().Value(ContextKeyWrappedRequest).(*ReqWrapper)

	groups, err := srv.ProcessManager.Queries.GetProcessGroups(r.Context())
	if err != nil {
		rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
		return
	}

	rw.MarshalAndRespond(groups)
}

func (srv *HttpServer) GetGroup(w http.ResponseWriter, r *http.Request) {
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

	group, err := srv.ProcessManager.Queries.GetProcessGroup(r.Context(), int32(idInt))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			rw.E(MessageCodeGroupNotFound, "Group not found", http.StatusNotFound, "Group not found")
			return
		}
		rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
		return
	}
	rw.MarshalAndRespond(group)
}

func (srv *HttpServer) DeleteGroup(w http.ResponseWriter, r *http.Request) {
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

	_, err = srv.ProcessManager.Queries.GetProcessGroup(r.Context(), int32(idInt))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			rw.E(MessageCodeGroupNotFound, "Group not found", http.StatusNotFound, "Group not found")
			return
		}
		rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
		return
	}

	err = srv.ProcessManager.Queries.DeleteProcessGroup(r.Context(), int32(idInt))
	if err != nil {
		rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

type UpdateProcessGroupRequest struct {
	Name   string           `json:"name"`
	Color  pgtype.Text      `json:"color"`
	Config db.Configuration `json:"config"`
}

func (u *UpdateProcessGroupRequest) Validate(ctx context.Context, srv *HttpServer) *Error {
	if u.Name == "" {
		return MakeE(MessageCodeNameRequired, "Name required", http.StatusBadRequest, "Name required")
	}

	return nil
}

func (srv *HttpServer) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	rw := r.Context().Value(ContextKeyWrappedRequest).(*ReqWrapper)
	req := r.Context().Value(ContextKeyUnmarshalledJson).(*UpdateProcessGroupRequest)

	idStr := r.PathValue("id")
	if idStr == "" {
		rw.E(MessageCodeNoIdProvided, "No id provided", http.StatusBadRequest, "No id provided")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		rw.E(MessageCodeInvalidId, "Invalid id", http.StatusBadRequest, "Invalid id")
		return
	}

	groupsWithSameName, err := srv.ProcessManager.Queries.GetProcessGroupByName(r.Context(), req.Name)
	if err != nil {
		rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
		return
	}

	for _, group := range groupsWithSameName {
		if group.ID != int32(id) {
			rw.E(MessageCodeGroupAlreadyExists, "Group already exists", http.StatusBadRequest, "Group already exists with the same name")
			return
		}
	}

	_, err = srv.ProcessManager.Queries.GetProcessGroup(r.Context(), int32(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			rw.E(MessageCodeGroupNotFound, "Group not found", http.StatusNotFound, "Group not found")
			return
		}
		rw.E(MessageCodeInternalError, "Internal error", http.StatusInternalServerError, err.Error())
		return
	}

	group, err := srv.ProcessManager.Queries.UpdateProcessGroup(r.Context(), db.UpdateProcessGroupParams{
		ID:                   int32(id),
		Name:                 req.Name,
		Color:                req.Color,
		ScriptsConfiguration: req.Config,
	})
	if err != nil {
		rw.E(MessageCodeCouldNotCreateGroup, "Could not create process group", http.StatusInternalServerError, err.Error())
		return
	}

	rw.MarshalAndRespond(group)
}
