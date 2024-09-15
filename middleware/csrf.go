package middleware

import (
	"net/http"
	"strings"

	"github.com/lemmego/api/app"
	"github.com/lemmego/api/req"
	"github.com/lemmego/api/utils"
)

func matchedToken(c *app.Context) bool {
	sessionToken := c.GetSessionString("_token")
	token := getTokenFromRequest(c)

	println("sessionToken", sessionToken, "inputToken", token)

	matched := false
	if sessionToken != "" && token != "" {
		matched = sessionToken == token
	}

	if matched {
		c.PutSession("_token", utils.GenerateRandomString(40))
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
				token = utils.GenerateRandomString(40)
			}
			c.PutSession("_token", token)

			if c.I() != nil {
				c.I().ShareProp("csrfToken", token)
			}

			http.SetCookie(c.ResponseWriter(), &http.Cookie{
				Name:     "XSRF-TOKEN",
				Value:    token,
				Path:     "/",
				HttpOnly: true,                    // Not accessible via JavaScript
				Secure:   true,                    // Send only over HTTPS
				SameSite: http.SameSiteStrictMode, // Prevents the browser from sending this cookie along with cross-site requests
			})
		}
		return c.Next()
	}

	return c.PageExpired()
}
