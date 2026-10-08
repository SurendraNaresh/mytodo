package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithCORSAllowsConfiguredOriginAndRejectsOthers(t *testing.T) {
	handler := WithCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), "http://192.168.1.20:9786")

	preflight := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	preflight.Header.Set("Origin", "http://192.168.1.20:9786")
	preflight.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, preflight)
	if response.Code != http.StatusNoContent || response.Header().Get("Access-Control-Allow-Origin") != "http://192.168.1.20:9786" {
		t.Fatalf("configured preflight returned %d with origin %q", response.Code, response.Header().Get("Access-Control-Allow-Origin"))
	}

	blocked := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	blocked.Header.Set("Origin", "http://192.168.1.21:9786")
	blockedResponse := httptest.NewRecorder()
	handler.ServeHTTP(blockedResponse, blocked)
	if blockedResponse.Code != http.StatusForbidden {
		t.Fatalf("unconfigured preflight returned %d, want 403", blockedResponse.Code)
	}
}
