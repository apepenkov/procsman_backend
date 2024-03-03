package api

import (
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type ReqWrapper struct {
	r   *http.Request
	w   http.ResponseWriter
	Srv *HttpServer
	// TODO: implement responded everywhere
	responded bool
	Id        string
}

func (rw *ReqWrapper) Debugf(format string, args ...interface{}) {
	rw.Srv.Logger.Debugf(fmt.Sprintf("[%s] %s", rw.Id, format), args...)
}

func (rw *ReqWrapper) Infof(format string, args ...interface{}) {
	rw.Srv.Logger.Infof(fmt.Sprintf("[%s] %s", rw.Id, format), args...)
}

func (rw *ReqWrapper) Errorf(format string, args ...interface{}) {
	rw.Srv.Logger.Errorf(fmt.Sprintf("[%s] %s", rw.Id, format), args...)
}

func (rw *ReqWrapper) Debugln(args ...interface{}) {
	idStr := fmt.Sprintf("[%s]", rw.Id)
	args = append([]interface{}{idStr}, args...)
	rw.Srv.Logger.Debugln(args...)
}

func (rw *ReqWrapper) Infoln(args ...interface{}) {
	idStr := fmt.Sprintf("[%s]", rw.Id)
	args = append([]interface{}{idStr}, args...)
	rw.Srv.Logger.Infoln(args...)
}

func (rw *ReqWrapper) Errorln(args ...interface{}) {
	idStr := fmt.Sprintf("[%s]", rw.Id)
	args = append([]interface{}{idStr}, args...)
	rw.Srv.Logger.Errorln(args...)
}

func randomString(length int) string {
	bytes := make([]byte, (length+1)/2)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)[:length]
}

func (rw *ReqWrapper) MarshalAndRespond(resp interface{}) {
	rw.MarshalAndRespondWithStatus(resp, http.StatusOK)
}

const MinForGzip = 1024

func (rw *ReqWrapper) MarshalAndRespondWithStatus(resp interface{}, status int) {
	jsonResp, err := json.Marshal(resp)
	if err != nil {
		http.Error(rw.w, "Error marshalling response", http.StatusInternalServerError)
		return
	}

	canGzip := len(jsonResp) > MinForGzip && strings.Contains(rw.r.Header.Get("Accept-Encoding"), "gzip")
	if !canGzip {
		rw.w.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(status)
		_, err = rw.w.Write(jsonResp)
		return
	}

	gz := gzip.NewWriter(rw.w)

	defer func() {
		_ = gz.Close()
	}()

	// why does gz.Close() overwrite the headers?
	rw.w.Header().Set("Content-Encoding", "gzip")
	rw.w.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	_, _ = gz.Write(jsonResp)
}
