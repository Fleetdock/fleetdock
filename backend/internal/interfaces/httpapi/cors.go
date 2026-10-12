package httpapi

import "net/http"

// cors applies permissive-but-configurable CORS for the web frontend and
// short-circuits preflight requests before they reach authentication.
func cors(origin string, credentials bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Add("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, "+csrfHeader)
		// The session cookie is shared cross-origin only when an operator
		// opts in for a split-origin dashboard, and never with "*".
		if credentials && origin != "" && origin != "*" {
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
