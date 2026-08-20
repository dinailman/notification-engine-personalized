package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireAPIKey(t *testing.T) {
	server := &Server{APIKey: "secret"}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := server.RequireAPIKey(next)

	request := httptest.NewRequest(http.MethodPost, "/events", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("missing key status = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/events", nil)
	request.Header.Set("X-API-Key", "secret")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("valid key status = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("health bypass status = %d", response.Code)
	}
}
