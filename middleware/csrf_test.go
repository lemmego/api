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

// TestExclusionPatternsCannotBeSmuggledIntoAPath is the regression for an
// unanchored exclusion match.
//
// The patterns were tested with regexp.MatchString, which is "contains". So
// the pattern every project writes — "/api/.*" — excluded any path with
// "/api/" anywhere in it, and a state-changing route whose own path happened
// to contain that segment had CSRF silently off. Nothing in the exclusion
// list showed it, which is what makes it the expensive kind of bug.
func TestExclusionPatternsCannotBeSmuggledIntoAPath(t *testing.T) {
	patterns := []string{`/api/.*`, `/webhooks/stripe`}

	excluded := []string{
		"/api/users",
		"/api/",
		"/webhooks/stripe",
	}
	for _, path := range excluded {
		if !shouldExcludePath(path, patterns) {
			t.Errorf("%s should be excluded", path)
		}
	}

	notExcluded := []string{
		// The smuggling case: not under /api/ at all.
		"/jobs/cool-startup/api/delete",
		"/circles/api/members",
		"/@ada/api/follow",
		"/settings/profile",
		"/",
	}
	for _, path := range notExcluded {
		if shouldExcludePath(path, patterns) {
			t.Errorf("%s was excluded from CSRF verification", path)
		}
	}
}

// TestAnchoredPatternsStillWork: a pattern that anchors itself is the other
// style people write, and wrapping must not break it.
func TestAnchoredPatternsStillWork(t *testing.T) {
	patterns := []string{`^/api/`, `^/hooks/.*$`}

	for _, path := range []string{"/api/users", "/hooks/x"} {
		if !shouldExcludePath(path, patterns) {
			t.Errorf("%s should be excluded", path)
		}
	}
	for _, path := range []string{"/jobs/x/api/y", "/hooks"} {
		if shouldExcludePath(path, patterns) {
			t.Errorf("%s was excluded", path)
		}
	}
}

// TestAlternationBindsAcrossTheWholePattern: anchoring by string
// concatenation would turn `a|b` into `\Aa|b`, which anchors only the first
// branch and leaves the second one matching anywhere.
func TestAlternationBindsAcrossTheWholePattern(t *testing.T) {
	patterns := []string{`/api/.*|/webhooks/.*`}

	if !shouldExcludePath("/webhooks/stripe", patterns) {
		t.Error("/webhooks/stripe should be excluded by the second branch")
	}
	if shouldExcludePath("/jobs/x/webhooks/y", patterns) {
		t.Error("the second branch matched in the middle of a path")
	}
}

// TestAnExactPatternStaysExact documents the escape hatch: end the pattern
// with $ when a prefix is not what you meant.
func TestAnExactPatternStaysExact(t *testing.T) {
	patterns := []string{`/login$`}

	if !shouldExcludePath("/login", patterns) {
		t.Error("/login should be excluded")
	}
	if shouldExcludePath("/login-with-github", patterns) {
		t.Error("/login-with-github was excluded by an exact pattern")
	}
}
