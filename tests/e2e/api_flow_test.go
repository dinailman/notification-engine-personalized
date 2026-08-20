package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestEventNotificationLifecycle(t *testing.T) {
	base := strings.TrimRight(os.Getenv("E2E_BASE_URL"), "/")
	if base == "" {
		t.Skip("set E2E_BASE_URL to run against Docker Compose")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	email := fmt.Sprintf("e2e-%d@example.com", time.Now().UnixNano())
	user := post(t, client, base+"/users", map[string]any{"email": email, "name": "E2E User", "timezone": "UTC", "preferences": []any{map[string]any{"channel": "in_app", "frequency": "daily", "enabled": true}}}, http.StatusCreated, "")
	uid := user["id"].(string)
	rule := post(t, client, base+"/users/"+uid+"/rules", map[string]any{"name": "Task summary", "trigger_type": "event", "event_type": "task_completed", "channel": "in_app", "subject_template": "Summary", "body_template": "Your {{event_type}} summary is ready", "enabled": true}, http.StatusCreated, "")
	if rule["id"] == nil {
		t.Fatal("rule id missing")
	}
	event := post(t, client, base+"/events", map[string]any{"user_id": uid, "event_type": "task_completed", "external_id": email, "payload": map[string]any{"items_completed": 3}}, http.StatusAccepted, "")
	ids := event["notification_ids"].([]any)
	if len(ids) != 1 {
		t.Fatalf("notification ids = %v", ids)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		list := get(t, client, base+"/users/"+uid+"/notifications", http.StatusOK)
		items := list.([]any)
		if len(items) > 0 {
			n := items[0].(map[string]any)
			if n["status"] == "sent" {
				logs := get(t, client, base+"/notifications/"+n["id"].(string)+"/logs", http.StatusOK).([]any)
				if len(logs) != 1 {
					t.Fatalf("logs = %v", logs)
				}
				return
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatal("notification was not sent before timeout")
}

func post(t *testing.T, c *http.Client, url string, payload map[string]any, status int, token string) map[string]any {
	t.Helper()
	b, _ := json.Marshal(payload)
	r, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-API-Key", "dev-api-key")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := c.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != status {
		t.Fatalf("POST %s status=%d want=%d body=%s", url, res.StatusCode, status, raw)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func get(t *testing.T, c *http.Client, url string, status int) any {
	t.Helper()
	r, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("X-API-Key", "dev-api-key")
	res, err := c.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != status {
		t.Fatalf("GET %s status=%d want=%d body=%s", url, res.StatusCode, status, raw)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
