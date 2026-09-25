package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

const (
	// DefaultPageLimit is the page size when the request has no limit.
	DefaultPageLimit = 50
	// MaxPageLimit is the largest page size. A bigger limit is cut to this.
	MaxPageLimit = 200
	// maxCursorLength keeps a hostile client from sending a huge cursor to decode.
	maxCursorLength = 1024
)

// ParsePage reads the paging parameters of a list request: `limit` (default 50, at most 200; a
// bigger number is cut to 200) and `cursor` (empty for the first page). A limit that is not a
// whole number of 1 or more is an invalid_argument error. The cursor is not checked here: the
// handler decodes it with DecodeCursor, because only the handler knows its shape.
func ParsePage(r *http.Request) (limit int, cursor string, err error) {
	query := r.URL.Query()
	cursor = query.Get("cursor")
	if len(cursor) > maxCursorLength {
		return 0, "", errBadCursor()
	}
	text := query.Get("limit")
	if text == "" {
		return DefaultPageLimit, cursor, nil
	}
	limit, convErr := strconv.Atoi(text)
	if convErr != nil || limit < 1 {
		return 0, "", protocol.InvalidArgument(
			fmt.Sprintf("The page size must be a whole number from 1 to %d. Change the limit and try again.", MaxPageLimit))
	}
	return min(limit, MaxPageLimit), cursor, nil
}

// EncodeCursor makes a cursor from a small value that says where the next page starts, such as a
// sort key. A cursor is the value as JSON, encoded as URL-safe base64 without padding. Clients
// treat it as opaque, so the shape can change without a new API version.
func EncodeCursor[T any](position T) (string, error) {
	data, err := json.Marshal(position)
	if err != nil {
		return "", fmt.Errorf("encode a cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

// DecodeCursor reads a cursor made by EncodeCursor. An empty cursor is not an error: it returns
// the zero value and false, which means the first page. A cursor that cannot be read is an
// invalid_argument error.
func DecodeCursor[T any](cursor string) (position T, found bool, err error) {
	if cursor == "" {
		return position, false, nil
	}
	data, decodeErr := base64.RawURLEncoding.DecodeString(cursor)
	if decodeErr != nil || len(cursor) > maxCursorLength {
		return position, false, errBadCursor()
	}
	if jsonErr := json.Unmarshal(data, &position); jsonErr != nil {
		var zero T
		return zero, false, errBadCursor()
	}
	return position, true, nil
}

func errBadCursor() *protocol.Error {
	return protocol.InvalidArgument("That page marker is not valid. Go back to the first page and try again.")
}
