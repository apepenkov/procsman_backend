package api

import (
	"context"
	"github.com/apepenkov/yalog"
	"net/http"
	"procsman_backend/procsmanager"
	"time"
)

type InterfaceGetter func() ModelWithValidation

type HttpServer struct {
	Mux            *http.ServeMux
	Server         *http.Server
	ProcessManager *procsmanager.ProcessManager
	Logger         *yalog.Logger
}

type ModelWithValidation interface {
	Validate(ctx context.Context, srv *HttpServer) *Error
}

func NewHttpServer(processManager *procsmanager.ProcessManager, serveAddr string) *HttpServer {
	srv := &HttpServer{
		Mux: http.NewServeMux(),
		Server: &http.Server{
			Addr:         serveAddr,
			ReadTimeout:  time.Second * 10,
			WriteTimeout: time.Second * 10,
		},
		ProcessManager: processManager,
		Logger:         processManager.Logger.NewLogger("http"),
	}

	hf := func(a func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
		return a
	}

	WrapAuth := func(a func(http.ResponseWriter, *http.Request)) http.Handler {
		return srv.WrapAccessControl(srv.WrapRequestMiddleware(srv.AuthMiddleware(hf(a))))
	}

	WrapAuthAndJson := func(a func(http.ResponseWriter, *http.Request), toGetter InterfaceGetter) http.Handler {
		return srv.WrapAccessControl(srv.WrapRequestMiddleware(srv.AuthMiddleware(srv.MustUnmarshalJsonMiddleware(hf(a), toGetter))))
	}

	GetAddProcessRequest := func() ModelWithValidation {
		return &AddProcessRequest{}
	}
	GetAddGroupRequest := func() ModelWithValidation {
		return &CreateProcessGroupRequest{}
	}
	GetUpdateProcessRequest := func() ModelWithValidation {
		return &UpdateProcessRequest{}
	}
	GetUpdateGroupRequest := func() ModelWithValidation {
		return &UpdateProcessGroupRequest{}
	}
	GetStdInRequest := func() ModelWithValidation {
		return &StdInRequest{}
	}

	srv.Mux.Handle("GET /processes", WrapAuth(srv.GetProcesses))
	srv.Mux.Handle("POST /processes", WrapAuthAndJson(srv.AddProcess, GetAddProcessRequest))
	srv.Mux.Handle("GET /processes/by_id/{id}", WrapAuth(srv.GetProcess))
	srv.Mux.Handle("DELETE /processes/by_id/{id}", WrapAuth(srv.DeleteProcess))
	srv.Mux.Handle("PATCH /processes/by_id/{id}", WrapAuthAndJson(srv.UpdateProcess, GetUpdateProcessRequest))

	srv.Mux.Handle("POST /processes/by_id/{id}/stop", WrapAuth(srv.StopProcess))
	srv.Mux.Handle("POST /processes/by_id/{id}/start", WrapAuth(srv.StartProcess))
	srv.Mux.Handle("POST /processes/by_id/{id}/restart", WrapAuth(srv.RestartProcess))

	srv.Mux.Handle("GET /processes/by_id/{id}/stats", WrapAuth(srv.GetProcessStats))
	srv.Mux.Handle("GET /processes/by_id/{id}/events", WrapAuth(srv.GetProcessEvents))
	srv.Mux.Handle("GET /processes/by_id/{id}/logs", WrapAuth(srv.GetProcessLogs))
	srv.Mux.Handle("GET /processes/by_id/{id}/export_logs", WrapAuth(srv.ExportLogsAsZip))
	srv.Mux.Handle("PUT /processes/by_id/{id}/stdin", WrapAuthAndJson(srv.PostStdin, GetStdInRequest))

	srv.Mux.Handle("GET /groups", WrapAuth(srv.GetGroups))
	srv.Mux.Handle("POST /groups", WrapAuthAndJson(srv.CreateGroup, GetAddGroupRequest))
	srv.Mux.Handle("GET /groups/by_id/{id}", WrapAuth(srv.GetGroup))
	srv.Mux.Handle("DELETE /groups/by_id/{id}", WrapAuth(srv.DeleteGroup))
	srv.Mux.Handle("PATCH /groups/by_id/{id}", WrapAuthAndJson(srv.UpdateGroup, GetUpdateGroupRequest))

	srv.Mux.Handle("GET /notification_config", WrapAuth(srv.GetNotificationSettings))
	srv.Mux.Handle("PATCH /notification_config", WrapAuthAndJson(srv.UpdateNotificationSettings, func() ModelWithValidation {
		return &PatchNotificationsConfig{}
	}))

	srv.Mux.Handle("GET /health", srv.WrapAccessControl(srv.WrapRequestMiddleware(http.HandlerFunc(srv.HealthCheck))))
	srv.Mux.Handle("GET /check_auth", WrapAuth(srv.HealthCheck))

	srv.Mux.Handle("GET /default_config", WrapAuth(srv.GetDefaultConfiguration))

	srv.Mux.Handle("OPTIONS /", srv.WrapAccessControl(srv.WrapRequestMiddleware(http.HandlerFunc(srv.OPTIONS))))

	return srv
}

func (srv *HttpServer) OPTIONS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS, PATCH, PUT")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Auth-Key")
	w.WriteHeader(http.StatusOK)
}
func (srv *HttpServer) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (srv *HttpServer) Close() {
	_ = srv.Server.Shutdown(context.Background())
}

func (srv *HttpServer) ListenAndServe() error {
	srv.Server.Handler = srv.Mux
	return srv.Server.ListenAndServe()
}
