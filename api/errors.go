package api

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
)

type MessageCode string

const (
	MessageCodeProcessNotFound         MessageCode = "process_not_found"
	MessageCodeErrorGettingProcess     MessageCode = "error_getting_process"
	MessageCodeNoIdProvided            MessageCode = "no_id_provided"
	MessageCodeInvalidId               MessageCode = "invalid_id"
	MessageCodeInvalidRequest          MessageCode = "invalid_request"
	MessageCodeNameRequired            MessageCode = "name_required"
	MessageCodeExecutableRequired      MessageCode = "executable_required"
	MessageCodeExecutableNotFound      MessageCode = "executable_not_found"
	MessageCodeExecutableNotFile       MessageCode = "executable_not_file"
	MessageCodeWdNotFound              MessageCode = "wd_not_found"
	MessageCodeWdNotDir                MessageCode = "wd_not_dir"
	MessageCodeInvalidGroup            MessageCode = "invalid_group"
	MessageCodeGroupAlreadyExists      MessageCode = "group_already_exists"
	MessageCodeInvalidColor            MessageCode = "invalid_color"
	MessageCodeCouldNotCreateProcess   MessageCode = "could_not_create_process"
	MessageCodeProcessNotRunning       MessageCode = "process_not_running"
	MessageCodeInternalError           MessageCode = "internal_error"
	MessageCodeCouldNotCreateGroup     MessageCode = "could_not_create_group"
	MessageCodeGroupNotFound           MessageCode = "group_not_found"
	MessageCodeExecutableNotExecutable MessageCode = "executable_not_executable"
	MessageCodeInvalidTimeFrame        MessageCode = "invalid_time_frame"
	MessageCodeInvalidLimit            MessageCode = "invalid_limit"
	MessageCodeTextRequired            MessageCode = "text_required"
)

type Error struct {
	MessageCode    MessageCode `json:"message_code"`
	MessageDefault string      `json:"message_default"`
	Code           int
	Details        string `json:"details"`
}

func (rw *ReqWrapper) WriteHeader(code int) error {
	if rw.responded {
		return errors.New("attempted to write header after responding")
	}
	rw.w.WriteHeader(code)
	rw.responded = true
	return nil
}

func (rw *ReqWrapper) WriteError(err *Error) error {
	if rw.responded {
		return errors.New("attempted to write error after responding")
	}
	caller, file, line, _ := runtime.Caller(1)
	// if caller is rw.E, need to skip another frame
	callerName := runtime.FuncForPC(caller).Name()
	if strings.HasSuffix(callerName, ".E") {
		_, file, line, _ = runtime.Caller(2)
	}

	file = filepath.Base(file)
	rw.Infof("Error: \n%s:%d: Writing error: %s (%s)\n", file, line, err.MessageDefault, err.Details)
	rw.MarshalAndRespondWithStatus(err, err.Code)
	return nil
}

func MakeE(messageCode MessageCode, messageDefault string, code int, details string) *Error {
	return &Error{
		MessageCode:    messageCode,
		MessageDefault: messageDefault,
		Code:           code,
		Details:        details,
	}
}

func (rw *ReqWrapper) E(messageCode MessageCode, messageDefault string, code int, details string) {
	rw.WriteError(MakeE(messageCode, messageDefault, code, details))
}

func (e *Error) Error() string {
	return e.MessageDefault
}
