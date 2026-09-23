package fleet

import "net/http"

// requireToken wraps a handler with shared-token authentication.
//
// An empty token never disables authentication: the wrapper fails closed, so a
// dispatcher built without a token rejects every request. Serving without
// authentication is an explicit decision made by the caller through
// DispatcherConfig.AllowAnonymous.
func requireToken(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token == "" || r.Header.Get("X-Auth-Token") != token {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}
