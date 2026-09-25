package protocol

import (
	"fmt"
	"strings"
)

// ErrorCode is a stable, machine-readable reason a request was not done. Codes never change
// once released, so a client can branch on them. The HTTP status for each code is in
// docs/architecture.md section 11.5.
type ErrorCode string

const (
	// ErrorCodeInvalidArgument means the request was malformed or a value was not allowed.
	ErrorCodeInvalidArgument ErrorCode = "invalid_argument"
	// ErrorCodeUnauthorized means the request carried no valid token.
	ErrorCodeUnauthorized ErrorCode = "unauthorized"
	// ErrorCodeForbidden means the token is valid but is not allowed to do this.
	ErrorCodeForbidden ErrorCode = "forbidden"
	// ErrorCodeNotFound means the thing asked for does not exist.
	ErrorCodeNotFound ErrorCode = "not_found"
	// ErrorCodeMethodNotAllowed means the address exists but not for this kind of request. The
	// answer carries an Allow header and an "allow" detail that list the methods that work.
	ErrorCodeMethodNotAllowed ErrorCode = "method_not_allowed"
	// ErrorCodeConflict means the request clashes with the current state, for example a name in use.
	ErrorCodeConflict ErrorCode = "conflict"
	// ErrorCodeRefused means the request is valid but the rules do not allow it, such as an illegal
	// card move.
	ErrorCodeRefused ErrorCode = "refused"
	// ErrorCodeUnsupported means the daemon does not do this on this machine or yet.
	ErrorCodeUnsupported ErrorCode = "unsupported"
	// ErrorCodeUnavailable means the daemon cannot do this right now and the client can try again.
	ErrorCodeUnavailable ErrorCode = "unavailable"
	// ErrorCodeInternal means something broke inside the daemon. The details stay in its log.
	ErrorCodeInternal ErrorCode = "internal"
)

// ErrorCodeValues lists every error code.
func ErrorCodeValues() []ErrorCode {
	return []ErrorCode{
		ErrorCodeInvalidArgument, ErrorCodeUnauthorized, ErrorCodeForbidden, ErrorCodeNotFound,
		ErrorCodeMethodNotAllowed, ErrorCodeConflict, ErrorCodeRefused, ErrorCodeUnsupported,
		ErrorCodeUnavailable, ErrorCodeInternal,
	}
}

// Error is the one error shape of the API. It is sent as {"error": {...}} (see ErrorResponse).
// The message is a plain sentence for the person using the app: it says what happened and, when
// there is something to do, what to do next. It never holds a stack, a file path, or a secret.
type Error struct {
	// Code is the stable reason.
	Code ErrorCode `json:"code"`
	// Message is the sentence to show.
	Message string `json:"message"`
	// Details holds extra facts for the "Details" section, such as the id that was not found.
	Details map[string]string `json:"details,omitempty"`
	// cause is the underlying error. It is for the daemon's log and is never sent.
	cause error
}

// ErrorResponse is the body of every error answer.
type ErrorResponse struct {
	// Error is the error itself.
	Error Error `json:"error"`
}

// NewError makes an error with a code and a message.
func NewError(code ErrorCode, message string) *Error {
	return &Error{Code: code, Message: message}
}

// Error returns the code, the message, and the cause if there is one, for logs. The client is
// sent Code, Message, and Details only.
func (e *Error) Error() string {
	text := string(e.Code) + ": " + e.Message
	if e.cause != nil {
		text += ": " + e.cause.Error()
	}
	return text
}

// Unwrap returns the underlying error set by WithCause.
func (e *Error) Unwrap() error {
	return e.cause
}

// With adds one detail and returns the error, so calls can be chained.
func (e *Error) With(key, value string) *Error {
	if e.Details == nil {
		e.Details = map[string]string{}
	}
	e.Details[key] = value
	return e
}

// WithCause records the underlying error for the log. The client never sees it.
func (e *Error) WithCause(err error) *Error {
	e.cause = err
	return e
}

// InvalidArgument says a value in the request is wrong. The message says which one and how to fix it.
func InvalidArgument(message string) *Error {
	return NewError(ErrorCodeInvalidArgument, message)
}

// Unauthorized says the request had no valid token.
func Unauthorized() *Error {
	return NewError(ErrorCodeUnauthorized,
		"Marshal does not recognize this device. Pair it again from the desktop app.")
}

// Forbidden says the token may not do this. The message says what is not allowed.
func Forbidden(message string) *Error {
	return NewError(ErrorCodeForbidden, message)
}

// NotFound says a thing does not exist. what names it in lower case, such as "card" or "project".
func NotFound(what string) *Error {
	return NewError(ErrorCodeNotFound, fmt.Sprintf("Marshal cannot find that %s. It may have been removed.", what))
}

// MethodNotAllowed says the address does not accept this kind of request. allowed lists the
// methods that do, such as "GET" and "POST".
func MethodNotAllowed(allowed ...string) *Error {
	list := strings.Join(allowed, ", ")
	return NewError(ErrorCodeMethodNotAllowed,
		"That address does not accept this kind of request. Use one of these instead: "+list+".").With("allow", list)
}

// Conflict says the request clashes with the current state. The message says how to resolve it.
func Conflict(message string) *Error {
	return NewError(ErrorCodeConflict, message)
}

// Refused says the rules do not allow a valid request. The message says why and what is allowed.
func Refused(message string) *Error {
	return NewError(ErrorCodeRefused, message)
}

// Unsupported says the daemon does not do this. The message says what and, if known, when.
func Unsupported(message string) *Error {
	return NewError(ErrorCodeUnsupported, message)
}

// Unavailable says to try again later. The message says what is not ready.
func Unavailable(message string) *Error {
	return NewError(ErrorCodeUnavailable, message)
}

// Internal is the answer for a failure that is the daemon's own. It is deliberately generic:
// the real reason goes to the log through WithCause.
func Internal() *Error {
	return NewError(ErrorCodeInternal,
		"Marshal hit an unexpected problem and could not finish that. Try again. If it keeps happening, check Marshal's log.")
}
