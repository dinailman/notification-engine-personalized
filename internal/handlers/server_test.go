package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeRejectsUnknownAndMultipleValues(t *testing.T) {
	for _, body := range []string{`{"email":"a@example.com","name":"A","extra":true}`, `{"email":"a@example.com","name":"A"}{}`} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
		if decode(w, r, &userRequest{}) {
			t.Fatal("invalid body accepted")
		}
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d", w.Code)
		}
	}
}
func TestDateRangeDefaultsAndValidation(t *testing.T) {
	from, to, err := dateRange("", "")
	if err != nil || !from.Before(to) {
		t.Fatal("default date range invalid")
	}
	if _, _, err = dateRange("bad", ""); err == nil {
		t.Fatal("invalid from accepted")
	}
	if _, _, err = dateRange("2026-09-02", "2026-09-01"); err == nil {
		t.Fatal("reversed range accepted")
	}
}
