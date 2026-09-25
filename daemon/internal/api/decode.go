package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// bitsPerKiB turns a byte count into KiB for the "too large" message (limit>>bitsPerKiB).
const bitsPerKiB = 10

// decodeJSON reads the request body into dst and holds it to the rules: the body must be one JSON
// value, a field that dst does not have is refused by name, nothing may follow the value, and the
// body may not be larger than the size limit. The error is always a *protocol.Error with the code
// invalid_argument and a plain sentence, so a handler can pass it straight to writeError.
func (s *Server) decodeJSON(r *http.Request, dst any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, s.limits.MaxBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return s.decodeError(err)
	}
	// A second value, even a valid one, means the body is not what the route expects.
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return protocol.InvalidArgument("The request body has more than one JSON value. Send a single JSON object.")
		}
		return s.decodeError(err)
	}
	return nil
}

// decodeError turns what the JSON decoder reported into a sentence for the person.
func (s *Server) decodeError(err error) *protocol.Error {
	var (
		syntax   *json.SyntaxError
		mismatch *json.UnmarshalTypeError
		tooLarge *http.MaxBytesError
	)
	switch {
	case errors.Is(err, io.EOF):
		return protocol.InvalidArgument("The request body is empty. Send a JSON object.")
	case errors.As(err, &tooLarge):
		return errBodyTooLarge(tooLarge.Limit)
	case errors.As(err, &syntax), errors.Is(err, io.ErrUnexpectedEOF):
		return protocol.InvalidArgument("The request body is not valid JSON. Check it and try again.")
	case errors.As(err, &mismatch):
		return typeMismatch(mismatch)
	}
	if field, ok := unknownField(err); ok {
		return protocol.InvalidArgument(fmt.Sprintf("The field %q is not one Marshal knows. Remove it and try again.", field))
	}
	return protocol.InvalidArgument("The request body could not be read. Check it and try again.")
}

func typeMismatch(err *json.UnmarshalTypeError) *protocol.Error {
	if err.Field == "" {
		return protocol.InvalidArgument("The request body must be " + plainKind(err.Type) + ".")
	}
	return protocol.InvalidArgument(fmt.Sprintf("The field %q must be %s.", err.Field, plainKind(err.Type)))
}

// unknownField pulls the field name out of the decoder's "unknown field" error, which has no type
// of its own.
func unknownField(err error) (string, bool) {
	rest, found := strings.CutPrefix(err.Error(), `json: unknown field "`)
	if !found {
		return "", false
	}
	return strings.TrimSuffix(rest, `"`), true
}

// plainKind names a Go type in words a person reads, for the "must be" sentence.
func plainKind(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "text"
	case reflect.Bool:
		return "true or false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "a whole number"
	case reflect.Float32, reflect.Float64:
		return "a number"
	case reflect.Slice, reflect.Array:
		return "a list"
	case reflect.Map, reflect.Struct:
		return "an object"
	default:
		return "a different kind of value"
	}
}

func errBodyTooLarge(limit int64) *protocol.Error {
	return protocol.InvalidArgument(fmt.Sprintf("The request is larger than Marshal accepts. Send less than %d KiB.", limit>>bitsPerKiB))
}
