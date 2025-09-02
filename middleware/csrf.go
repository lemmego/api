package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"github.com/lemmego/api/app"
	"github.com/lemmego/api/config"
	"github.com/lemmego/api/req"
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

func matchedToken(c app.HttpProvider) bool {
	sessionToken := c.SessionString("_token")
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

func getTokenFromRequest(c app.HttpProvider) string {
	token := c.Header("X-XSRF-TOKEN")
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

func VerifyCSRF(c app.Context) error {
	if c.IsReading() || matchedToken(c) {
		if c.WantsHTML() && !strings.HasPrefix(c.Request().URL.Path, "/static") {
			token := ""
			if val, ok := c.Session("_token").(string); ok && val != "" {
				token = val
			} else {
				token = getRandomToken(40)
			}
			c.PutSession("_token", token)
			c.Set("_token", token)

			// TODO: Find a way to share the token with inertia
			//i, err := di.Resolve[*inertia.Inertia](c.App().Container())
			//
			//if err == nil && i != nil {
			//	i.ShareProp("csrfToken", token)
			//}

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
