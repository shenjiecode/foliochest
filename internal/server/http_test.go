package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shenjiecode/keyfolio/internal/access"
	"github.com/shenjiecode/keyfolio/internal/vault"
)

type testBrowser struct {
	t         *testing.T
	app       *Server
	http      *http.Client
	url, csrf string
}

func setupHTTP(t *testing.T) *testBrowser {
	t.Helper()
	v := vault.New(t.TempDir())
	if err := v.Init("synthetic-test-password"); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewUnstartedServer(nil)
	baseURL := "http://" + ts.Listener.Addr().String()
	app, err := New(v, t.TempDir(), baseURL, fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html><body>Keyfolio</body></html>")}})
	if err != nil {
		t.Fatal(err)
	}
	ts.Config.Handler = app
	ts.Start()
	t.Cleanup(ts.Close)
	jar, _ := cookiejar.New(nil)
	browser := &testBrowser{t: t, app: app, http: &http.Client{Jar: jar, Timeout: 10 * time.Second}, url: ts.URL}
	resp, err := browser.http.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	_, data := browser.request("GET", "/api/state", nil, "")
	var state struct{ CSRF string }
	if err = json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	browser.csrf = state.CSRF
	code, data := browser.request("POST", "/api/vault/unlock", map[string]string{"password": "synthetic-test-password"}, browser.csrf)
	if code != 200 {
		t.Fatalf("unlock %d %s", code, data)
	}
	return browser
}
func (b *testBrowser) request(method, path string, body any, csrf string) (int, []byte) {
	b.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, b.url+path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Keyfolio-CSRF", csrf)
	}
	resp, err := b.http.Do(req)
	if err != nil {
		b.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		b.t.Fatal(err)
	}
	return resp.StatusCode, data
}
func (b *testBrowser) seed() vault.Summary {
	b.t.Helper()
	description := "Synthetic deployment notes"
	value := "SYNTHETIC_PRIVATE_VALUE"
	rows, err := b.app.Vault.Commit([]vault.Change{{Key: "demo/deploy", Description: &description, Value: &value}}, true, true)
	if err != nil {
		b.t.Fatal(err)
	}
	return rows[0]
}
func TestWebAuthenticationAndCSRF(t *testing.T) {
	b := setupHTTP(t)
	b.seed()
	code, _ := b.request("GET", "/api/entries", nil, "")
	if code != 200 {
		t.Fatal(code)
	}
	code, _ = b.request("POST", "/api/vault/lock", map[string]any{}, "wrong-csrf")
	if code != 403 {
		t.Fatal("CSRF bypass", code)
	}
	// A new browser must still provide the master password when another UI is unlocked.
	jar, _ := cookiejar.New(nil)
	other := &http.Client{Jar: jar}
	resp, err := other.Get(b.url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	resp, err = other.Get(b.url + "/api/entries")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatal("fresh browser inherited unlock", resp.StatusCode)
	}
	for _, header := range []string{"Origin", "Host"} {
		req, _ := http.NewRequest("GET", b.url+"/api/entries", nil)
		if header == "Origin" {
			req.Header.Set(header, "https://evil.example")
		} else {
			req.Host = "evil.example"
		}
		resp, err = b.http.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 403 {
			t.Fatal("origin/host bypass", header, resp.StatusCode)
		}
	}
	code, _ = b.request("POST", "/api/vault/lock", map[string]any{}, b.csrf)
	if code != 200 {
		t.Fatal(code)
	}
	code, _ = b.request("GET", "/api/entries", nil, "")
	if code != 401 {
		t.Fatal("data after lock", code)
	}
}
func TestNoPublicMCPOrApprovalEndpoint(t *testing.T) {
	b := setupHTTP(t)
	req, _ := http.NewRequest("POST", b.url+"/mcp", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatal(resp.StatusCode)
	}
	code, _ := b.request("POST", "/api/approvals/decide", map[string]any{"id": "fake", "accept": true}, "")
	if code != 403 {
		t.Fatal(code)
	}
}

type bearerTransport struct{ token string }

func (t bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	copyReq := req.Clone(req.Context())
	copyReq.Header = req.Header.Clone()
	copyReq.Header.Set("Authorization", "Bearer "+t.token)
	return http.DefaultTransport.RoundTrip(copyReq)
}
func toolOutput[T any](t *testing.T, result *mcp.CallToolResult) T {
	t.Helper()
	if result.IsError {
		t.Fatalf("tool error: %+v", result.Content)
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out T
	if err = json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode structured result: %v", err)
	}
	return out
}
func TestMCPDiscoveryApprovalReplaceAndRevoke(t *testing.T) {
	b := setupHTTP(t)
	original := b.seed()
	client, token, err := b.app.clients.Add("test harness")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "keyfolio-test", Version: "1"}, nil)
	session, err := mcpClient.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: b.url + "/mcp", HTTPClient: &http.Client{Transport: bearerTransport{token}}, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	t.Logf("negotiated protocol: %s", session.InitializeResult().ProtocolVersion)
	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 4 {
		t.Fatal(tools, err)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "discover_entries", Arguments: map[string]any{"query": "deployment"}})
	if err != nil {
		t.Fatal(err)
	}
	discovery := toolOutput[discoverResult](t, result)
	if len(discovery.Entries) != 1 || discovery.Entries[0].Description != "Synthetic deployment notes" {
		t.Fatal(discovery)
	}
	raw, _ := json.Marshal(result)
	if bytes.Contains(raw, []byte("SYNTHETIC_PRIVATE_VALUE")) {
		t.Fatal("discovery leaked secret")
	}
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "read_entries", Arguments: map[string]any{"entries": []map[string]any{{"key": "demo/deploy", "include_value": true}}}})
	if err != nil {
		t.Fatal(err)
	}
	pending := toolOutput[access.Result](t, result)
	if pending.State != "pending" || len(pending.Reads) != 0 {
		t.Fatal(pending)
	}
	// The human UI, not an MCP tool argument, approves this exact request.
	code, data := b.request("POST", "/api/approvals/decide", map[string]any{"id": pending.RequestID, "accept": true}, b.csrf)
	if code != 200 {
		t.Fatalf("approval %d %s", code, data)
	}
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "request_result", Arguments: map[string]any{"request_id": pending.RequestID}})
	if err != nil {
		t.Fatal(err)
	}
	read := toolOutput[access.Result](t, result)
	if len(read.Reads) != 1 || read.Reads[0].Value == nil || *read.Reads[0].Value != "SYNTHETIC_PRIVATE_VALUE" {
		t.Fatal("wrong read result")
	}
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "replace_entries", Arguments: map[string]any{"changes": []map[string]any{{"key": "demo/deploy", "expected_revision": original.Revision, "value": "SYNTHETIC_REPLACEMENT"}}}})
	if err != nil {
		t.Fatal(err)
	}
	pending = toolOutput[access.Result](t, result)
	if pending.State != "pending" {
		t.Fatal(pending)
	}
	code, _ = b.request("POST", "/api/approvals/decide", map[string]any{"id": pending.RequestID, "accept": true}, b.csrf)
	if code != 200 {
		t.Fatal(code)
	}
	entry, err := b.app.Vault.Get("demo/deploy")
	if err != nil || entry.Value != "SYNTHETIC_REPLACEMENT" {
		t.Fatal("replacement failed", err)
	}
	code, _ = b.request("POST", "/api/clients/revoke", map[string]string{"id": client.ID}, b.csrf)
	if code != 200 {
		t.Fatal(code)
	}
	if _, ok := b.app.clients.Authenticate(token); ok {
		t.Fatal("revoked token still authenticates")
	}
}
func TestMCPNativeElicitation(t *testing.T) {
	for _, accept := range []bool{true, false} {
		t.Run(map[bool]string{true: "accept", false: "decline"}[accept], func(t *testing.T) {
			b := setupHTTP(t)
			b.seed()
			_, token, err := b.app.clients.Add("interactive harness")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			prompted := false
			client := mcp.NewClient(&mcp.Implementation{Name: "interactive-test", Version: "1"}, &mcp.ClientOptions{
				ElicitationHandler: func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
					prompted = true
					if strings.Contains(req.Params.Message, "SYNTHETIC_PRIVATE_VALUE") {
						t.Error("read approval preview leaked the old value")
					}
					if !accept {
						return &mcp.ElicitResult{Action: "decline"}, nil
					}
					return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approve": true}}, nil
				},
			})
			session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: b.url + "/mcp", HTTPClient: &http.Client{Transport: bearerTransport{token}}, MaxRetries: -1}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "read_entries", Arguments: map[string]any{"entries": []map[string]any{{"key": "demo/deploy", "include_value": true}}}})
			if err != nil {
				t.Fatal(err)
			}
			out := toolOutput[access.Result](t, result)
			if !prompted {
				t.Fatal("native approval was not shown")
			}
			if accept && (out.State != "complete" || len(out.Reads) != 1) {
				t.Fatal("approved read not returned")
			}
			if !accept && (out.State != "denied" || len(out.Reads) != 0) {
				t.Fatal("declined read leaked data")
			}
		})
	}
}

func TestClientTokensPersistOnlyAsHashes(t *testing.T) {
	dir := t.TempDir()
	store, err := newClients(dir)
	if err != nil {
		t.Fatal(err)
	}
	client, token, err := store.Add("client")
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := newClients(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := reopened.Authenticate(token)
	if !ok || got.ID != client.ID {
		t.Fatal("client persistence failed")
	}
	for _, c := range reopened.All() {
		if c.TokenHash != "" {
			t.Fatal("hash exposed in UI")
		}
	}
}
