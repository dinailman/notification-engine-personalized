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

// A malformed quiet window is rejected at the edge, before the request reaches the
// database, and omitting the window entirely stays valid.
func TestCreateUserRejectsUnusableQuietHours(t *testing.T) {
	for _, body := range []string{
		`{"email":"a@example.com","name":"A","quiet_hours_start":"22:00"}`,
		`{"email":"a@example.com","name":"A","quiet_hours_start":"10pm","quiet_hours_end":"07:00"}`,
		`{"email":"a@example.com","name":"A","quiet_hours_start":"22:00","quiet_hours_end":"22:00"}`,
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
		(&Server{}).CreateUser(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("body %s status = %d, want 400", body, w.Code)
		}
	}
	if !validQuietHours(userRequest{}) {
		t.Fatal("a user with no quiet window was rejected")
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
