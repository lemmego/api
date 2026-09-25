package middleware

import (
	"sync"
	"testing"
)

func TestShouldExcludePathConcurrentCacheAccess(t *testing.T) {
	patterns := []string{`^/api/`, `^/hooks/.*`, `[invalid`}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				shouldExcludePath("/api/users", patterns)
			}
		}()
	}
	wg.Wait()
}

// A CSRF token is a shared secret for the session, not a per-request nonce.
// Verification therefore compares and nothing more: it does not rotate the
// token. Rotating on every verified request meant any client reusing one — a
// client that missed a Set-Cookie, had a request already in flight, came back
// from the back/forward cache, or was a second tab — got 419, and stayed
// broken until a full page load.
func TestTokensMatch(t *testing.T) {
	const token = "a-stable-token"

	if !tokensMatch(token, token) {
		t.Fatal("identical tokens must verify")
	}
	// The same pair must keep verifying; nothing is consumed.
	if !tokensMatch(token, token) {
		t.Fatal("a token must still verify on a second request")
	}

	for _, c := range []struct{ name, session, request string }{
		{"different", token, "something-else"},
		{"empty session", "", token},
		{"empty request", token, ""},
		{"both empty", "", ""},
		{"prefix only", token, token[:4]},
	} {
		if tokensMatch(c.session, c.request) {
			t.Errorf("%s: must not verify", c.name)
		}
	}
}
