package server

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shenjiecode/keyfolio/internal/access"
)

// Only opaque random handles cross the wire. The original operation and expiry
// stay server-side; a response can never replace its key, scope or revision.
// This store belongs to one authenticated MCP client (one connection for stdio).
// It contains no plaintext values and is bounded independently of the access queue.
type readContinuations struct {
	mu    sync.Mutex
	items map[string]readContinuation
}

type readContinuation struct {
	clientID  string
	requestID string
	arguments [32]byte
	created   time.Time
}

func readArgumentsHash(args readArgs) [32]byte {
	raw, _ := json.Marshal(args)
	return sha256.Sum256(raw)
}

func approvalForm(message string) *mcp.ElicitParams {
	return &mcp.ElicitParams{Mode: "form", Message: message, RequestedSchema: map[string]any{
		"type":       "object",
		"properties": map[string]any{"approve": map[string]any{"type": "boolean", "title": "批准本次操作", "description": "仅当你同意以上全部范围时选择是；拒绝或取消不会授权。"}},
		"required":   []string{"approve"},
	}}
}

func elapsedApproval(now, created time.Time) time.Duration {
	return max(now.Sub(created), now.Round(0).Sub(created.Round(0)))
}

func (c *readContinuations) begin(client Client, args readArgs, out access.Result, message string) *mcp.CallToolResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.items == nil {
		c.items = make(map[string]readContinuation)
	}
	now := time.Now()
	for token, item := range c.items {
		if elapsedApproval(now, item.created) > 5*time.Minute {
			delete(c.items, token)
		}
	}
	if len(c.items) >= 32 {
		return nil // Keep the existing request available in the webpage.
	}
	token := access.ID()
	c.items[token] = readContinuation{clientID: client.ID, requestID: out.RequestID, arguments: readArgumentsHash(args), created: now}
	return &mcp.CallToolResult{RequestState: token, InputRequests: mcp.InputRequestMap{"approval": approvalForm(message)}}
}

func (c *readContinuations) resume(ctx context.Context, s *Server, client Client, req *mcp.CallToolRequest, args readArgs) (access.Result, error) {
	if err := ctx.Err(); err != nil {
		return access.Result{}, err
	}
	p := req.Params
	c.mu.Lock()
	item, ok := c.items[p.RequestState]
	if !ok || item.clientID != client.ID || item.arguments != readArgumentsHash(args) {
		c.mu.Unlock()
		return access.Result{}, errors.New("原生审批续传无效或操作范围已变化，请重新请求")
	}
	answer, typed := p.InputResponses["approval"].(*mcp.ElicitResult)
	if len(p.InputResponses) != 1 || !typed || answer == nil || (answer.Action != "accept" && answer.Action != "decline" && answer.Action != "cancel") {
		c.mu.Unlock()
		return access.Result{}, errors.New("原生审批响应无效；未授权本次操作")
	}
	// Claim before executing: concurrent/replayed responses cannot reapply a decision.
	delete(c.items, p.RequestState)
	c.mu.Unlock()

	state, err := s.Access.Status(client.ID, item.requestID)
	if err != nil {
		return access.Result{}, err
	}
	if state != "pending" {
		// A webpage decision, lock, revocation or prior consumption wins.
		return s.Access.Result(client.ID, item.requestID)
	}
	if elapsedApproval(time.Now(), item.created) >= 2*time.Minute {
		err = s.Access.CancelWaiting(client.ID, item.requestID, true)
	} else if answer.Action == "cancel" {
		err = s.Access.CancelWaiting(client.ID, item.requestID, false)
	} else {
		err = s.Access.Decide(item.requestID, answer.Action == "accept" && answer.Content["approve"] == true)
	}
	if err != nil {
		// A simultaneous webpage decision is authoritative; never overwrite it.
		state, statusErr := s.Access.Status(client.ID, item.requestID)
		if statusErr != nil || state == "pending" {
			return access.Result{}, err
		}
	}
	return s.Access.Result(client.ID, item.requestID)
}

// Return input_required only for clients that explicitly advertise modern form
// support. Do not send legacy server-initiated requests on a modern connection.
func (s *Server) beginRoundTrip(req *mcp.CallToolRequest, client Client, args readArgs, out access.Result, continuations *readContinuations) (*mcp.CallToolResult, access.Result, error) {
	if out.State != "pending" {
		return nil, out, nil
	}
	caps := req.ClientCapabilities()
	if caps == nil || caps.Elicitation == nil || caps.Elicitation.Form == nil {
		out.Message = "当前客户端未声明 MCP 表单审批能力，请在本地网页审批。"
		return nil, out, nil
	}
	preview, err := s.Access.Preview(client.ID, out.RequestID)
	if err != nil {
		out, err = s.Access.Result(client.ID, out.RequestID)
		return nil, out, err
	}
	message, ok := approvalMessage(preview)
	if !ok {
		out.Message = "当前读取范围不适合简短确认框，请在本地网页详细审阅并批准。"
		return nil, out, nil
	}
	result := continuations.begin(client, args, out, message)
	if result == nil {
		out.Message = "原生审批续传队列已满，请在本地网页审批。"
	}
	return result, out, nil
}
