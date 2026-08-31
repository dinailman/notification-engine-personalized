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

// The rate limiter counts against clientIP, so it must strip the ephemeral port. Keying on
// the raw RemoteAddr gave every new connection its own budget and the limit never applied.
func TestClientIPStripsThePort(t *testing.T) {
	for _, testCase := range []struct{ remoteAddr, want string }{
		{"192.0.2.1:54321", "192.0.2.1"},
		{"192.0.2.1:54322", "192.0.2.1"},
		{"[2001:db8::1]:443", "2001:db8::1"},
		{"unix-socket-or-otherwise-unsplittable", "unix-socket-or-otherwise-unsplittable"},
	} {
		r := httptest.NewRequest(http.MethodGet, "/users", nil)
		r.RemoteAddr = testCase.remoteAddr
		if got := clientIP(r); got != testCase.want {
			t.Errorf("clientIP(%q) = %q, want %q", testCase.remoteAddr, got, testCase.want)
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

func TestValidUUID(t *testing.T) {
	for _, id := range []string{
		"0f8fad5b-d9cb-469f-a165-70867728950e",
		"0F8FAD5B-D9CB-469F-A165-70867728950E",
		"00000000-0000-0000-0000-000000000000",
	} {
		if !validUUID(id) {
			t.Errorf("validUUID(%q) = false, want true", id)
		}
	}
	for _, id := range []string{
		"",
		"not-a-uuid",
		"0f8fad5b-d9cb-469f-a165-70867728950",    // one short
		"0f8fad5b-d9cb-469f-a165-70867728950ef",  // one long
		"0f8fad5bd9cb469fa16570867728950e",       // undashed, which Postgres takes and we do not
		"{0f8fad5b-d9cb-469f-a165-70867728950e}", // braced, likewise
		"0f8fad5b-d9cb-469f-a165-70867728950g",   // g is not hex
		"0f8fad5bd-9cb-469f-a165-70867728950e",   // dashes misplaced
		"0f8fad5bd9cb469fa16570867728950eabcd",   // 36 hex characters and no dashes at all
	} {
		if validUUID(id) {
			t.Errorf("validUUID(%q) = true, want false", id)
		}
	}
}

// Every handler that takes an {id} must reject a malformed one itself. The Server here has a
// nil Repo, so a handler that lets the id through panics instead of answering 400 -- which is
// the point: reaching the driver is what turned a client mistake into a 500.
func TestHandlersRejectMalformedIDs(t *testing.T) {
	s := &Server{}
	for _, testCase := range []struct {
		name    string
		handler http.HandlerFunc
		body    string
	}{
		{"GetUser", s.GetUser, ""},
		{"UpdateUser", s.UpdateUser, "{}"},
		{"Preferences", s.Preferences, ""},
		{"CreateRule", s.CreateRule, "{}"},
		{"ListRules", s.ListRules, ""},
		{"UpdateRule", s.UpdateRule, "{}"},
		{"DeleteRule", s.DeleteRule, ""},
		{"ListEvents", s.ListEvents, ""},
		{"ListNotifications", s.ListNotifications, ""},
		{"GetNotification", s.GetNotification, ""},
		{"NotificationLogs", s.NotificationLogs, ""},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(testCase.body))
		r.SetPathValue("id", "not-a-uuid")
		testCase.handler(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s status = %d, want 400", testCase.name, w.Code)
		}
	}
}

// The other two ways a user id arrives: the body of an ingest, where a malformed one used to
// be answered 409 as though the event conflicted, and the optional analytics filter.
func TestMalformedUserIDOutsideThePath(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(`{"user_id":"nope","event_type":"task_completed"}`))
	(&Server{}).CreateEvent(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("POST /events status = %d, want 400", w.Code)
	}
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/analytics/notifications?user_id=nope", nil)
	(&Server{}).Analytics(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("GET /analytics/notifications status = %d, want 400", w.Code)
	}
}
