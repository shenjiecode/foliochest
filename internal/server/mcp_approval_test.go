package server

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shenjiecode/keyfolio/internal/access"
	"github.com/shenjiecode/keyfolio/internal/vault"
)

func TestApprovalSummary(t *testing.T) {
	p := access.Pending{Operation: "read", ClientName: "Synthetic", Reads: []vault.ReadSpec{{Key: "demo\nforged", IncludeValue: true, Attachments: []string{"one.txt"}}}, ExpiresAt: time.Now()}
	message, ok := approvalMessage(p)
	if !ok || !strings.Contains(message, `demo\nforged`) || !strings.Contains(message, "one.txt") || !strings.Contains(message, "仅限本次") || strings.Contains(message, "client_id") {
		t.Fatal(message, ok)
	}
	p.Reads[0].Key = strings.Repeat("x", 13000)
	if _, ok = approvalMessage(p); ok {
		t.Fatal("oversized prompt approved")
	}
	p.Operation = "replace"
	if _, ok = approvalMessage(p); ok {
		t.Fatal("replacement must receive full webpage review")
	}
}

func TestApprovalWaitOutcomes(t *testing.T) {
	for _, mode := range []string{"accept", "missing", "false", "cancel", "decline", "web-accept", "web-deny", "web-before-late-accept", "failure", "timeout", "local-timeout", "outer-cancel"} {
		t.Run(mode, func(t *testing.T) {
			b := setupHTTP(t)
			b.seed()
			out, err := b.app.Access.Read("synthetic", "Synthetic", []vault.ReadSpec{{Key: "demo/deploy", IncludeValue: true}})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan struct{})
			elicit := func(wait context.Context) (*mcp.ElicitResult, error) {
				defer close(done)
				switch mode {
				case "web-accept", "web-deny", "web-before-late-accept":
					if err := b.app.Access.Decide(out.RequestID, mode == "web-accept"); err != nil {
						return nil, err
					}
					if mode == "web-before-late-accept" {
						return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approve": true}}, nil
					}
					<-wait.Done()
					return nil, wait.Err()
				case "local-timeout":
					<-wait.Done()
					return nil, wait.Err()
				case "outer-cancel":
					cancel()
					<-wait.Done()
					return nil, wait.Err()
				case "timeout":
					// Exercise the distinct terminal timeout without a two-minute test delay.
					if err := b.app.Access.CancelWaiting("synthetic", out.RequestID, true); err != nil {
						return nil, err
					}
					<-wait.Done()
					return nil, wait.Err()
				case "failure":
					return nil, errors.New("synthetic unavailable client")
				case "missing":
					return &mcp.ElicitResult{Action: "accept"}, nil
				case "false":
					return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approve": false}}, nil
				case "accept":
					return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approve": true}}, nil
				default:
					return &mcp.ElicitResult{Action: mode}, nil
				}
			}
			timeout := time.Second
			if mode == "local-timeout" {
				timeout = 10 * time.Millisecond
			}
			got, err := b.app.waitForApprovalWithin(ctx, "synthetic", out, elicit, timeout)
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("elicitation not cancelled")
			}
			if mode == "outer-cancel" {
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
				got, err = b.app.Access.Result("synthetic", out.RequestID)
			}
			if err != nil {
				t.Fatal(err)
			}
			expected := "denied"
			switch mode {
			case "accept", "web-accept":
				expected = "complete"
			case "cancel", "outer-cancel":
				expected = "cancelled"
			case "timeout", "local-timeout":
				expected = "timed_out"
			case "failure":
				expected = "pending"
			}
			if got.State != expected {
				t.Fatalf("got %s want %s", got.State, expected)
			}
			if expected != "complete" && len(got.Reads) > 0 {
				t.Fatal("unapproved read leaked")
			}
			if expected == "complete" && len(got.Reads) != 1 {
				t.Fatal("approved result missing")
			}
			if expected != "pending" {
				if err := b.app.Access.Decide(out.RequestID, true); err == nil {
					t.Fatal("replayed decision")
				}
			}
		})
	}
}
