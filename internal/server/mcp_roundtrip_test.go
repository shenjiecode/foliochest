package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shenjiecode/keyfolio/internal/access"
	"github.com/shenjiecode/keyfolio/internal/vault"
)

func modernSession(t *testing.T, b *testBrowser, transport string, options *mcp.ClientOptions) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	client := mcp.NewClient(&mcp.Implementation{Name: "synthetic-modern", Version: "1"}, options)
	var tr mcp.Transport
	if transport == "stdio" {
		conn, err := net.Dial("tcp", strings.TrimPrefix(b.url, "http://"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close() })
		req, _ := http.NewRequest("GET", b.url+"/internal/mcp/stdio", nil)
		req.Header.Set("X-FolioChest-Stdio", b.app.mcpTransport.token)
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Upgrade", "foliochest-stdio")
		if err := req.Write(conn); err != nil {
			t.Fatal(err)
		}
		reader := bufio.NewReader(conn)
		response, err := http.ReadResponse(reader, req)
		if err != nil || response.StatusCode != 101 {
			t.Fatal(response, err)
		}
		tr = &mcp.IOTransport{Reader: bridgeReader{reader, conn}, Writer: conn}
	} else {
		_, token, err := b.app.clients.Add("synthetic-modern")
		if err != nil {
			t.Fatal(err)
		}
		tr = &mcp.StreamableClientTransport{Endpoint: b.url + "/mcp", HTTPClient: &http.Client{Transport: bearerTransport{token}}, MaxRetries: -1}
	}
	session, err := client.Connect(ctx, tr, &mcp.ClientSessionOptions{ProtocolVersion: "2026-07-28"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	if session.InitializeResult().ProtocolVersion != "2026-07-28" {
		t.Fatal("protocol downgraded")
	}
	return session
}

func syntheticRead() *mcp.CallToolParams {
	return &mcp.CallToolParams{Name: "read_entries", Arguments: readArgs{Entries: []vault.ReadSpec{{Key: "demo/deploy", IncludeValue: true}}}}
}

func TestModernApprovalAutomatic(t *testing.T) {
	for _, transport := range []string{"stdio", "http"} {
		for _, mode := range []string{"accept", "decline", "cancel", "missing", "false", "web-deny", "web-accept"} {
			t.Run(transport+"/"+mode, func(t *testing.T) {
				b := setupHTTP(t)
				b.seed()
				if transport == "http" {
					enableTestHTTP(b)
				}
				prompts := 0
				session := modernSession(t, b, transport, &mcp.ClientOptions{ElicitationHandler: func(ctx context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
					prompts++
					if strings.Contains(req.Params.Message, "SYNTHETIC_PRIVATE_VALUE") {
						t.Error("secret in approval")
					}
					if mode == "web-deny" || mode == "web-accept" {
						pending := b.app.Access.Pending()
						if len(pending) != 1 {
							t.Error("duplicated approval")
							return &mcp.ElicitResult{Action: "cancel"}, nil
						}
						if err := b.app.Access.Decide(pending[0].ID, mode == "web-accept"); err != nil {
							t.Error(err)
						}
					}
					switch mode {
					case "decline", "cancel":
						return &mcp.ElicitResult{Action: mode}, nil
					case "missing":
						return &mcp.ElicitResult{Action: "accept"}, nil
					case "false":
						return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approve": false}}, nil
					default:
						return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approve": true}}, nil
					}
				}})
				result, err := session.CallTool(t.Context(), syntheticRead())
				if err != nil {
					t.Fatal(err)
				}
				out := toolOutput[access.Result](t, result)
				want := "denied"
				if mode == "accept" || mode == "web-accept" {
					want = "complete"
				}
				if mode == "cancel" {
					want = "cancelled"
				}
				if prompts != 1 || out.State != want {
					t.Fatalf("prompts=%d state=%s want=%s", prompts, out.State, want)
				}
				if want == "complete" && (len(out.Reads) != 1 || *out.Reads[0].Value != "SYNTHETIC_PRIVATE_VALUE") {
					t.Fatal("missing result")
				}
				if want != "complete" && len(out.Reads) != 0 {
					t.Fatal("unapproved content")
				}
			})
		}
	}
}

func rawModernOptions() *mcp.ClientOptions {
	return &mcp.ClientOptions{MultiRoundTrip: &mcp.MultiRoundTripOptions{Disabled: true}, ElicitationHandler: func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "cancel"}, nil
	}}
}

func TestModernContinuationBinding(t *testing.T) {
	b := setupHTTP(t)
	b.seed()
	session := modernSession(t, b, "stdio", rawModernOptions())
	other := modernSession(t, b, "stdio", rawModernOptions())
	first, err := session.CallTool(t.Context(), syntheticRead())
	if err != nil {
		t.Fatal(err)
	}
	if !first.NeedsInput() || first.RequestState == "" || len(first.InputRequests) != 1 {
		t.Fatal("not input_required")
	}
	raw, _ := json.Marshal(first)
	if strings.Contains(string(raw), "SYNTHETIC_PRIVATE_VALUE") || len(first.Content) > 0 || first.StructuredContent != nil {
		t.Fatal("content in input_required")
	}
	retry := syntheticRead()
	retry.RequestState = first.RequestState
	retry.InputResponses = mcp.InputResponseMap{"approval": &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approve": true}}}
	for _, mode := range []string{"wrong-client", "changed-scope", "wrong-state", "wrong-response-id", "wrong-tool", "response-without-state"} {
		t.Run(mode, func(t *testing.T) {
			p := *retry
			target := session
			switch mode {
			case "wrong-client":
				target = other
			case "changed-scope":
				p.Arguments = readArgs{Entries: []vault.ReadSpec{{Key: "demo/deploy", IncludeValue: true, AllAttachments: true}}}
			case "wrong-state":
				p.RequestState = "forged"
			case "wrong-response-id":
				p.InputResponses = mcp.InputResponseMap{"wrong": &mcp.ElicitResult{Action: "accept"}}
			case "wrong-tool":
				p.Name = "discover_entries"
				p.Arguments = map[string]any{}
			case "response-without-state":
				p.RequestState = ""
			}
			r, err := target.CallTool(t.Context(), &p)
			if err == nil && !r.IsError {
				t.Fatal("invalid continuation accepted")
			}
			if len(b.app.Access.Pending()) != 1 {
				t.Fatal("invalid continuation changed approval queue")
			}
		})
	}
	// Two simultaneous responses must release the secret exactly once.
	var wg sync.WaitGroup
	results := make(chan bool, 2)
	for range 2 {
		// The SDK injects metadata into caller-owned params; each concurrent
		// request needs its own params and metadata map.
		p := *retry
		p.Meta = nil
		wg.Go(func() {
			r, err := session.CallTool(t.Context(), &p)
			results <- err == nil && !r.IsError
		})
	}
	wg.Wait()
	close(results)
	successes := 0
	for ok := range results {
		if ok {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent approval successes: %d", successes)
	}
	r, err := session.CallTool(t.Context(), retry)
	if err == nil && !r.IsError {
		t.Fatal("replay accepted")
	}
	// Reusing the original arguments must cause a fresh approval, not implicit consent.
	r, err = session.CallTool(t.Context(), syntheticRead())
	if err != nil || !r.NeedsInput() {
		t.Fatal("new read inherited approval", err)
	}
}

func TestModernContinuationInvalidation(t *testing.T) {
	for _, mode := range []string{"lock", "revision", "policy", "disconnect", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			b := setupHTTP(t)
			original := b.seed()
			session := modernSession(t, b, "stdio", rawModernOptions())
			first, err := session.CallTool(t.Context(), syntheticRead())
			if err != nil {
				t.Fatal(err)
			}
			pending := b.app.Access.Pending()
			if len(pending) != 1 {
				t.Fatal("missing approval")
			}
			switch mode {
			case "lock":
				b.app.Access.Lock()
			case "revision":
				value := "SYNTHETIC_CHANGED"
				_, err = b.app.Vault.Commit([]vault.Change{{Key: original.Key, ExpectedRevision: original.Revision, Value: &value}}, false, true)
			case "policy":
				deny := vault.Policy("deny")
				_, err = b.app.Vault.Commit([]vault.Change{{Key: original.Key, ExpectedRevision: original.Revision, ReadPolicy: &deny}}, true, true)
			case "disconnect":
				session.Close()
				session = modernSession(t, b, "stdio", rawModernOptions())
			case "timeout":
				// Test the same continuation validator with an aged server-side clock value.
				c := &readContinuations{items: map[string]readContinuation{first.RequestState: {clientID: pending[0].ClientID, requestID: pending[0].ID, arguments: readArgumentsHash(readArgs{Entries: []vault.ReadSpec{{Key: original.Key, IncludeValue: true}}}), created: time.Now().Add(-3 * time.Minute)}}}
				out, e := c.resume(t.Context(), b.app, Client{ID: pending[0].ClientID}, &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{RequestState: first.RequestState, InputResponses: mcp.InputResponseMap{"approval": &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approve": true}}}}}, readArgs{Entries: []vault.ReadSpec{{Key: original.Key, IncludeValue: true}}})
				if e != nil || out.State != "timed_out" || len(out.Reads) != 0 {
					t.Fatal(out.State, e)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			p := syntheticRead()
			p.RequestState = first.RequestState
			p.InputResponses = mcp.InputResponseMap{"approval": &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approve": true}}}
			r, err := session.CallTool(t.Context(), p)
			if err == nil && !r.IsError {
				out := toolOutput[access.Result](t, r)
				if out.State == "complete" || len(out.Reads) != 0 {
					t.Fatal("invalidated approval leaked")
				}
			}
		})
	}
}

func TestModernWithoutCapabilityFallsBack(t *testing.T) {
	b := setupHTTP(t)
	b.seed()
	session := modernSession(t, b, "stdio", nil)
	r, err := session.CallTool(t.Context(), syntheticRead())
	if err != nil {
		t.Fatal(err)
	}
	out := toolOutput[access.Result](t, r)
	if out.State != "pending" || !strings.Contains(out.Message, "未声明") || len(out.Reads) != 0 {
		t.Fatal("wrong fallback")
	}
}
