package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shenjiecode/keyfolio/internal/vault"
)

func enableTestHTTP(b *testBrowser) {
	b.t.Helper()
	code, data := b.request("POST", "/api/mcp/settings", map[string]any{"mode": "http", "confirmed": true}, b.csrf)
	if code != 200 {
		b.t.Fatal(code, string(data))
	}
	code, data = b.request("POST", "/api/vault/unlock", map[string]string{"password": "synthetic-test-password"}, b.csrf)
	if code != 200 {
		b.t.Fatal(code, string(data))
	}
}
func TestMCPDiscoveryProjectionPaginationAndGroups(t *testing.T) {
	b := setupHTTP(t)
	original := b.seed()
	description, value, group := "second public", "synthetic private", "Work"
	_, err := b.app.Vault.Commit([]vault.Change{{Key: "second", Description: &description, Value: &value, Group: &group}}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	no := false
	out, err := b.app.discoverEntries(discoverArgs{Limit: 1, IncludeDescription: &no})
	if err != nil || out.Total != 2 || len(out.Entries) != 1 || out.NextOffset == nil {
		t.Fatal(out, err)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "description") || strings.Contains(string(raw), value) {
		t.Fatal("projection leaked omitted field")
	}
	out, err = b.app.discoverEntries(discoverArgs{Offset: *out.NextOffset, Limit: 1})
	if err != nil || len(out.Entries) != 1 || out.Entries[0].Key != "second" || out.NextOffset != nil {
		t.Fatal(out, err)
	}
	out, err = b.app.discoverEntries(discoverArgs{Keys: []string{original.Key}, Query: "deployment"})
	if err != nil || len(out.Entries) != 1 {
		t.Fatal(out, err)
	}
	if _, err = b.app.discoverEntries(discoverArgs{Limit: 201}); err == nil {
		t.Fatal("unbounded page")
	}
	code, _ := b.request("POST", "/api/groups/create", map[string]string{"name": "local empty"}, b.csrf)
	if code != 200 {
		t.Fatal(code)
	}
	groups, err := b.app.listMCPGroups(listGroupsArgs{})
	if err != nil || len(groups.Groups) != 2 {
		t.Fatal(groups, err)
	}
	groups, err = b.app.listMCPGroups(listGroupsArgs{IncludeEmpty: true})
	if err != nil || len(groups.Groups) != 3 {
		t.Fatal(groups, err)
	}
	b.app.Access.Lock()
	if _, err = b.app.listMCPGroups(listGroupsArgs{}); err == nil {
		t.Fatal("groups accessible while locked")
	}
}
func TestStdioBridgeAndExclusiveMode(t *testing.T) {
	b := setupHTTP(t)
	b.seed()
	if b.app.mcpMode() != "stdio" {
		t.Fatal("not default stdio")
	}
	code, _ := b.request("POST", "/mcp", map[string]any{}, b.csrf)
	if code != 404 {
		t.Fatal("HTTP enabled by default", code)
	}
	code, _ = b.request("POST", "/api/clients/create", map[string]string{"name": "hidden HTTP"}, b.csrf)
	if code != 409 {
		t.Fatal("created HTTP credential in stdio mode", code)
	}
	for _, token := range []string{"", b.app.LauncherToken, "synthetic-bearer"} {
		req, _ := http.NewRequest("GET", b.url+"/internal/mcp/stdio", nil)
		req.Header.Set("X-FolioChest-Stdio", token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 401 {
			t.Fatal("unauthenticated IPC", res.StatusCode)
		}
	}
	conn, err := net.Dial("tcp", strings.TrimPrefix(b.url, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	req, _ := http.NewRequest("GET", b.url+"/internal/mcp/stdio", nil)
	req.Header.Set("X-FolioChest-Stdio", b.app.mcpTransport.token)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "foliochest-stdio")
	if err = req.Write(conn); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, req)
	if err != nil || response.StatusCode != 101 {
		t.Fatal(response, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "synthetic stdio", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: bridgeReader{reader, conn}, Writer: conn}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 5 {
		t.Fatal(tools, err)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "discover_entries", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatal(result, err)
	}
	b.app.Access.Lock()
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "discover_entries", Arguments: map[string]any{}})
	if err == nil && !result.IsError {
		t.Fatal("stdio bypassed lock")
	}
	code, _ = b.request("POST", "/api/vault/unlock", map[string]string{"password": "synthetic-test-password"}, b.csrf)
	if code != 200 {
		t.Fatal(code)
	}
	enableTestHTTP(b)
	if _, err = session.ListTools(ctx, nil); err == nil {
		t.Fatal("stdio survived switching to HTTP")
	}
	restarted := setupSelectionHTTP(t, b.app.Vault.Status().Directory, b.app.deviceDir)
	if restarted.app.mcpMode() != "http" {
		t.Fatal("mode not persisted")
	}
}
