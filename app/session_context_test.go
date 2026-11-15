package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alexedwards/scs/v2"
	"github.com/lemmego/api/session"
)

// The session id has to be on the context before HTTP middleware runs.
//
// It was put there by the route dispatcher instead (makeHandlerFunc), which is
// inside the mux — so it arrived *after* every HTTPMiddleware had already run
// and returned. Anything installed with Router().Use that keys on a session
// therefore saw nothing, on every request, silently.
//
// lemmego/inertia is the consumer that paid for it. Its flash provider keys on
// "sessionID", and gonertia restores flash data from its own middleware, which
// lemmego installs with Router().Use — so a message flashed on a redirect was
// written under the session that set it and then read back with no session at
// all. page.flash was empty on every request in every Inertia project, which
// is a whole feature that worked in one direction only.
//
// The store's own tests passed throughout, because they build a context
// carrying "sessionID" by hand. That is the gap: a unit test of the store can
// only ever prove the store, and the bug was in who calls it and when.
func TestTheSessionIDIsOnTheContextBeforeHTTPMiddlewareRuns(t *testing.T) {
	a := Configure().(*application)
	manager := scs.New()
	a.AddService(session.New(manager.Store, manager.Cookie))

	var seenByMiddleware, seenByHandler any
	a.Router().Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seenByMiddleware = r.Context().Value("sessionID")
			next.ServeHTTP(w, r)
		})
	})
	a.Router().Get("/x", func(c Context) error {
		seenByHandler = c.Request().Context().Value("sessionID")
		// Something has to be written, or scs has no session to persist and
		// no cookie to send — and then the second request below is as
		// anonymous as the first.
		c.PutSession("k", "v")
		return c.Text([]byte("ok"))
	})

	handler := a.serveHandler()

	// The first request establishes the session. Its own token is empty until
	// scs commits, which is why the assertion is made on the second.
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/x", nil))

	cookies := first.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no session cookie was set, so this test cannot make its point")
	}
	request := httptest.NewRequest(http.MethodGet, "/x", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	handler.ServeHTTP(httptest.NewRecorder(), request)

	// The probe itself, first: a handler has always been able to see this, so
	// a nil here means the test is broken rather than the code.
	if seenByHandler == nil {
		t.Fatal("the handler saw no session id, so the probe is wrong")
	}
	if seenByMiddleware == nil {
		t.Error("HTTP middleware saw no session id. Anything installed with " +
			"Router().Use that keys on a session — inertia's flash provider, " +
			"for one — cannot work.")
	}
	if seenByMiddleware != seenByHandler {
		t.Errorf("middleware saw session %v and the handler saw %v", seenByMiddleware, seenByHandler)
	}
}
