package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/shenjiecode/keyfolio/internal/platform"
	"github.com/shenjiecode/keyfolio/internal/vault"
)

type memoryCredentials struct {
	mu                           sync.Mutex
	values                       map[string][]byte
	failGet, failSet, failDelete bool
	reads                        int
	onSet                        func()
}

func (c *memoryCredentials) Get(_ context.Context, id string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reads++
	if c.failGet {
		return nil, platform.ErrUnavailable
	}
	raw, ok := c.values[id]
	if !ok {
		return nil, platform.ErrNotFound
	}
	return bytes.Clone(raw), nil
}
func (c *memoryCredentials) Set(_ context.Context, id string, raw []byte) error {
	if c.onSet != nil {
		c.onSet()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failSet {
		return platform.ErrUnavailable
	}
	c.values[id] = bytes.Clone(raw)
	return nil
}
func (c *memoryCredentials) Delete(_ context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failDelete {
		return platform.ErrUnavailable
	}
	delete(c.values, id)
	return nil
}

type memoryStartup struct {
	mu      sync.Mutex
	enabled bool
}

func (s *memoryStartup) Status() (platform.StartupStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return platform.StartupStatus{Supported: true, Enabled: s.enabled, Current: true}, nil
}
func (s *memoryStartup) Set(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = enabled
	return nil
}
func setupDeviceHTTP(t *testing.T) (*testBrowser, *memoryCredentials) {
	b := setupHTTP(t)
	credentials := &memoryCredentials{values: map[string][]byte{}}
	b.app.device.credentials = credentials
	b.app.device.startup = &memoryStartup{}
	return b, credentials
}
func enrollBrowser(b *testBrowser, remember bool) {
	b.t.Helper()
	code, data := b.request("POST", "/api/device/unlock", map[string]any{"enabled": true, "password": "synthetic-test-password", "remember_browser": remember}, b.csrf)
	if code != 200 {
		b.t.Fatalf("enroll %d %s", code, data)
	}
}
func openBrowser(t *testing.T, app *Server, address string, jar http.CookieJar) *testBrowser {
	t.Helper()
	if jar == nil {
		jar, _ = cookiejar.New(nil)
	}
	b := &testBrowser{t: t, app: app, http: &http.Client{Jar: jar, Timeout: 10 * time.Second}, url: address}
	resp, err := b.http.Get(address)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	code, data := b.request("GET", "/api/state", nil, "")
	if code != 200 {
		t.Fatal(code, string(data))
	}
	var state struct{ CSRF string }
	if err = json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	b.csrf = state.CSRF
	return b
}
func restartDeviceBrowser(t *testing.T, original *testBrowser, c *memoryCredentials) *testBrowser {
	t.Helper()
	original.app.Access.Lock()
	ts := httptest.NewUnstartedServer(nil)
	app, err := New(vault.New(original.app.Vault.Status().Directory), original.app.deviceDir, "http://"+ts.Listener.Addr().String(), original.app.static)
	if err != nil {
		t.Fatal(err)
	}
	app.device.credentials = c
	app.device.startup = &memoryStartup{}
	app.SystemEvent(platform.SystemEvent{Available: true})
	app.TryAutoUnlock(context.Background())
	ts.Config.Handler = app
	ts.Start()
	t.Cleanup(ts.Close)
	return openBrowser(t, app, ts.URL, original.http.Jar)
}
func TestDeviceUnlockAndTrustedBrowserIsolation(t *testing.T) {
	for _, remember := range []bool{false, true} {
		t.Run(map[bool]string{false: "service-only", true: "remember-browser"}[remember], func(t *testing.T) {
			b, credentials := setupDeviceHTTP(t)
			b.seed()
			enrollBrowser(b, remember)
			raw, err := os.ReadFile(filepath.Join(b.app.deviceDir, "device.json"))
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(raw, []byte("synthetic-test-password")) {
				t.Fatal("persisted master password")
			}
			credentials.mu.Lock()
			for _, value := range credentials.values {
				var secret struct{ Key string }
				if err = json.Unmarshal(value, &secret); err != nil {
					t.Fatal(err)
				}
				if secret.Key == "" || bytes.Contains(raw, []byte(secret.Key)) {
					t.Fatal("device key stored outside OS provider")
				}
			}
			credentials.mu.Unlock()
			u, _ := url.Parse(b.url)
			for _, cookie := range b.http.Jar.Cookies(u) {
				if cookie.Name == b.app.trustCookieName() && bytes.Contains(raw, []byte(cookie.Value)) {
					t.Fatal("plaintext browser token persisted in device settings")
				}
			}
			b = restartDeviceBrowser(t, b, credentials)
			if b.app.Vault.Locked() {
				t.Fatal("startup unlock failed", b.app.deviceNotice())
			}
			code, _ := b.request("GET", "/api/entries", nil, "")
			if (code == 200) != remember {
				t.Fatal("unexpected browser authentication", remember, code)
			}
			stranger := openBrowser(t, b.app, b.url, nil)
			if code, _ = stranger.request("GET", "/api/entries", nil, ""); code != 401 {
				t.Fatal("new browser inherited unlock", code)
			}
			if code, _ = stranger.request("POST", "/api/device/unlock", map[string]bool{"enabled": false}, stranger.csrf); code != 401 {
				t.Fatal("unauthenticated device settings", code)
			}
			if remember {
				if code, _ = b.request("POST", "/api/vault/lock", map[string]bool{}, b.csrf); code != 200 {
					t.Fatal(code)
				}
				b.app.TryAutoUnlock(context.Background())
				if !b.app.Vault.Locked() {
					t.Fatal("manual lock undone by startup retry")
				}
				if code, _ = b.request("GET", "/api/entries", nil, ""); code != 401 {
					t.Fatal("remembered browser bypassed manual lock", code)
				}
			}
		})
	}
}
func TestDeviceEnrollmentRequiresPasswordAndCSRF(t *testing.T) {
	b, c := setupDeviceHTTP(t)
	for _, test := range []struct {
		password, csrf string
		code           int
	}{{"wrong", b.csrf, 400}, {"synthetic-test-password", "wrong-csrf", 403}} {
		code, _ := b.request("POST", "/api/device/unlock", map[string]any{"enabled": true, "password": test.password}, test.csrf)
		if code != test.code {
			t.Fatal(code)
		}
	}
	if len(c.values) != 0 || b.app.device.record.Enabled {
		t.Fatal("unapproved enrollment created a credential")
	}
	enrollBrowser(b, true)
	_, data := b.request("GET", "/api/device", nil, "")
	for _, forbidden := range []string{"credential_id", "browser_hash", "config_hash", "synthetic-test-password"} {
		if bytes.Contains(data, []byte(forbidden)) {
			t.Fatal("device status leaked internal credential material")
		}
	}
}
func TestDeviceUnavailableAndConfigMismatchStayLocked(t *testing.T) {
	for _, cause := range []string{"unavailable", "config-changed", "directory-changed"} {
		t.Run(cause, func(t *testing.T) {
			b, c := setupDeviceHTTP(t)
			enrollBrowser(b, true)
			switch cause {
			case "unavailable":
				c.failGet = true
			case "config-changed":
				if err := b.app.Vault.ChangePassword("synthetic-test-password", "synthetic-next-password"); err != nil {
					t.Fatal(err)
				}
			case "directory-changed":
				if err := b.app.Vault.SelectDirectory(t.TempDir()); err != nil {
					t.Fatal(err)
				}
			}
			b = restartDeviceBrowser(t, b, c)
			if !b.app.Vault.Locked() || b.app.deviceNotice() == "" {
				t.Fatal("failed startup did not fail closed")
			}
			if code, _ := b.request("GET", "/api/entries", nil, ""); code != 401 {
				t.Fatal(code)
			}
			if cause == "unavailable" {
				code, raw := b.request("POST", "/api/vault/unlock", map[string]string{"password": "synthetic-test-password"}, b.csrf)
				if code != 200 {
					t.Fatal("manual fallback failed", code, string(raw))
				}
			}
		})
	}
}
func TestDeviceDisablePersistsBeforeUnavailableCleanup(t *testing.T) {
	b, c := setupDeviceHTTP(t)
	enrollBrowser(b, true)
	c.failDelete = true
	code, raw := b.request("POST", "/api/device/unlock", map[string]bool{"enabled": false}, b.csrf)
	if code != 200 || !bytes.Contains(raw, []byte("尚未删除")) {
		t.Fatal(code, string(raw))
	}
	if b.app.device.record.Enabled || b.app.device.record.BrowserHash != "" || len(c.values) != 1 {
		t.Fatal("unsafe disable")
	}
	beforeReads := c.reads
	b = restartDeviceBrowser(t, b, c)
	if !b.app.Vault.Locked() || c.reads != beforeReads {
		t.Fatal("disabled credential queried at restart")
	}
	code, _ = b.request("POST", "/api/vault/unlock", map[string]string{"password": "synthetic-test-password"}, b.csrf)
	if code != 200 {
		t.Fatal(code)
	}
	c.failDelete = false
	code, raw = b.request("POST", "/api/device/unlock", map[string]bool{"enabled": false}, b.csrf)
	if code != 200 || len(c.values) != 0 || b.app.device.record.CredentialID != "" {
		t.Fatal(code, string(raw))
	}
}
func TestPasswordHTTPRevokesApprovalsAndDevice(t *testing.T) {
	b, c := setupDeviceHTTP(t)
	b.seed()
	enrollBrowser(b, true)
	pending, err := b.app.Access.Read("synthetic-client", "Synthetic", []vault.ReadSpec{{Key: "demo/deploy", IncludeValue: true}})
	if err != nil || pending.State != "pending" {
		t.Fatal(pending, err)
	}
	code, raw := b.request("POST", "/api/vault/password", map[string]string{"current_password": "wrong-password", "new_password": "synthetic-next-password"}, b.csrf)
	if code != 400 || !b.app.device.record.Enabled {
		t.Fatal("invalid password disabled device", code, string(raw))
	}
	code, raw = b.request("POST", "/api/vault/password", map[string]string{"current_password": "synthetic-test-password", "new_password": "synthetic-next-password"}, b.csrf)
	if code != 200 {
		t.Fatal(code, string(raw))
	}
	if !b.app.Vault.Locked() || len(c.values) != 0 || b.app.device.record.Enabled {
		t.Fatal("password change did not revoke device/session")
	}
	if err = b.app.Access.Decide(pending.RequestID, true); err == nil {
		t.Fatal("old approval accepted after password change")
	}
	if code, _ = b.request("GET", "/api/entries", nil, ""); code != 401 {
		t.Fatal(code)
	}
	for password, want := range map[string]int{"synthetic-test-password": 400, "synthetic-next-password": 200} {
		code, raw = b.request("POST", "/api/vault/unlock", map[string]string{"password": password}, b.csrf)
		if code != want {
			t.Fatal(code, string(raw))
		}
	}
}
func TestDeviceEnrollmentInterruptedByLock(t *testing.T) {
	b, c := setupDeviceHTTP(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	c.onSet = func() { close(entered); <-release }
	result := make(chan int, 1)
	go func() {
		code, _ := b.request("POST", "/api/device/unlock", map[string]any{"enabled": true, "password": "synthetic-test-password", "remember_browser": true}, b.csrf)
		result <- code
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("enrollment did not reach OS store")
	}
	// Lock is deliberately not serialized behind maintenance/slow keyring calls.
	code, _ := b.request("POST", "/api/vault/lock", map[string]any{}, b.csrf)
	close(release)
	if code != 200 {
		t.Fatal(code)
	}
	if code = <-result; code != 409 {
		t.Fatal("stale enrollment accepted", code)
	}
	if !b.app.Vault.Locked() || b.app.device.record.Enabled || len(c.values) != 0 {
		t.Fatal("enrollment survived lock")
	}
}
func TestDeviceStartupAndIdleLockRemainIndependent(t *testing.T) {
	b, c := setupDeviceHTTP(t)
	if code, _ := b.request("POST", "/api/device/startup", map[string]bool{"enabled": true}, b.csrf); code != 200 {
		t.Fatal(code)
	}
	if b.app.device.record.Enabled || len(c.values) != 0 {
		t.Fatal("autostart enabled auto-unlock")
	}
	raw, err := os.ReadFile(filepath.Join(b.app.deviceDir, "location.json"))
	if err != nil || !bytes.Contains(raw, []byte(b.app.Vault.Status().Directory)) {
		t.Fatal("startup did not persist active vault", err)
	}
	enrollBrowser(b, true)
	b = restartDeviceBrowser(t, b, c)
	b.app.idle.mu.Lock()
	b.app.idle.settings.IdleLockMinutes = 1
	b.app.idle.lastActivity = time.Now().Add(-2 * time.Minute)
	b.app.idle.mu.Unlock()
	if code, _ := b.request("GET", "/api/entries", nil, ""); code != 401 {
		t.Fatal("trusted cookie bypassed idle lock", code)
	}
	b.app.TryAutoUnlock(context.Background())
	if !b.app.Vault.Locked() {
		t.Fatal("idle lock auto-reopened")
	}
}
func TestCannotPutDeviceDirectoryInsideVault(t *testing.T) {
	b, _ := setupDeviceHTTP(t)
	if code, _ := b.request("POST", "/api/vault/select", map[string]string{"directory": b.app.deviceDir}, b.csrf); code != 409 {
		t.Fatal("device files could be synced with vault", code)
	}
	_, err := New(vault.New(b.app.deviceDir), filepath.Join(b.app.deviceDir, "nested"), b.url, b.app.static)
	if err == nil {
		t.Fatal("unsafe startup directory accepted")
	}
}
func TestFailedDeviceSetLeavesCleanupMarker(t *testing.T) {
	b, c := setupDeviceHTTP(t)
	c.failSet = true
	code, _ := b.request("POST", "/api/device/unlock", map[string]any{"enabled": true, "password": "synthetic-test-password"}, b.csrf)
	if code != 400 || b.app.device.record.Enabled || b.app.device.record.CredentialID == "" {
		t.Fatal("ambiguous provider failure lost cleanup marker", code)
	}
	b = restartDeviceBrowser(t, b, c)
	if !b.app.Vault.Locked() {
		t.Fatal("failed enrollment unlocked vault")
	}
}
func TestEnrollmentPersistenceFailureDoesNotReportDisabled(t *testing.T) {
	b, _ := setupDeviceHTTP(t)
	enrollBrowser(b, true)
	path := filepath.Join(b.app.deviceDir, "device.json")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	code, _ := b.request("POST", "/api/device/unlock", map[string]bool{"enabled": false}, b.csrf)
	if code != 500 || !b.app.device.record.Enabled {
		t.Fatal("failed disable was reported as saved", code)
	}
}
func TestTrustedBrowserExpiryAndTamperedCookie(t *testing.T) {
	b, c := setupDeviceHTTP(t)
	enrollBrowser(b, true)
	b = restartDeviceBrowser(t, b, c)
	original := b.app.device.record.BrowserExpires
	for _, cause := range []string{"expired", "tampered"} {
		t.Run(cause, func(t *testing.T) {
			b.app.mu.Lock()
			b.app.sessions = map[string]*webSession{}
			b.app.mu.Unlock()
			b.app.device.mu.Lock()
			b.app.device.record.BrowserExpires = original
			if cause == "expired" {
				b.app.device.record.BrowserExpires = time.Now().Add(-time.Second)
			}
			b.app.device.mu.Unlock()
			if cause == "tampered" {
				u, _ := url.Parse(b.url)
				b.http.Jar.SetCookies(u, []*http.Cookie{{Name: b.app.trustCookieName(), Value: "000000000000000000000000000000000000000000000000", Path: "/"}})
			}
			b = openBrowser(t, b.app, b.url, b.http.Jar)
			if code, _ := b.request("GET", "/api/entries", nil, ""); code != 401 {
				t.Fatal("invalid remembered browser accepted", code)
			}
		})
	}
}
