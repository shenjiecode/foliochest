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
	"github.com/shenjiecode/keyfolio/internal/access"
)

// stdio session dialing the bridge with an optional X-FolioChest-Approval header.
func approvalStdioSession(t *testing.T, b *testBrowser, protocol string, options *mcp.ClientOptions, approval string) *mcp.ClientSession {
	t.Helper()
	conn, err := net.Dial("tcp", strings.TrimPrefix(b.url, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	req, _ := http.NewRequest("GET", b.url+"/internal/mcp/stdio", nil)
	req.Header.Set("X-FolioChest-Stdio", b.app.mcpTransport.token)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "foliochest-stdio")
	if approval != "" {
		req.Header.Set("X-FolioChest-Approval", approval)
	}
	if err := req.Write(conn); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, req)
	if err != nil || response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatal(response, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	client := mcp.NewClient(&mcp.Implementation{Name: "synthetic-approval", Version: "1"}, options)
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: bridgeReader{reader, conn}, Writer: conn}, &mcp.ClientSessionOptions{ProtocolVersion: protocol})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

type approvalBearerTransport struct{ token, approval string }

func (t approvalBearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	copyReq := req.Clone(req.Context())
	copyReq.Header = req.Header.Clone()
	copyReq.Header.Set("Authorization", "Bearer "+t.token)
	if t.approval != "" {
		copyReq.Header.Set("X-FolioChest-Approval", t.approval)
	}
	return http.DefaultTransport.RoundTrip(copyReq)
}

func TestWebApprovalModeSkipsHarnessElicitation(t *testing.T) {
	for _, transport := range []string{"stdio", "http"} {
		t.Run(transport, func(t *testing.T) {
			b := setupHTTP(t)
			b.seed()
			if transport == "http" {
				enableTestHTTP(b)
			}
			prompts := 0
			options := &mcp.ClientOptions{ElicitationHandler: func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				prompts++
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approve": true}}, nil
			}}
			var session *mcp.ClientSession
			if transport == "stdio" {
				session = approvalStdioSession(t, b, "2026-07-28", options, "web")
			} else {
				_, token, err := b.app.clients.Add("synthetic-web-approval")
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				client := mcp.NewClient(&mcp.Implementation{Name: "synthetic-web-approval", Version: "1"}, options)
				tr := &mcp.StreamableClientTransport{Endpoint: b.url + "/mcp", HTTPClient: &http.Client{Transport: approvalBearerTransport{token, "web"}}, MaxRetries: -1}
				session, err = client.Connect(ctx, tr, &mcp.ClientSessionOptions{ProtocolVersion: "2026-07-28"})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { session.Close() })
			}
			result, err := session.CallTool(t.Context(), syntheticRead())
			if err != nil {
				t.Fatal(err)
			}
			out := toolOutput[access.Result](t, result)
			if prompts != 0 {
				t.Fatalf("elicitation handler invoked %d times", prompts)
			}
			if out.State != "pending" || out.RequestID == "" || out.ApprovalURL == "" {
				t.Fatalf("state=%s request_id=%q approval_url=%q", out.State, out.RequestID, out.ApprovalURL)
			}
			if !strings.Contains(out.Message, "仅本地网页审批") {
				t.Fatalf("unexpected message: %s", out.Message)
			}
			raw, _ := json.Marshal(result)
			if strings.Contains(string(raw), "SYNTHETIC_PRIVATE_VALUE") {
				t.Fatal("secret leaked in pending result")
			}
			pending := b.app.Access.Pending()
			if len(pending) != 1 || pending[0].ID != out.RequestID {
				t.Fatalf("pending queue mismatch: %d", len(pending))
			}
			if err := b.app.Access.Decide(pending[0].ID, true); err != nil {
				t.Fatal(err)
			}
			result, err = session.CallTool(t.Context(), &mcp.CallToolParams{Name: "request_result", Arguments: resultArgs{RequestID: out.RequestID}})
			if err != nil {
				t.Fatal(err)
			}
			done := toolOutput[access.Result](t, result)
			if done.State != "complete" || len(done.Reads) != 1 || *done.Reads[0].Value != "SYNTHETIC_PRIVATE_VALUE" {
				t.Fatalf("approved read not returned once: %+v", done)
			}
			replay, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "request_result", Arguments: resultArgs{RequestID: out.RequestID}})
			if err != nil {
				t.Fatal(err)
			}
			again := toolOutput[access.Result](t, replay)
			if len(again.Reads) != 0 {
				t.Fatalf("replayed secret: %+v", again)
			}
		})
	}
}

func TestWebApprovalModeLegacyStdioSkipsElicitation(t *testing.T) {
	b := setupHTTP(t)
	b.seed()
	prompts := 0
	options := &mcp.ClientOptions{ElicitationHandler: func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		prompts++
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approve": true}}, nil
	}}
	session := approvalStdioSession(t, b, "2025-11-25", options, "web")
	if session.InitializeResult().ProtocolVersion != "2025-11-25" {
		t.Fatal("protocol downgraded")
	}
	result, err := session.CallTool(t.Context(), syntheticRead())
	if err != nil {
		t.Fatal(err)
	}
	out := toolOutput[access.Result](t, result)
	if prompts != 0 {
		t.Fatalf("elicitation handler invoked %d times", prompts)
	}
	if out.State != "pending" || out.RequestID == "" || out.ApprovalURL == "" || !strings.Contains(out.Message, "仅本地网页审批") {
		t.Fatalf("unexpected legacy fallback: %+v", out)
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "SYNTHETIC_PRIVATE_VALUE") {
		t.Fatal("secret leaked in pending result")
	}
	if len(b.app.Access.Pending()) != 1 {
		t.Fatal("missing pending approval")
	}
}

func TestInvalidApprovalHeaderRejected(t *testing.T) {
	b := setupHTTP(t)
	b.seed()
	// stdio bridge: authenticated upgrade with an unsupported approval value.
	conn, err := net.Dial("tcp", strings.TrimPrefix(b.url, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	req, _ := http.NewRequest("GET", b.url+"/internal/mcp/stdio", nil)
	req.Header.Set("X-FolioChest-Stdio", b.app.mcpTransport.token)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "foliochest-stdio")
	req.Header.Set("X-FolioChest-Approval", "harness")
	if err = req.Write(conn); err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil || response.StatusCode != 400 {
		t.Fatal(response, err)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err = json.NewDecoder(response.Body).Decode(&body); err != nil || !strings.Contains(body.Error, "无效的审批方式") {
		t.Fatal(body, err)
	}
	// HTTP endpoint with an unsupported approval value.
	enableTestHTTP(b)
	_, token, err := b.app.clients.Add("synthetic-bad-approval")
	if err != nil {
		t.Fatal(err)
	}
	httpReq, _ := http.NewRequest("POST", b.url+"/mcp", strings.NewReader("{}"))
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("X-FolioChest-Approval", "harness")
	res, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatal("HTTP /mcp accepted invalid approval header", res.StatusCode)
	}
	if err = json.NewDecoder(res.Body).Decode(&body); err != nil || !strings.Contains(body.Error, "无效的审批方式") {
		t.Fatal(body, err)
	}
}
