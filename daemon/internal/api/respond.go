package api

import (
	"encoding/json"
	"net/http"
)

const (
	jsonContentType = "application/json; charset=utf-8"
	// fallbackErrorBody is sent when a response cannot be encoded. It is the encoded form of
	// protocol.Internal(), and a test keeps the two equal.
	fallbackErrorBody = `{"error":{"code":"internal","message":"Marshal hit an unexpected problem and could not finish that. Try again. If it keeps happening, check Marshal's log."}}` + "\n"
)

// writeJSON answers with a JSON body. The body is encoded first, so a value that cannot be
// encoded becomes a clean error answer and never a half-written success.
func (s *Server) writeJSON(w http.ResponseWriter, status int, body any) {
	data, err := json.Marshal(body)
	if err != nil {
		s.log.Error("encode a response", "error", err)
		status = http.StatusInternalServerError
		data = []byte(fallbackErrorBody)
	} else {
		data = append(data, '\n')
	}
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)
	if _, err := w.Write(data); err != nil {
		// The client went away. There is nobody left to answer.
		s.log.Debug("write a response", "error", err)
	}
}
