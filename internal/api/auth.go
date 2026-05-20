package api

import (
	"crypto/subtle"
	"net/http"
)

// auth wraps a handler with bearer-token authentication. When the configured
// token is empty, the protected endpoints are refused outright rather than
// left open — an unauthenticated /send is a footgun.
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.deps.Token == "" {
			writeJSON(w, http.StatusServiceUnavailable, errBody("api token not configured; refusing protected request"))
			return
		}
		got := r.Header.Get("Authorization")
		want := "Bearer " + s.deps.Token
		if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			writeJSON(w, http.StatusUnauthorized, errBody("unauthorized"))
			return
		}
		next.ServeHTTP(w, r)
	})
}
