package server

import (
	"net/http"
	"strings"
)

func WithCORS(next http.Handler, configuredOrigins string) http.Handler {
	origins := make(map[string]struct{})
	allowAny := false
	for _, origin := range strings.Split(configuredOrigins, ",") {
		origin = strings.TrimSpace(origin)
		if origin == "*" {
			allowAny = true
		}
		if origin != "" {
			origins[origin] = struct{}{}
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		_, allowed := origins[origin]
		allowed = allowed || allowAny
		if origin != "" && allowed {
			if allowAny {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			if origin != "" && !allowed {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
