package middleware

import "net/http"

// CORS removes the browser-side CORS block between the frontend (a
// dashboard running on a different origin, e.g. http://localhost:5173)
// and this API (http://localhost:8080). The browser doesn't let JS read
// the response unless it sees an Access-Control-Allow-Origin header in
// it — which shows up on the frontend side as "Failed to fetch" (the
// request actually reaches the server and gets a response, the browser
// just doesn't hand it off to JS).
//
// allowedOrigin is a parameter: as with the CORS setting in
// elasticsearch.yml, it's preferred here too to give the origin the
// dashboard actually runs on, instead of a wildcard ("*").
func CORS(allowedOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

			// Before a request other than GET/POST, or one carrying
			// custom headers, the browser sends an OPTIONS "preflight"
			// request; we close it here with 204 without ever letting it
			// reach the actual handler.
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
