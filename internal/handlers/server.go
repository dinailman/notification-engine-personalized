package handlers

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dinailman/personalized-notification-engine/internal/models"
	"github.com/dinailman/personalized-notification-engine/internal/queue"
	"github.com/dinailman/personalized-notification-engine/internal/repositories"
	"github.com/dinailman/personalized-notification-engine/internal/rules"
)

type Server struct {
	Repo                 *repositories.Repository
	Queue                *queue.Queue
	Logger               *slog.Logger
	RateLimit            int
	APIKey               string
	Requests             atomic.Uint64
	NotificationsCreated atomic.Uint64
}

// log returns the server's logger, falling back to the default one so a Server built
// without a logger -- as the handler tests do -- still works.
func (s *Server) log() *slog.Logger {
	if s.Logger == nil {
		return slog.Default()
	}
	return s.Logger
}

type userRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Timezone string `json:"timezone"`
	Active   *bool  `json:"active"`
	// QuietHoursStart and QuietHoursEnd are "15:04" wall clock in the user's timezone.
	// Both must be given together; both empty means the user has no quiet window.
	QuietHoursStart string              `json:"quiet_hours_start"`
	QuietHoursEnd   string              `json:"quiet_hours_end"`
	Preferences     []models.Preference `json:"preferences"`
}
type ruleRequest struct {
	Name            string `json:"name"`
	TriggerType     string `json:"trigger_type"`
	EventType       string `json:"event_type"`
	ScheduledTime   string `json:"scheduled_time"`
	Frequency       string `json:"frequency"`
	Channel         string `json:"channel"`
	SubjectTemplate string `json:"subject_template"`
	BodyTemplate    string `json:"body_template"`
	Enabled         *bool  `json:"enabled"`
}
type eventRequest struct {
	UserID     string         `json:"user_id"`
	EventType  string         `json:"event_type"`
	ExternalID string         `json:"external_id"`
	Payload    map[string]any `json:"payload"`
	OccurredAt *time.Time     `json:"occurred_at"`
}

func (s *Server) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req userRequest
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Email) == "" || strings.TrimSpace(req.Name) == "" {
		errorJSON(w, 400, "email and name are required")
		return
	}
	tz := req.Timezone
	if tz == "" {
		tz = "UTC"
	}
	if _, err := time.LoadLocation(tz); err != nil {
		errorJSON(w, 400, "timezone is invalid")
		return
	}
	if !validQuietHours(req) {
		errorJSON(w, 400, quietHoursMessage)
		return
	}
	u, err := s.Repo.CreateUser(r.Context(), models.User{Email: strings.ToLower(strings.TrimSpace(req.Email)), Name: strings.TrimSpace(req.Name), Timezone: tz, Active: true, QuietHoursStart: req.QuietHoursStart, QuietHoursEnd: req.QuietHoursEnd}, req.Preferences)
	if err != nil {
		errorJSON(w, 409, "user or preference already exists")
		return
	}
	jsonResponse(w, 201, u)
}
func (s *Server) GetUser(w http.ResponseWriter, r *http.Request) {
	u, err := s.Repo.GetUser(r.Context(), r.PathValue("id"))
	if err != nil {
		notFound(w, err)
		return
	}
	jsonResponse(w, 200, u)
}
func (s *Server) UpdateUser(w http.ResponseWriter, r *http.Request) {
	var req userRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Name == "" || req.Timezone == "" {
		errorJSON(w, 400, "name and timezone are required")
		return
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	if _, err := time.LoadLocation(req.Timezone); err != nil {
		errorJSON(w, 400, "timezone is invalid")
		return
	}
	if !validQuietHours(req) {
		errorJSON(w, 400, quietHoursMessage)
		return
	}
	// An update carrying no quiet hours clears the window the user had before.
	u, err := s.Repo.UpdateUser(r.Context(), r.PathValue("id"), models.User{Name: req.Name, Timezone: req.Timezone, Active: active, QuietHoursStart: req.QuietHoursStart, QuietHoursEnd: req.QuietHoursEnd})
	if err != nil {
		notFound(w, err)
		return
	}
	jsonResponse(w, 200, u)
}
func (s *Server) Preferences(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if r.Method == http.MethodGet {
		p, err := s.Repo.Preferences(r.Context(), id)
		if err != nil {
			errorJSON(w, 500, "could not load preferences")
			return
		}
		jsonResponse(w, 200, p)
		return
	}
	var req []models.Preference
	if !decode(w, r, &req) {
		return
	}
	p, err := s.Repo.ReplacePreferences(r.Context(), id, req)
	if err != nil {
		errorJSON(w, 400, err.Error())
		return
	}
	jsonResponse(w, 200, p)
}
func (s *Server) CreateRule(w http.ResponseWriter, r *http.Request) {
	var req ruleRequest
	if !decode(w, r, &req) {
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	x, err := s.Repo.CreateRule(r.Context(), models.Rule{UserID: r.PathValue("id"), Name: req.Name, TriggerType: req.TriggerType, EventType: req.EventType, ScheduledTime: req.ScheduledTime, Frequency: req.Frequency, Channel: req.Channel, SubjectTemplate: req.SubjectTemplate, BodyTemplate: req.BodyTemplate, Enabled: enabled})
	if err != nil {
		errorJSON(w, 400, err.Error())
		return
	}
	jsonResponse(w, 201, x)
}
func (s *Server) ListRules(w http.ResponseWriter, r *http.Request) {
	x, err := s.Repo.ListRules(r.Context(), r.PathValue("id"))
	if err != nil {
		errorJSON(w, 500, "could not list rules")
		return
	}
	jsonResponse(w, 200, x)
}
func (s *Server) UpdateRule(w http.ResponseWriter, r *http.Request) {
	var req ruleRequest
	if !decode(w, r, &req) {
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	x, err := s.Repo.UpdateRule(r.Context(), r.PathValue("id"), enabled, req.SubjectTemplate, req.BodyTemplate)
	if err != nil {
		notFound(w, err)
		return
	}
	jsonResponse(w, 200, x)
}
func (s *Server) DeleteRule(w http.ResponseWriter, r *http.Request) {
	if err := s.Repo.DeleteRule(r.Context(), r.PathValue("id")); err != nil {
		notFound(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) CreateEvent(w http.ResponseWriter, r *http.Request) {
	var req eventRequest
	if !decode(w, r, &req) {
		return
	}
	if req.UserID == "" || req.EventType == "" {
		errorJSON(w, 400, "user_id and event_type are required")
		return
	}
	occurred := time.Now().UTC()
	if req.OccurredAt != nil {
		occurred = req.OccurredAt.UTC()
	}
	event, created, err := s.Repo.CreateEvent(r.Context(), models.Event{UserID: req.UserID, EventType: req.EventType, ExternalID: req.ExternalID, Payload: req.Payload, OccurredAt: occurred})
	if err != nil {
		errorJSON(w, 409, "event could not be recorded")
		return
	}
	ids := make([]string, 0, len(created))
	deferred := make([]string, 0, len(created))
	for _, c := range created {
		ids = append(ids, c.ID)
		s.NotificationsCreated.Add(1)
		// A notification held for the user's quiet window is not queued now; the
		// worker's recovery loop picks it up once the window closes.
		if c.Deferred {
			deferred = append(deferred, c.ID)
			s.log().Info("notification deferred for quiet hours", "notification_id", c.ID, "user_id", req.UserID, "deliver_at", c.DeliverAt)
			continue
		}
		if err := s.Queue.Enqueue(r.Context(), c.ID); err != nil {
			errorJSON(w, 503, "event recorded but notification queue is unavailable")
			return
		}
	}
	jsonResponse(w, 202, map[string]any{"event": event, "notification_ids": ids, "deferred_notification_ids": deferred})
}
func (s *Server) ListEvents(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Repo.DB.Query(r.Context(), `SELECT id,user_id,event_type,COALESCE(external_id,''),payload,occurred_at,created_at FROM events WHERE user_id=$1 ORDER BY occurred_at DESC LIMIT 100`, r.PathValue("id"))
	if err != nil {
		errorJSON(w, 500, "could not list events")
		return
	}
	defer rows.Close()
	out := []models.Event{}
	for rows.Next() {
		var e models.Event
		var raw []byte
		if err := rows.Scan(&e.ID, &e.UserID, &e.EventType, &e.ExternalID, &raw, &e.OccurredAt, &e.CreatedAt); err != nil {
			errorJSON(w, 500, "could not read events")
			return
		}
		_ = json.Unmarshal(raw, &e.Payload)
		out = append(out, e)
	}
	jsonResponse(w, 200, out)
}
func (s *Server) ListNotifications(w http.ResponseWriter, r *http.Request) {
	x, err := s.Repo.ListNotifications(r.Context(), r.PathValue("id"))
	if err != nil {
		errorJSON(w, 500, "could not list notifications")
		return
	}
	jsonResponse(w, 200, x)
}
func (s *Server) GetNotification(w http.ResponseWriter, r *http.Request) {
	x, err := s.Repo.GetNotification(r.Context(), r.PathValue("id"))
	if err != nil {
		notFound(w, err)
		return
	}
	jsonResponse(w, 200, x)
}
func (s *Server) NotificationLogs(w http.ResponseWriter, r *http.Request) {
	x, err := s.Repo.Logs(r.Context(), r.PathValue("id"))
	if err != nil {
		errorJSON(w, 500, "could not list notification logs")
		return
	}
	jsonResponse(w, 200, x)
}
func (s *Server) Analytics(w http.ResponseWriter, r *http.Request) {
	from, to, err := dateRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		errorJSON(w, 400, err.Error())
		return
	}
	x, err := s.Repo.Analytics(r.Context(), from, to, r.URL.Query().Get("user_id"), r.URL.Query().Get("channel"))
	if err != nil {
		errorJSON(w, 500, "could not build analytics")
		return
	}
	jsonResponse(w, 200, x)
}
func (s *Server) Metrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	depth, _ := s.Queue.Depth(context.Background())
	fmt.Fprintf(w, "# TYPE http_requests_total counter\nhttp_requests_total %d\n# TYPE notifications_created_total counter\nnotifications_created_total %d\n# TYPE notification_queue_depth gauge\nnotification_queue_depth %d\n", s.Requests.Load(), s.NotificationsCreated.Load(), depth)
}
func (s *Server) Health(w http.ResponseWriter, r *http.Request) {
	healthCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.Repo.Health(healthCtx); err != nil {
		errorJSON(w, 503, "database unavailable")
		return
	}
	if err := s.Queue.Ping(healthCtx); err != nil {
		errorJSON(w, 503, "redis unavailable")
		return
	}
	jsonResponse(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) RateLimited(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Requests.Add(1)
		if r.URL.Path == "/healthz" || r.URL.Path == "/metrics" || s.RateLimit <= 0 {
			next.ServeHTTP(w, r)
			return
		}
		allowed, err := s.Queue.Allow(r.Context(), r.RemoteAddr, s.RateLimit)
		if err != nil {
			errorJSON(w, 503, "rate limiter unavailable")
			return
		}
		if !allowed {
			errorJSON(w, 429, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) RequireAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/metrics" || r.URL.Path == "/openapi.json" || s.APIKey == "" {
			next.ServeHTTP(w, r)
			return
		}
		provided := r.Header.Get("X-API-Key")
		if subtle.ConstantTimeCompare([]byte(provided), []byte(s.APIKey)) != 1 {
			errorJSON(w, http.StatusUnauthorized, "missing or invalid API key")
			return
		}
		next.ServeHTTP(w, r)
	})
}

const quietHoursMessage = "quiet_hours_start and quiet_hours_end must both be HH:MM and must differ, or both be omitted"

// validQuietHours reports whether the optional quiet window on a user request is usable.
// Omitting both halves is valid and means the user has no quiet window.
func validQuietHours(req userRequest) bool {
	if req.QuietHoursStart == "" && req.QuietHoursEnd == "" {
		return true
	}
	return rules.ValidQuietHours(req.QuietHoursStart, req.QuietHoursEnd)
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		errorJSON(w, 400, "invalid JSON body")
		return false
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		errorJSON(w, 400, "request body must contain one JSON value")
		return false
	}
	return true
}
func dateRange(from, to string) (time.Time, time.Time, error) {
	end := time.Now().UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
	start := end.Add(-7 * 24 * time.Hour)
	if from != "" {
		x, err := time.Parse("2006-01-02", from)
		if err != nil {
			return start, end, errors.New("from must use YYYY-MM-DD")
		}
		start = x.UTC()
	}
	if to != "" {
		x, err := time.Parse("2006-01-02", to)
		if err != nil {
			return start, end, errors.New("to must use YYYY-MM-DD")
		}
		end = x.UTC().Add(24 * time.Hour)
	}
	if !start.Before(end) {
		return start, end, errors.New("from must be before to")
	}
	return start, end, nil
}
func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func errorJSON(w http.ResponseWriter, status int, msg string) {
	jsonResponse(w, status, map[string]string{"error": msg})
}
func notFound(w http.ResponseWriter, err error) {
	if errors.Is(err, repositories.ErrNotFound) {
		errorJSON(w, 404, "resource not found")
		return
	}
	errorJSON(w, 500, "database operation failed")
}
