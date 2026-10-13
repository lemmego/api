package middleware

import (
	"bufio"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// RequestLogger wraps the ResponseWriter to capture the status. Embedding
// http.ResponseWriter promotes only its three methods, so without an Unwrap
// every optional interface a handler might reach for disappears behind this
// middleware — which is how tasker's dashboard event stream came to answer
// 500 "streaming not supported" in a real application.
//
// These run against a real listener, because httptest.NewRecorder cannot be
// hijacked and would pass the flush case for the wrong reason.

func TestAHandlerBehindTheLoggerCanFlush(t *testing.T) {
	var flushable bool
	handler := RequestLogger()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, flushable = w.(http.Flusher)
		w.WriteHeader(http.StatusOK)
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	response, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("requesting: %v", err)
	}
	response.Body.Close()

	if !flushable {
		t.Fatal("the response writer does not flush behind RequestLogger, so no " +
			"server-sent events endpoint can work in an application that installs it")
	}
}

func TestAHandlerBehindTheLoggerCanControlItsWriteDeadline(t *testing.T) {
	var err error
	handler := RequestLogger()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// What a stream must do to outlive the server's WriteTimeout. It
		// reaches the real writer only by following Unwrap.
		err = http.NewResponseController(w).SetWriteDeadline(time.Time{})
		w.WriteHeader(http.StatusOK)
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	response, requestErr := http.Get(server.URL)
	if requestErr != nil {
		t.Fatalf("requesting: %v", requestErr)
	}
	response.Body.Close()

	if err != nil {
		t.Fatalf("clearing the write deadline: %v — a stream cannot outlive "+
			"http.Server.WriteTimeout without this", err)
	}
}

func TestAHandlerBehindTheLoggerCanHijack(t *testing.T) {
	handler := RequestLogger()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			// Reported through the response, since after a failed hijack the
			// test's own assertion is the only thing that can speak.
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		conn, buffered, err := hijacker.Hijack()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer conn.Close()
		fmt.Fprint(buffered, "HTTP/1.1 101 Switching Protocols\r\n\r\n")
		buffered.Flush()
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	response, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("requesting: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("got status %d, want 101 — a WebSocket upgrade cannot complete "+
			"behind RequestLogger", response.StatusCode)
	}
	_ = bufio.NewReader
}
