package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/shenjiecode/keyfolio/internal/platform"
	"github.com/shenjiecode/keyfolio/internal/vault"
)

func setupSelectionHTTP(t *testing.T, root, device string) *testBrowser {
	t.Helper()
	ts := httptest.NewUnstartedServer(nil)
	app, err := New(vault.New(root), device, "http://"+ts.Listener.Addr().String(), fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("synthetic")}})
	if err != nil {
		t.Fatal(err)
	}
	ts.Config.Handler = app
	ts.Start()
	t.Cleanup(ts.Close)
	jar, _ := cookiejar.New(nil)
	b := &testBrowser{t: t, app: app, http: &http.Client{Jar: jar, Timeout: 5 * time.Second}, url: ts.URL}
	_, data := b.request("GET", "/api/state", nil, "")
	var state struct{ CSRF string }
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	b.csrf = state.CSRF
	return b
}
func TestFirstUseCreationAndMissingLocation(t *testing.T) {
	device := t.TempDir()
	b := setupSelectionHTTP(t, "", device)
	code, data := b.request("GET", "/api/state", nil, "")
	var state struct {
		Vault         vault.Status
		Authenticated bool
	}
	if code != 200 || json.Unmarshal(data, &state) != nil || state.Vault.Directory != "" || state.Vault.Exists || state.Authenticated {
		t.Fatalf("initial %d %s", code, data)
	}
	if _, err := os.Stat(filepath.Join(device, "vault")); !os.IsNotExist(err) {
		t.Fatal("invented default")
	}
	empty := t.TempDir()
	code, _ = b.request("POST", "/api/vault/select", map[string]string{"directory": empty}, b.csrf)
	if code != 400 || b.app.Vault.Status().Directory != "" {
		t.Fatal("opening empty directory changed selection", code)
	}
	root := filepath.Join(t.TempDir(), "created")
	for _, input := range []map[string]string{
		{"password": "synthetic-test-password"},
		{"directory": root, "password": "short"},
		{"directory": device, "password": "synthetic-test-password"},
	} {
		if code, _ := b.request("POST", "/api/vault/create", input, b.csrf); code != 400 {
			t.Fatal("invalid create", code)
		}
		if b.app.Vault.Status().Directory != "" {
			t.Fatal("failed create changed selection")
		}
	}
	code, data = b.request("POST", "/api/vault/create", map[string]string{"directory": root, "password": "synthetic-test-password"}, b.csrf)
	if code != 200 || b.app.Vault.Status().Directory != root {
		t.Fatalf("create %d %s", code, data)
	}
	_, data = b.request("GET", "/api/state", nil, "")
	_ = json.Unmarshal(data, &state)
	if !state.Authenticated || !state.Vault.Exists {
		t.Fatal("created vault not authenticated", string(data))
	}
	// Removing the entire selected vault must not turn it into first-use or allow
	// an initialization at that remembered location, including after restart.
	b.app.Access.Lock()
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	code, _ = b.request("POST", "/api/vault/create", map[string]string{"directory": root, "password": "synthetic-test-password"}, b.csrf)
	if code != 400 {
		t.Fatal("recreated missing selected vault", code)
	}
	restarted := setupSelectionHTTP(t, root, device)
	if got := restarted.app.Vault.Status(); got.Directory != root || got.Exists {
		t.Fatal(got)
	}
	code, _ = restarted.request("POST", "/api/vault/create", map[string]string{"directory": root, "password": "synthetic-test-password"}, restarted.csrf)
	if code != 400 {
		t.Fatal("recreated missing remembered vault", code)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("missing vault recreated")
	}
}
func TestExplicitNewDirectoryCanBeCreatedButCannotSwitchAfterBinding(t *testing.T) {
	root := filepath.Join(t.TempDir(), "explicit-new")
	b := setupSelectionHTTP(t, root, t.TempDir())
	code, data := b.request("POST", "/api/vault/create", map[string]string{"directory": root, "password": "synthetic-test-password"}, b.csrf)
	if code != 200 {
		t.Fatalf("explicit create %d %s", code, data)
	}
	other := t.TempDir()
	v := vault.New(other)
	if err := v.Init("synthetic-other-password"); err != nil {
		t.Fatal(err)
	}
	v.Lock()
	code, data = b.request("POST", "/api/vault/select", map[string]string{"directory": other}, b.csrf)
	if code != 409 || b.app.Vault.Status().Directory != root || b.app.Vault.Locked() {
		t.Fatalf("switch should be denied %d %s", code, data)
	}
}
func TestCreationAndSelectionCannotHijackAnotherBrowser(t *testing.T) {
	b := setupHTTP(t)
	jar, _ := cookiejar.New(nil)
	other := &testBrowser{t: t, app: b.app, http: &http.Client{Jar: jar}, url: b.url}
	_, data := other.request("GET", "/api/state", nil, "")
	var state struct{ CSRF string }
	_ = json.Unmarshal(data, &state)
	other.csrf = state.CSRF
	target := filepath.Join(t.TempDir(), "new")
	for _, path := range []string{"/api/vault/create", "/api/vault/select", "/api/directory/pick"} {
		code, _ := other.request("POST", path, map[string]string{"directory": target, "password": "synthetic-test-password"}, other.csrf)
		if code != 401 {
			t.Fatalf("other browser %s: %d", path, code)
		}
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("unauthorized creation")
	}
}
func TestDirectoryPickerSecurityCancellationAndNoSelectionSideEffects(t *testing.T) {
	b := setupSelectionHTTP(t, "", t.TempDir())
	target := t.TempDir()
	calls := 0
	b.app.chooseDirectory = func(_ context.Context, initial string) (string, error) {
		calls++
		if initial != "" {
			t.Fatal(initial)
		}
		return target, nil
	}
	for _, tc := range []struct {
		method, csrf string
		want         int
	}{{"GET", "", 405}, {"POST", "wrong", 403}} {
		code, _ := b.request(tc.method, "/api/directory/pick", map[string]string{"directory": ""}, tc.csrf)
		if code != tc.want {
			t.Fatalf("method/CSRF: %d", code)
		}
	}
	// Bearer alone is never a human page session.
	req, _ := http.NewRequest("POST", b.url+"/api/directory/pick", strings.NewReader(`{"directory":""}`))
	req.Header.Set("Authorization", "Bearer synthetic-token")
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 401 || calls != 0 {
		t.Fatal("picker reached without page protection")
	}
	for name, value := range map[string]string{"Origin": "https://evil.example", "Sec-Fetch-Site": "cross-site"} {
		req, _ := http.NewRequest("POST", b.url+"/api/directory/pick", strings.NewReader(`{"directory":""}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-FolioChest-CSRF", b.csrf)
		req.Header.Set(name, value)
		res, err := b.http.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 403 {
			t.Fatal(name, res.StatusCode)
		}
	}
	code, data := b.request("POST", "/api/directory/pick", map[string]string{"directory": ""}, b.csrf)
	var selection struct {
		Directory string
		Cancelled bool
	}
	_ = json.Unmarshal(data, &selection)
	if code != 200 || selection.Directory != target || selection.Cancelled || calls != 1 {
		t.Fatalf("pick %d %s", code, data)
	}
	if b.app.Vault.Status().Directory != "" {
		t.Fatal("picker switched vault")
	}
	if _, err := os.Stat(filepath.Join(b.app.deviceDir, "location.json")); !os.IsNotExist(err) {
		t.Fatal("picker persisted location")
	}
	b.app.chooseDirectory = func(context.Context, string) (string, error) { return "", platform.ErrDirectoryCanceled }
	code, data = b.request("POST", "/api/directory/pick", map[string]string{}, b.csrf)
	if code != 200 || !strings.Contains(string(data), `"cancelled":true`) {
		t.Fatalf("cancel %d %s", code, data)
	}
	b.app.chooseDirectory = func(context.Context, string) (string, error) { return "", platform.ErrDirectoryUnavailable }
	if code, _ := b.request("POST", "/api/directory/pick", map[string]string{}, b.csrf); code != 503 {
		t.Fatal(code)
	}
}
func TestDirectoryPickerSingletonAndLockInvalidation(t *testing.T) {
	b := setupHTTP(t)
	entered := make(chan struct{})
	done := make(chan int, 1)
	b.app.chooseDirectory = func(ctx context.Context, _ string) (string, error) {
		close(entered)
		<-ctx.Done()
		return "/synthetic/late", nil
	}
	go func() { code, _ := b.request("POST", "/api/directory/pick", map[string]string{}, b.csrf); done <- code }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("picker not entered")
	}
	if code, _ := b.request("POST", "/api/directory/pick", map[string]string{}, b.csrf); code != 409 {
		t.Fatal("duplicate picker", code)
	}
	if code, _ := b.request("POST", "/api/vault/lock", map[string]string{}, b.csrf); code != 200 {
		t.Fatal("picker blocked lock", code)
	}
	select {
	case code := <-done:
		if code != 409 {
			t.Fatal("late path accepted", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("lock did not close picker")
	}
}
