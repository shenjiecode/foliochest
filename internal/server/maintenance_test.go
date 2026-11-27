package server

import (
	"encoding/json"
	"github.com/shenjiecode/keyfolio/internal/vault"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

func TestMaintenanceRequiresHumanSessionAndCSRF(t *testing.T) {
	b := setupHTTP(t)
	original := b.seed()
	for _, route := range []string{"/api/history/restore", "/api/backup", "/api/settings", "/api/activity", "/api/vault/restore"} {
		code, _ := b.request("POST", route, map[string]any{}, "invalid")
		if code != 403 {
			t.Fatal(route, code)
		}
	}
	code, body := b.request("GET", "/api/history?key=demo/deploy", nil, "")
	if code != 200 {
		t.Fatal(code, string(body))
	}
	var history vault.History
	if err := json.Unmarshal(body, &history); err != nil {
		t.Fatal(err)
	}
	code, body = b.request("POST", "/api/history/restore", vault.RestoreChange{Key: "demo/deploy", SourceRevision: original.Revision, ExpectedHeads: history.Heads}, b.csrf)
	if code != 200 {
		t.Fatal(code, string(body))
	}
	code, body = b.request("POST", "/api/backup", map[string]any{}, b.csrf)
	if code != 200 {
		t.Fatal(code, string(body))
	}
	destination := filepath.Join(t.TempDir(), "restored")
	code, result := b.request("POST", "/api/vault/restore", map[string]any{"archive": body, "directory": destination, "password": "synthetic-test-password", "confirmed": true}, b.csrf)
	if code != 200 {
		t.Fatal(code, string(result))
	}
	if !b.app.Vault.Locked() || b.app.Vault.Status().Directory != destination {
		t.Fatal("restore should switch and lock")
	}
	for _, path := range []string{"/api/history?key=demo/deploy", "/api/history/entry?key=demo/deploy&revision=" + original.Revision, "/api/settings", "/api/entries?scope=body&q=PRIVATE"} {
		code, _ = b.request("GET", path, nil, "")
		if code != 401 {
			t.Fatal("locked route", path, code)
		}
	}
}
func TestMCPBearerCannotAccessHumanRecoveryOrSearch(t *testing.T) {
	b := setupHTTP(t)
	original := b.seed()
	_, token, err := b.app.clients.Add("mcp only")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/history?key=demo/deploy", "/api/history/entry?key=demo/deploy&revision=" + original.Revision, "/api/entries?scope=body&q=PRIVATE", "/api/settings"} {
		request, err := http.NewRequest("GET", b.url+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 401 {
			t.Fatal(path, response.StatusCode)
		}
	}
	code, body := b.request("GET", "/api/entries?scope=body&q=PRIVATE", nil, "")
	if code != 200 {
		t.Fatal(code)
	}
	var hits []vault.Summary
	if err = json.Unmarshal(body, &hits); err != nil || len(hits) != 1 {
		t.Fatal(hits, err)
	}
}

func TestIdleLockIgnoresPollingAndInvalidatesApproval(t *testing.T) {
	b := setupHTTP(t)
	b.seed()
	clock := time.Now()
	b.app.idle.mu.Lock()
	b.app.idle.now = func() time.Time { return clock }
	b.app.idle.mu.Unlock()
	code, _ := b.request("POST", "/api/settings", DeviceSettings{IdleLockMinutes: 1}, b.csrf)
	if code != 200 {
		t.Fatal(code)
	}
	pending, err := b.app.Access.Read("test", "test", []vault.ReadSpec{{Key: "demo/deploy", IncludeValue: true}})
	if err != nil {
		t.Fatal(err)
	}
	b.app.idle.mu.Lock()
	clock = clock.Add(45 * time.Second)
	b.app.idle.mu.Unlock()
	for _, route := range []string{"/api/state", "/api/entries", "/api/approvals"} {
		code, _ = b.request("GET", route, nil, "")
		if code != 200 {
			t.Fatal(code)
		}
	}
	b.app.idle.mu.Lock()
	clock = clock.Add(16 * time.Second)
	b.app.idle.mu.Unlock()
	code, _ = b.request("GET", "/api/entries", nil, "")
	if code != 401 || !b.app.Vault.Locked() {
		t.Fatal("poll kept vault unlocked")
	}
	if err = b.app.Access.Decide(pending.RequestID, true); err == nil {
		t.Fatal("approval survived idle lock")
	}
	saved, err := loadIdle(b.app.deviceDir)
	if err != nil || saved.settings.IdleLockMinutes != 1 {
		t.Fatal(saved, err)
	}
}
func TestHumanActivityExtendsIdleDeadline(t *testing.T) {
	b := setupHTTP(t)
	clock := time.Now()
	b.app.idle.mu.Lock()
	b.app.idle.now = func() time.Time { return clock }
	b.app.idle.mu.Unlock()
	code, _ := b.request("POST", "/api/settings", DeviceSettings{IdleLockMinutes: 1}, b.csrf)
	if code != 200 {
		t.Fatal(code)
	}
	b.app.idle.mu.Lock()
	clock = clock.Add(45 * time.Second)
	b.app.idle.mu.Unlock()
	code, _ = b.request("POST", "/api/activity", map[string]any{}, b.csrf)
	if code != 200 {
		t.Fatal(code)
	}
	b.app.idle.mu.Lock()
	clock = clock.Add(45 * time.Second)
	b.app.idle.mu.Unlock()
	code, _ = b.request("GET", "/api/entries", nil, "")
	if code != 200 {
		t.Fatal("activity ignored", code)
	}
	code, _ = b.request("POST", "/api/settings", DeviceSettings{IdleLockMinutes: -1}, b.csrf)
	if code != 400 {
		t.Fatal(code)
	}
}
