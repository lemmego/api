package session

import (
	"context"
	"encoding/gob"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/alexedwards/scs/v2/memstore"
)

type storedType struct{ Name string }

func newTestSession(t *testing.T) *Session {
	t.Helper()
	return New(memstore.New(), scs.SessionCookie{Name: "test_session", Path: "/"})
}

// A session whose stored data cannot be decoded must be treated as no session
// at all.
//
// scs answers 500 and never calls the handler, and nothing sets an ErrorFunc,
// so one unreadable record locked that visitor out of every page — including
// the login page they would need to recover.
//
// It becomes unreadable for ordinary reasons: the record is gob-encoded as a
// unit, so a deploy that stops registering a type stored in it invalidates
// every session holding one. That is exactly what removing a gob.Register
// does.
func TestUndecodableSessionIsTreatedAsAbsent(t *testing.T) {
	gob.Register(storedType{})
	manager := newTestSession(t)

	// Commit a session holding a type, the way a logged-in user's would be.
	ctx, err := manager.Load(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	manager.Put(ctx, "user", storedType{Name: "ada"})
	token, _, err := manager.Commit(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Now stand in for a deploy that no longer registers that type, by
	// corrupting the stored bytes the same way a failed gob decode presents.
	if err := manager.Store.Commit(token, []byte("not a gob record"), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	reached := false
	handler := manager.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		if got := manager.GetString(r.Context(), "user"); got != "" {
			t.Errorf("the unreadable session leaked a value: %q", got)
		}
		w.WriteHeader(http.StatusOK)
	}))

	request := httptest.NewRequest(http.MethodGet, "/login", nil)
	request.AddCookie(&http.Cookie{Name: "test_session", Value: token})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if !reached {
		t.Fatal("the handler never ran; an unreadable session still locks the visitor out")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
}

// A readable session must still work, or the fix above has thrown the
// mechanism away along with the failure.
func TestReadableSessionStillLoads(t *testing.T) {
	manager := newTestSession(t)

	ctx, err := manager.Load(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	manager.Put(ctx, "greeting", "hello")
	token, _, err := manager.Commit(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var seen string
	handler := manager.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = manager.GetString(r.Context(), "greeting")
	}))

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: "test_session", Value: token})
	handler.ServeHTTP(httptest.NewRecorder(), request)

	if seen != "hello" {
		t.Fatalf("session value = %q, want %q", seen, "hello")
	}
}

// A request with no session cookie must not be slowed down or altered.
func TestRequestWithoutASessionIsUntouched(t *testing.T) {
	manager := newTestSession(t)

	reached := false
	handler := manager.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if !reached {
		t.Fatal("the handler did not run for a request carrying no session")
	}
}

// Other cookies must survive the one being dropped.
func TestOtherCookiesSurvive(t *testing.T) {
	manager := newTestSession(t)

	var names []string
	handler := manager.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, cookie := range r.Cookies() {
			names = append(names, cookie.Name)
		}
	}))

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: "test_session", Value: "not-a-real-token"})
	request.AddCookie(&http.Cookie{Name: "jwt", Value: "keep-me"})
	request.AddCookie(&http.Cookie{Name: "XSRF-TOKEN", Value: "keep-me-too"})
	handler.ServeHTTP(httptest.NewRecorder(), request)

	for _, want := range []string{"jwt", "XSRF-TOKEN"} {
		found := false
		for _, got := range names {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Errorf("cookie %q was dropped along with the session; got %v", want, names)
		}
	}
}
