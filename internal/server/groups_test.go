package server

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shenjiecode/keyfolio/internal/vault"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGroupHTTPAuthenticationFilteringAndVersionPins(t *testing.T) {
	b := setupHTTP(t)
	first := b.seed()
	request := vault.GroupMove{Group: "项目 A", Entries: []vault.GroupMember{{Key: first.Key, ExpectedRevision: first.Revision}}}
	fresh := openBrowser(t, b.app, b.url, nil)
	if code, _ := fresh.request("GET", "/api/groups", nil, ""); code != 401 {
		t.Fatal(code)
	}
	if code, _ := fresh.request("POST", "/api/groups/move", request, fresh.csrf); code != 401 {
		t.Fatal(code)
	}
	if code, _ := b.request("POST", "/api/groups/move", request, "wrong"); code != 403 {
		t.Fatal(code)
	}
	if code, _ := b.request("GET", "/api/groups/move", nil, ""); code != 405 {
		t.Fatal(code)
	}
	// New branding uses a new CSRF header; old clients remain supported separately.
	raw, _ := json.Marshal(request)
	req, _ := http.NewRequest("POST", b.url+"/api/groups/move", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-FolioChest-CSRF", b.csrf)
	response, err := b.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
	if code, _ := b.request("POST", "/api/groups/move", request, b.csrf); code != 409 {
		t.Fatal("accepted old revision", code)
	}
	code, data := b.request("GET", "/api/entries?group=", nil, "")
	if code != 200 || string(data) != "[]\n" {
		t.Fatal(code, string(data))
	}
	code, data = b.request("GET", "/api/groups", nil, "")
	var groups []vault.GroupInfo
	if code != 200 || json.Unmarshal(data, &groups) != nil || len(groups) != 1 || groups[0].Name != "项目 A" {
		t.Fatal(code, string(data))
	}
	edit := vault.GroupEdit{Source: "项目 A", Target: "B", Expected: groups[0].Members}
	if code, _ = fresh.request("POST", "/api/groups/edit", edit, fresh.csrf); code != 401 {
		t.Fatal(code)
	}
	if code, _ = b.request("POST", "/api/groups/edit", edit, "wrong"); code != 403 {
		t.Fatal(code)
	}
	if code, data = b.request("POST", "/api/groups/edit", edit, b.csrf); code != 200 {
		t.Fatal(code, string(data))
	}
	current, err := b.app.Vault.Get(first.Key)
	if err != nil || current.Group != "B" || current.Value != "SYNTHETIC_PRIVATE_VALUE" {
		t.Fatal(current, err)
	}
}
func TestMCPGroupDiscoveryCannotManageGroups(t *testing.T) {
	b := setupHTTP(t)
	first := b.seed()
	g := "项目 A"
	allow := vault.Allow
	rows, err := b.app.Vault.Commit([]vault.Change{{Key: first.Key, ExpectedRevision: first.Revision, Group: &g, WritePolicy: &allow}}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	enableTestHTTP(b)
	_, token, err := b.app.clients.Add("synthetic group client")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "group-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: b.url + "/mcp", HTTPClient: &http.Client{Transport: bearerTransport{token}}, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if session.InitializeResult().ServerInfo.Name != "foliochest" {
		t.Fatal("legacy server brand")
	}
	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 5 {
		t.Fatal(tools, err)
	}
	for _, args := range []struct {
		group string
		count int
	}{{"项目 A", 1}, {"", 0}, {"项目", 0}} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "discover_entries", Arguments: map[string]any{"group": args.group}})
		if err != nil {
			t.Fatal(err)
		}
		out := toolOutput[discoverResult](t, result)
		if len(out.Entries) != args.count {
			t.Fatal(args, out)
		}
		raw, _ := json.Marshal(result)
		if bytes.Contains(raw, []byte("SYNTHETIC_PRIVATE_VALUE")) {
			t.Fatal("discovery leaked body")
		}
	}
	discovered, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "discover_entries", Arguments: map[string]any{"query": "SYNTHETIC_PRIVATE_VALUE"}})
	if err != nil {
		t.Fatal(err)
	}
	if out := toolOutput[discoverResult](t, discovered); len(out.Entries) != 0 {
		t.Fatal("MCP discovery searched private content", out)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "replace_entries", Arguments: map[string]any{"changes": []map[string]any{{"key": first.Key, "expected_revision": rows[0].Revision, "group": "forged", "value": "SYNTHETIC_NEW"}}}})
	// The SDK may reject unknown fields; even if ignored, the group must stay intact.
	_ = result
	_ = err
	current, err := b.app.Vault.Get(first.Key)
	if err != nil || current.Group != g {
		t.Fatal("MCP changed group", err)
	}
	req, _ := http.NewRequest("GET", b.url+"/api/groups", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("bearer accessed human catalog", response.StatusCode)
	}
}
func copyFixture(t *testing.T, source string) string {
	t.Helper()
	root := t.TempDir()
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(root, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, raw, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	return root
}
func TestActualV04VaultSurvivesBrandAndExplicitGroupUpgrade(t *testing.T) {
	root := copyFixture(t, "testdata/v2")
	b := setupSelectionHTTP(t, root, t.TempDir())
	original := map[string][]byte{}
	paths, _ := filepath.Glob(filepath.Join(root, "records", "*.json"))
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		original[path] = raw
	}
	if code, data := b.request("POST", "/api/vault/unlock", map[string]string{"password": "synthetic-test-password"}, b.csrf); code != 200 {
		t.Fatal(code, string(data))
	}
	if b.app.Vault.Status().Format != 2 {
		t.Fatal("implicitly migrated legacy vault")
	}
	entry, err := b.app.Vault.Get("legacy/v2")
	if err != nil || entry.Group != "" || entry.Value != "SYNTHETIC_V2_VALUE" || string(entry.Attachments[0].Data) != "SYNTHETIC_ATTACHMENT" {
		t.Fatal(err)
	}
	trash, _ := b.app.Vault.List(vault.ListOptions{Scope: vault.ListTrash})
	if len(trash) != 1 {
		t.Fatal(trash)
	}
	code, _ := b.request("POST", "/api/groups/move", vault.GroupMove{Group: "A", Entries: []vault.GroupMember{{Key: entry.Key, ExpectedRevision: entry.Revision}}}, b.csrf)
	if code != 409 {
		t.Fatal("group writes allowed before migration", code)
	}
	for _, target := range []int{0, 2} {
		code, _ = b.request("POST", "/api/vault/upgrade", map[string]any{"password": "synthetic-test-password", "confirmed": true, "target_format": target}, b.csrf)
		if code != 409 || b.app.Vault.Status().Format != 2 {
			t.Fatal("stale consent migrated", code)
		}
	}
	code, data := b.request("POST", "/api/vault/upgrade", map[string]any{"password": "synthetic-test-password", "confirmed": true, "target_format": 3}, b.csrf)
	if code != 200 || !b.app.Vault.Locked() {
		t.Fatal(code, string(data))
	}
	for path, want := range original {
		actual, _ := os.ReadFile(path)
		if !bytes.Equal(want, actual) {
			t.Fatal("rewrote legacy history")
		}
	}
	if code, _ = b.request("POST", "/api/vault/unlock", map[string]string{"password": "synthetic-test-password"}, b.csrf); code != 200 {
		t.Fatal(code)
	}
	if b.app.Vault.Status().Format != 3 {
		t.Fatal("wrong format")
	}
	code, data = b.request("POST", "/api/groups/move", vault.GroupMove{Group: "迁移资料", Entries: []vault.GroupMember{{Key: entry.Key, ExpectedRevision: entry.Revision}}}, b.csrf)
	if code != 200 {
		t.Fatal(code, string(data))
	}
	current, err := b.app.Vault.Get(entry.Key)
	if err != nil || current.Group != "迁移资料" || current.Value != entry.Value {
		t.Fatal(err)
	}
	trash, _ = b.app.Vault.List(vault.ListOptions{Scope: vault.ListTrash})
	if len(trash) != 1 {
		t.Fatal("migration revived tombstone")
	}
}
