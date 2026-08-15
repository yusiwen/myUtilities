package fleet

import "net/http"

// requireToken wraps a handler with shared-token authentication. If token is
// empty, no authentication is applied.
func requireToken(token string, next http.HandlerFunc) http.HandlerFunc {
	if token == "" {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Auth-Token") != token {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}
