package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shenjiecode/keyfolio/internal/access"
)

// Keep every requested read scope visible. Never truncate an approvable prompt.
// Replacements require detailed webpage review rather than a generic yes/no summary.
func approvalMessage(p access.Pending) (string, bool) {
	if p.Operation != "read" || len(p.Reads) == 0 {
		return "", false
	}
	var b strings.Builder
	fmt.Fprintf(&b, "资料匣：允许以下 %d 项读取吗？\n接收方：当前 AI 客户端（%q）\n", len(p.Reads), p.ClientName)
	for _, r := range p.Reads {
		fmt.Fprintf(&b, "\n资料：%q\n范围：", r.Key)
		if r.IncludeValue {
			b.WriteString("正文")
		} else {
			b.WriteString("不读取正文")
		}
		if r.AllAttachments {
			b.WriteString("；全部附件")
		} else if len(r.Attachments) == 0 {
			b.WriteString("；不含附件")
		} else {
			b.WriteString("；附件：")
			for i, name := range r.Attachments {
				if i > 0 {
					b.WriteString("、")
				}
				fmt.Fprintf(&b, "%q", name)
			}
		}
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "\n批准后，以上正文和附件明文将交给当前 AI 客户端。\n仅限本次请求，不改变长期权限。\n请求有效至：%s；本次弹窗最多等待两分钟，客户端可能更早超时。", p.ExpiresAt.Format(time.RFC3339))
	return b.String(), b.Len() <= 12000
}

// Watch the shared decision without consuming it; whichever UI finishes first wins.
// Cancelling Elicit requests closure of the client prompt (client UI support required).
func (s *Server) waitForApproval(ctx context.Context, clientID string, out access.Result, elicit func(context.Context) (*mcp.ElicitResult, error)) (access.Result, error) {
	return s.waitForApprovalWithin(ctx, clientID, out, elicit, 2*time.Minute)
}

func (s *Server) waitForApprovalWithin(ctx context.Context, clientID string, out access.Result, elicit func(context.Context) (*mcp.ElicitResult, error), timeout time.Duration) (access.Result, error) {
	wait, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	type response struct {
		answer *mcp.ElicitResult
		err    error
	}
	replies := make(chan response, 1)
	go func() { answer, err := elicit(wait); replies <- response{answer, err} }()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	finish := func() (access.Result, error) { return s.Access.Result(clientID, out.RequestID) }
	stop := func() (access.Result, error) {
		if err := s.Access.CancelWaiting(clientID, out.RequestID, errors.Is(wait.Err(), context.DeadlineExceeded)); err != nil {
			return access.Result{}, err
		}
		// A cancelled outer call cannot safely deliver/consume an already approved result.
		if ctx.Err() != nil {
			return access.Result{}, ctx.Err()
		}
		return finish()
	}
	for {
		select {
		case <-wait.Done():
			return stop()
		case <-ticker.C:
			state, err := s.Access.Status(clientID, out.RequestID)
			if err != nil {
				return access.Result{}, err
			}
			if state != "pending" {
				return finish()
			}
		case r := <-replies:
			if wait.Err() != nil {
				return stop()
			}
			if r.err != nil {
				state, err := s.Access.Status(clientID, out.RequestID)
				if err != nil {
					return access.Result{}, err
				}
				if state != "pending" {
					return finish()
				}
				out.Message = "客户端未完成原生审批；请求仍待处理，请在本地网页审阅并批准，再查询结果。"
				return out, nil
			}
			if r.answer == nil || r.answer.Action == "cancel" {
				if err := s.Access.CancelWaiting(clientID, out.RequestID, false); err != nil {
					return access.Result{}, err
				}
			} else {
				accept := r.answer.Action == "accept" && r.answer.Content["approve"] == true
				// Decide is atomic; an earlier webpage decision cannot be overwritten.
				if err := s.Access.Decide(out.RequestID, accept); err != nil {
					state, statusErr := s.Access.Status(clientID, out.RequestID)
					if statusErr != nil || state == "pending" {
						return access.Result{}, err
					}
				}
			}
			return finish()
		}
	}
}
