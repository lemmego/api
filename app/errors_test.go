package app

import (
	"errors"
	"net/http"
	"testing"
)

func TestHttpMessageError(t *testing.T) {
	msg := HttpMessage{Status: http.StatusNotFound, Message: "not found"}
	if msg.Error() == "" {
		t.Error("HttpMessage.Error() should not be empty")
	}
}

func TestHttpErrorInterface(t *testing.T) {
	errs := []HttpError{
		&NotFoundError{HttpMessage{Status: http.StatusNotFound}},
		&BadRequestError{HttpMessage{Status: http.StatusBadRequest}},
		&UnauthorizedError{HttpMessage{Status: http.StatusUnauthorized}},
		&ForbiddenError{HttpMessage{Status: http.StatusForbidden}},
		&InternalServerError{HttpMessage{Status: http.StatusInternalServerError}},
		&MethodNotAllowedError{HttpMessage{Status: http.StatusMethodNotAllowed}},
		&PageExpiredError{HttpMessage{Status: 419}},
		&UnprocessableEntityError{HttpMessage{Status: http.StatusUnprocessableEntity}},
	}
	for _, e := range errs {
		msg := e.GetHttpMessage()
		if msg.Status == 0 {
			t.Errorf("%T has zero status", e)
		}
	}
}

func TestSentinelErrors(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrNotFound", ErrNotFound},
		{"ErrBadRequest", ErrBadRequest},
		{"ErrUnauthorized", ErrUnauthorized},
		{"ErrForbidden", ErrForbidden},
		{"ErrInternalServerError", ErrInternalServerError},
		{"ErrMethodNotAllowed", ErrMethodNotAllowed},
		{"ErrPageExpired", ErrPageExpired},
		{"ErrUnprocessableEntity", ErrUnprocessableEntity},
		{"ErrServiceNotFound", ErrServiceNotFound},
	}
	for _, s := range sentinels {
		if s.err == nil {
			t.Errorf("%s should not be nil", s.name)
		}
	}
}

func TestErrorStatusCodes(t *testing.T) {
	tests := []struct {
		err    HttpError
		status int
	}{
		{&NotFoundError{HttpMessage{Status: http.StatusNotFound}}, http.StatusNotFound},
		{&BadRequestError{HttpMessage{Status: http.StatusBadRequest}}, http.StatusBadRequest},
		{&UnauthorizedError{HttpMessage{Status: http.StatusUnauthorized}}, http.StatusUnauthorized},
		{&ForbiddenError{HttpMessage{Status: http.StatusForbidden}}, http.StatusForbidden},
		{&InternalServerError{HttpMessage{Status: http.StatusInternalServerError}}, http.StatusInternalServerError},
		{&MethodNotAllowedError{HttpMessage{Status: http.StatusMethodNotAllowed}}, http.StatusMethodNotAllowed},
		{&PageExpiredError{HttpMessage{Status: 419}}, 419},
		{&UnprocessableEntityError{HttpMessage{Status: http.StatusUnprocessableEntity}}, http.StatusUnprocessableEntity},
	}
	for _, tt := range tests {
		got := tt.err.GetHttpMessage().Status
		if got != tt.status {
			t.Errorf("%T status = %d, want %d", tt.err, got, tt.status)
		}
	}
}

func TestErrorWithCustomMessage(t *testing.T) {
	err := &NotFoundError{HttpMessage{Status: http.StatusNotFound, Message: "user not found"}}
	if err.Error() == "" {
		t.Error("expected non-empty error string")
	}
}

func TestInternalServerErrorMessage(t *testing.T) {
	err := &InternalServerError{HttpMessage{Status: http.StatusInternalServerError, Message: "db connection failed"}}
	if err.Error() == "" {
		t.Error("expected non-empty error string")
	}
}

func TestHttpErrorAs(t *testing.T) {
	err := &NotFoundError{HttpMessage{Status: http.StatusNotFound}}
	var httpErr HttpError
	if !errors.As(err, &httpErr) {
		t.Error("NotFoundError should be assertable as HttpError")
	}
	if httpErr.GetHttpMessage().Status != http.StatusNotFound {
		t.Errorf("expected 404, got %d", httpErr.GetHttpMessage().Status)
	}
}
