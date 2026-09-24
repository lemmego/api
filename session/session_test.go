package session

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alexedwards/scs/v2"
)

func TestTypedSessionAccessors(t *testing.T) {
	manager := scs.New()
	session := &Session{SessionManager: manager}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()

	manager.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session.Put(r.Context(), "count", 7)
		count, ok := session.GetAs[int](r.Context(), "count")
		if !ok || count != 7 {
			t.Fatalf("get typed session value = %d, %v", count, ok)
		}
		popped, ok := session.PopAs[int](r.Context(), "count")
		if !ok || popped != 7 {
			t.Fatalf("pop typed session value = %d, %v", popped, ok)
		}
		if _, ok := session.GetAs[int](r.Context(), "count"); ok {
			t.Fatal("popped session value should no longer be present")
		}
	})).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("session request status = %d", response.Code)
	}
}
