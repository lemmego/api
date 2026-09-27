package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A package that mounts a third-party http.Handler on the router needs some
// way to run a framework concern in front of it. Before NewContext, ctx was
// unexported and there was none: the tasker dashboard shipped mounted and
// permanently unreachable because nothing outside this package could build
// the Context its authentication check needed.
func TestNewContextExposesTheRequestToRawHandlers(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/tasker/api/jobs", nil)
	request.Header.Set("Authorization", "Bearer t0ken")
	recorder := httptest.NewRecorder()

	c := NewContext(nil, recorder, request)

	if got := c.Header("Authorization"); got != "Bearer t0ken" {
		t.Errorf("Header = %q", got)
	}
	if c.Request() != request {
		t.Error("Request is not the one passed in")
	}
	if c.ResponseWriter() != recorder {
		t.Error("ResponseWriter is not the one passed in")
	}
}

// Set rebinds the request, so anything downstream has to be handed
// c.Request() rather than the original. This is the trap the doc comment
// warns about, pinned so the behaviour cannot drift silently.
func TestNewContextValuesTravelOnTheRebindRequest(t *testing.T) {
	original := httptest.NewRequest(http.MethodGet, "/tasker/", nil)
	c := NewContext(nil, httptest.NewRecorder(), original)

	c.Set("who", "ada")

	if got := c.Get("who"); got != "ada" {
		t.Fatalf("Get = %v", got)
	}
	if c.Request() == original {
		t.Fatal("Set did not rebind the request")
	}
	if got := original.Context().Value("who"); got != nil {
		t.Errorf("the original request should not carry the value, got %v", got)
	}
	if got := c.Request().Context().Value("who"); got != "ada" {
		t.Errorf("the rebound request should carry the value, got %v", got)
	}
}

// With no handler chain, Next must terminate rather than run off the end of
// the slice.
func TestNewContextNextIsANoOp(t *testing.T) {
	c := NewContext(nil, httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if err := c.Next(); err != nil {
		t.Fatalf("Next = %v", err)
	}
	if err := c.Next(); err != nil {
		t.Fatalf("second Next = %v", err)
	}
}
