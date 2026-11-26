package server

import (
	"context"
	"encoding/json"
	"github.com/shenjiecode/keyfolio/internal/platform"
	"github.com/shenjiecode/keyfolio/internal/vault"
	"strings"
	"testing"
	"time"
)

func TestSystemLockRevokesBrowsersAndApprovedContent(t *testing.T) {
	for _, event := range []platform.SystemEvent{
		{Available: true, Locked: true},
		{Available: true, Sleeping: true, Lock: true},
		{Available: true, Lock: true, Reason: "synthetic resume"},
		{Available: false},
	} {
		t.Run(event.Reason+map[bool]string{true: "known", false: "lost"}[event.Available], func(t *testing.T) {
			b, _ := setupDeviceHTTP(t)
			entry := b.seed()
			enrollBrowser(b, true)
			b.app.SystemEvent(platform.SystemEvent{Available: true})
			request, err := b.app.Access.Read("test", "Synthetic", []vault.ReadSpec{{Key: entry.Key, IncludeValue: true}})
			if err != nil {
				t.Fatal(err)
			}
			if err = b.app.Access.Decide(request.RequestID, true); err != nil {
				t.Fatal(err)
			}
			b.app.SystemEvent(event)
			if !b.app.Vault.Locked() {
				t.Fatal("system event did not lock")
			}
			if result, err := b.app.Access.Result("test", request.RequestID); err == nil || len(result.Reads) != 0 {
				t.Fatal("buffer survived", result, err)
			}
			code, data := b.request("GET", "/api/state", nil, "")
			if code != 200 || strings.Contains(string(data), `"authenticated":true`) {
				t.Fatal(code, string(data))
			}
			if event.Locked || event.Sleeping {
				code, _ = b.request("POST", "/api/vault/unlock", map[string]string{"password": "synthetic-test-password"}, b.csrf)
				if code == 200 {
					t.Fatal("password bypassed known OS lock")
				}
				b.app.SystemEvent(platform.SystemEvent{}) // lost provider must retain known lock
				code, _ = b.request("POST", "/api/vault/unlock", map[string]string{"password": "synthetic-test-password"}, b.csrf)
				if code == 200 {
					t.Fatal("monitor failure cleared known locked state")
				}
			}
			b.app.SystemEvent(platform.SystemEvent{Available: true})
			b.app.TryAutoUnlock(context.Background())
			if !b.app.Vault.Locked() {
				t.Fatal("OS unlock re-opened vault")
			}
			code, _ = b.request("POST", "/api/vault/unlock", map[string]string{"password": "synthetic-test-password"}, b.csrf)
			if code != 200 {
				t.Fatal("explicit password could not unlock after OS unlock", code)
			}
		})
	}
}
func TestSystemUnlockFenceAndUnknownFallback(t *testing.T) {
	b := setupHTTP(t)
	b.app.SystemEvent(platform.SystemEvent{Available: true})
	sequence, err := b.app.beginUnlock(false)
	if err != nil {
		t.Fatal(err)
	}
	b.app.SystemEvent(platform.SystemEvent{Available: true, Lock: true})
	// Simulate a slow KDF completing after the event was received.
	if err = b.app.Vault.Unlock("synthetic-test-password"); err != nil {
		t.Fatal(err)
	}
	if err = b.app.finishUnlock(sequence); err == nil || !b.app.Vault.Locked() {
		t.Fatal("unlock crossed an event")
	}
	b.app.SystemEvent(platform.SystemEvent{})
	if err = b.app.unlockPassword("synthetic-test-password"); err != nil {
		t.Fatal("unknown state should permit explicit password with warning", err)
	}
	b.app.SystemEvent(platform.SystemEvent{})
	if b.app.Vault.Locked() {
		t.Fatal("permanent unavailable provider repeatedly locked manual session")
	}
}
func TestSystemHeartbeatBeforeRequestAndClockDiscontinuity(t *testing.T) {
	for _, offset := range []time.Duration{20 * time.Second, -10 * time.Second} {
		t.Run(offset.String(), func(t *testing.T) {
			b := setupHTTP(t)
			b.seed()
			now := time.Now()
			b.app.system.mu.Lock()
			b.app.system.heartbeat = now
			b.app.system.now = func() time.Time { return now.Add(offset) }
			b.app.system.mu.Unlock()
			code, data := b.request("GET", "/api/entry?key=demo%2Fdeploy", nil, "")
			if code != 401 || strings.Contains(string(data), "SYNTHETIC_PRIVATE_VALUE") || !b.app.Vault.Locked() {
				t.Fatal("first post-resume request leaked", code, string(data))
			}
		})
	}
}
func TestSystemPolicyDefaultsPersistenceAndOptOut(t *testing.T) {
	b := setupHTTP(t)
	code, data := b.request("GET", "/api/settings", nil, "")
	var settings DeviceSettings
	_ = json.Unmarshal(data, &settings)
	if code != 200 || !settings.LockOnSystemEvents || settings.IdleLockMinutes != 0 {
		t.Fatal("wrong defaults", string(data))
	}
	code, _ = b.request("POST", "/api/settings", map[string]any{"idle_lock_minutes": 3}, b.csrf)
	if code != 200 || !b.app.systemProtected() {
		t.Fatal("old client accidentally disabled system locking")
	}
	code, _ = b.request("POST", "/api/settings", map[string]any{"idle_lock_minutes": 0, "lock_on_system_events": false}, b.csrf)
	if code != 200 {
		t.Fatal(code)
	}
	state, err := loadIdle(b.app.deviceDir)
	if err != nil || state.settings.LockOnSystemEvents {
		t.Fatal(err)
	}
	b.app.SystemEvent(platform.SystemEvent{Available: true, Locked: true, Lock: true})
	if b.app.Vault.Locked() {
		t.Fatal("opt-out not honored")
	}
	b.app.system.mu.Lock()
	b.app.system.heartbeat = time.Now().Add(-time.Minute)
	b.app.system.mu.Unlock()
	b.app.SystemHeartbeat()
	if b.app.Vault.Locked() {
		t.Fatal("disabled heartbeat still locked")
	}
	// Enabling the setting while a known OS lock is present closes the session.
	code, _ = b.request("POST", "/api/settings", map[string]any{"idle_lock_minutes": 0, "lock_on_system_events": true}, b.csrf)
	if code != 200 || !b.app.Vault.Locked() {
		t.Fatal("enabling protection ignored known lock", code)
	}
}
func TestAutoUnlockWithExplicitSystemPolicyOptOut(t *testing.T) {
	b, _ := setupDeviceHTTP(t)
	enrollBrowser(b, false)
	code, _ := b.request("POST", "/api/settings", map[string]any{"idle_lock_minutes": 0, "lock_on_system_events": false}, b.csrf)
	if code != 200 {
		t.Fatal(code)
	}
	b.app.Access.Lock()
	b.app.TryAutoUnlock(context.Background())
	if b.app.Vault.Locked() {
		t.Fatal("explicit opt-out did not allow credential unlock", b.app.deviceNotice())
	}
}
func TestAutoUnlockRequiresReadySession(t *testing.T) {
	b, credentials := setupDeviceHTTP(t)
	enrollBrowser(b, false)
	b.app.Access.Lock()
	b.app.TryAutoUnlock(context.Background())
	if !b.app.Vault.Locked() {
		t.Fatal("auto-unlocked without an OS observation")
	}
	if !strings.Contains(b.app.deviceNotice(), "系统会话") {
		t.Fatal(b.app.deviceNotice())
	}
	// No retry, even when a provider subsequently becomes available.
	b.app.SystemEvent(platform.SystemEvent{Available: true})
	b.app.TryAutoUnlock(context.Background())
	if !b.app.Vault.Locked() {
		t.Fatal("auto-unlock retried")
	}
	// The same enrollment still works on a fresh, ready instance (the helper
	// injects a safe OS observation before its one startup attempt).
	fresh := restartDeviceBrowser(t, b, credentials)
	if fresh.app.Vault.Locked() {
		t.Fatal(fresh.app.deviceNotice())
	}
}
