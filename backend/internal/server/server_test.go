package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"eks-control-plane/backend/internal/config"
)

func newTestHandler() http.Handler {
	return New(config.Settings{CORSOrigins: []string{"http://localhost:5173"}})
}

func TestHealthReturnsOK(t *testing.T) {
	responseRecorder := httptest.NewRecorder()
	healthRequest := httptest.NewRequest(http.MethodGet, "/health", nil)
	newTestHandler().ServeHTTP(responseRecorder, healthRequest)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", responseRecorder.Code)
	}
	var responseBody map[string]string
	if err := json.NewDecoder(responseRecorder.Body).Decode(&responseBody); err != nil {
		t.Fatal(err)
	}
	if responseBody["status"] != "ok" {
		t.Errorf("body = %v, want {status: ok}", responseBody)
	}
}

func TestCORSPreflightAllowsConfiguredOrigin(t *testing.T) {
	responseRecorder := httptest.NewRecorder()
	preflightRequest := httptest.NewRequest(http.MethodOptions, "/health", nil)
	preflightRequest.Header.Set("Origin", "http://localhost:5173")
	preflightRequest.Header.Set("Access-Control-Request-Method", "GET")
	newTestHandler().ServeHTTP(responseRecorder, preflightRequest)

	if allowOriginHeader := responseRecorder.Header().Get("Access-Control-Allow-Origin"); allowOriginHeader != "http://localhost:5173" {
		t.Errorf("Access-Control-Allow-Origin = %q, want http://localhost:5173", allowOriginHeader)
	}
	if responseRecorder.Code != http.StatusNoContent {
		t.Errorf("preflight status = %d, want 204", responseRecorder.Code)
	}
}

func TestCORSDisallowsUnknownOrigin(t *testing.T) {
	responseRecorder := httptest.NewRecorder()
	preflightRequest := httptest.NewRequest(http.MethodOptions, "/health", nil)
	preflightRequest.Header.Set("Origin", "http://evil.example.com")
	preflightRequest.Header.Set("Access-Control-Request-Method", "GET")
	newTestHandler().ServeHTTP(responseRecorder, preflightRequest)

	if allowOriginHeader := responseRecorder.Header().Get("Access-Control-Allow-Origin"); allowOriginHeader != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty for unknown origin", allowOriginHeader)
	}
}

func TestHealthRejectsNonGet(t *testing.T) {
	responseRecorder := httptest.NewRecorder()
	postRequest := httptest.NewRequest(http.MethodPost, "/health", strings.NewReader(""))
	newTestHandler().ServeHTTP(responseRecorder, postRequest)

	if responseRecorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", responseRecorder.Code)
	}
}
