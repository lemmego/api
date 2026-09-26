package middleware

import (
	"testing"
	"time"

	"github.com/lemmego/api/config"
)

// The token cookie's lifetime was asserted straight out of the configuration,
// so a project whose session config was absent panicked on the first request
// that set the cookie — not at boot, where it would have been obvious.
func TestCookieLifetimeWithoutASessionConfig(t *testing.T) {
	config.Set("session", nil)

	if got := csrfCookieLifetime(); got != defaultCSRFCookieLifetime {
		t.Errorf("lifetime = %v, want the default", got)
	}
}

func TestCookieLifetimeFromADuration(t *testing.T) {
	config.Set("session", config.M{"lifetime": 45 * time.Minute})

	if got := csrfCookieLifetime(); got != 45*time.Minute {
		t.Errorf("lifetime = %v, want 45m", got)
	}
}

// A lifetime spelled as a plain number is minutes, matching how the session
// configuration is written, and used to be a panic.
func TestCookieLifetimeFromMinutes(t *testing.T) {
	config.Set("session", config.M{"lifetime": 120})

	if got := csrfCookieLifetime(); got != 2*time.Hour {
		t.Errorf("lifetime = %v, want 2h", got)
	}
}

func TestCookieLifetimeIgnoresNonsense(t *testing.T) {
	config.Set("session", config.M{"lifetime": "soon"})

	if got := csrfCookieLifetime(); got != defaultCSRFCookieLifetime {
		t.Errorf("lifetime = %v, want the default", got)
	}
}
