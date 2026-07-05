package app

import (
	"errors"
	"net/http"
)

var (
	// Sentinel error values for HTTP errors.
	ErrUnauthorized           = &UnauthorizedError{HttpMessage{http.StatusUnauthorized, "Unauthorized"}}
	ErrForbidden              = &ForbiddenError{HttpMessage{http.StatusForbidden, "Forbidden"}}
	ErrNotFound               = &NotFoundError{HttpMessage{http.StatusNotFound, "Not Found"}}
	ErrBadRequest             = &BadRequestError{HttpMessage{http.StatusBadRequest, "Bad Request"}}
	ErrMethodNotAllowed       = &MethodNotAllowedError{HttpMessage{http.StatusMethodNotAllowed, "Method Not Allowed"}}
	ErrPageExpired            = &PageExpiredError{HttpMessage{419, "Page Expired"}}
	ErrUnprocessableEntity    = &UnprocessableEntityError{HttpMessage{http.StatusUnprocessableEntity, "Unprocessable Entity"}}
	ErrInternalServerError    = &InternalServerError{HttpMessage{http.StatusInternalServerError, "Internal Server Error"}}
	ErrServiceNotFound 		  = errors.New("service not found")
)

type HttpMessage struct {
	Status  int
	Message string
}

type HttpError interface {
	GetHttpMessage() HttpMessage
}

func (he *HttpMessage) Error() string {
	return he.Message
}

type UnauthorizedError struct {
	HttpMessage
}

func (he *UnauthorizedError) GetHttpMessage() HttpMessage {
	return he.HttpMessage
}

type ForbiddenError struct {
	HttpMessage
}

func (he *ForbiddenError) GetHttpMessage() HttpMessage {
	return he.HttpMessage
}

type NotFoundError struct {
	HttpMessage
}

func (he *NotFoundError) GetHttpMessage() HttpMessage {
	return he.HttpMessage
}

type BadRequestError struct {
	HttpMessage
}

func (he *BadRequestError) GetHttpMessage() HttpMessage {
	return he.HttpMessage
}

type MethodNotAllowedError struct {
	HttpMessage
}

func (he *MethodNotAllowedError) GetHttpMessage() HttpMessage {
	return he.HttpMessage
}

type PageExpiredError struct {
	HttpMessage
}

func (he *PageExpiredError) GetHttpMessage() HttpMessage {
	return he.HttpMessage
}

type UnprocessableEntityError struct {
	HttpMessage
}

func (he *UnprocessableEntityError) GetHttpMessage() HttpMessage {
	return he.HttpMessage
}

type InternalServerError struct {
	HttpMessage
}

func (he *InternalServerError) GetHttpMessage() HttpMessage {
	return he.HttpMessage
}

type ErrMap = map[error]Handler
