package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// writeError answers with the one error shape. A *protocol.Error, even when wrapped, is sent as
// it is. Any other error is the daemon's own problem: it is logged with its detail and the client
// gets the generic internal message, so nothing about the inside leaks out.
func (s *Server) writeError(w http.ResponseWriter, err error) {
	var perr *protocol.Error
	if !errors.As(err, &perr) {
		perr = protocol.Internal().WithCause(err)
	}
	status := statusOf(perr.Code)
	switch {
	case errors.Is(err, context.Canceled):
		// The client hung up, or the daemon is shutting down, while the request was being answered.
		// That is not a failure of the daemon, so it must not read as one in the log.
		s.log.Debug("request cancelled", "error", err)
	case status >= http.StatusInternalServerError:
		s.log.Error("request failed", "code", perr.Code, "error", err)
	}
	s.writeJSON(w, status, protocol.ErrorResponse{Error: *perr})
}

// statusOf is the fixed table from error code to HTTP status. An unknown code is the daemon's own
// problem, so it is a 500.
func statusOf(code protocol.ErrorCode) int {
	switch code {
	case protocol.ErrorCodeInvalidArgument:
		return http.StatusBadRequest
	case protocol.ErrorCodeUnauthorized:
		return http.StatusUnauthorized
	case protocol.ErrorCodeForbidden:
		return http.StatusForbidden
	case protocol.ErrorCodeNotFound:
		return http.StatusNotFound
	case protocol.ErrorCodeMethodNotAllowed:
		return http.StatusMethodNotAllowed
	case protocol.ErrorCodeConflict:
		return http.StatusConflict
	case protocol.ErrorCodeRefused:
		return http.StatusUnprocessableEntity
	case protocol.ErrorCodeUnsupported:
		return http.StatusNotImplemented
	case protocol.ErrorCodeUnavailable:
		return http.StatusServiceUnavailable
	case protocol.ErrorCodeInternal:
		return http.StatusInternalServerError
	}
	return http.StatusInternalServerError
}
