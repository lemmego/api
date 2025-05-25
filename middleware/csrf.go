package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"github.com/lemmego/api/app"
	"github.com/lemmego/api/config"
	"github.com/lemmego/api/req"
	inertia "github.com/romsar/gonertia"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

func getRandomToken(length int) string {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		slog.Error("Critical error generating random token:", err)
		panic("Failed to generate CSRF token")
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func matchedToken(c *app.Context) bool {
	sessionToken := c.GetSessionString("_token")
	token := getTokenFromRequest(c)

	matched := false
	if sessionToken != "" && token != "" {
		matched = sessionToken == token
	}

	if matched {
		c.PutSession("_token", getRandomToken(40))
	}

	return matched
}

func getTokenFromRequest(c *app.Context) string {
	token := c.GetHeader("X-XSRF-TOKEN")
	if token == "" {
		token = c.Request().PostFormValue("_token")
	}
	if token == "" {
		token = c.Request().FormValue("_token")
	}

	//if token == "" {
	//	if csrfCookie, err := c.Request().Cookie("XSRF-TOKEN"); err == nil {
	//		token = strings.TrimSpace(csrfCookie.Value)
	//	}
	//}

	if token == "" {
		body := map[string]any{}
		if err := req.DecodeJSONBody(c.ResponseWriter(), c.Request(), &body); err != nil {
			token = ""
		}
		if val, ok := body["_token"].(string); ok {
			token = val
		}
	}
	return token
}

func VerifyCSRF(c *app.Context) error {
	if c.IsReading() || matchedToken(c) {
		if c.WantsHTML() && !strings.HasPrefix(c.Request().URL.Path, "/static") {
			token := ""
			if val, ok := c.GetSession("_token").(string); ok && val != "" {
				token = val
			} else {
				token = getRandomToken(40)
			}
			c.PutSession("_token", token)
			c.Set("_token", token)

			var i *inertia.Inertia
			if err := c.App().Service(&i); err == nil {
				i.ShareProp("csrfToken", token)
			}

			c.SetCookie(&http.Cookie{
				Name:     "XSRF-TOKEN",
				Value:    token,
				Expires:  time.Now().Add(config.Get("session.lifetime").(time.Duration)),
				Path:     "/",
				Domain:   "",
				Secure:   c.App().InProduction(),
				HttpOnly: false,
				SameSite: http.SameSiteLaxMode, // Prevents the browser from sending this cookie along with cross-site requests
			})
		}
		return c.Next()
	}

	return c.PageExpired()
}
