package access

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/shenjiecode/keyfolio/internal/vault"
)

const requestTTL = 5 * time.Minute

type Result struct {
	RequestID   string             `json:"request_id,omitempty"`
	State       string             `json:"state"`
	ApprovalURL string             `json:"approval_url,omitempty"`
	Reads       []vault.ReadResult `json:"reads,omitempty"`
	Changes     []vault.Summary    `json:"changes,omitempty"`
	Message     string             `json:"message,omitempty"`
}
type Pending struct {
	ID            string           `json:"id"`
	ClientID      string           `json:"client_id"`
	ClientName    string           `json:"client_name"`
	Operation     string           `json:"operation"`
	Reads         []vault.ReadSpec `json:"reads,omitempty"`
	Changes       []vault.Change   `json:"changes,omitempty"`
	CreatedAt     time.Time        `json:"created_at"`
	ExpiresAt     time.Time        `json:"expires_at"`
	State         string           `json:"state"`
	generation    uint64
	result        Result
	bufferedBytes int
}
type Service struct {
	mu          sync.Mutex
	Vault       *vault.Vault
	requests    map[string]*Pending
	approvalURL string
	now         func() time.Time
}

func New(v *vault.Vault, baseURL string) *Service {
	return &Service{Vault: v, requests: map[string]*Pending{}, approvalURL: baseURL + "/#approvals", now: time.Now}
}
func ID() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func clone[T any](in T) T {
	b, _ := json.Marshal(in)
	var out T
	_ = json.Unmarshal(b, &out)
	return out
}
func (s *Service) expire() {
	now := s.now()
	for id, p := range s.requests {
		// Linux monotonic time excludes suspend. Expire on whichever clock
		// reaches the deadline first; a backwards wall jump cannot extend it.
		overdue := max(now.Sub(p.ExpiresAt), now.Round(0).Sub(p.ExpiresAt.Round(0)))
		if overdue > 0 || p.generation != s.Vault.Generation() {
			p.State = "expired"
			p.Reads = nil
			p.Changes = nil
			p.result = Result{RequestID: id, State: "expired", Message: "请求已过期或资料库解锁会话已变化"}
			p.bufferedBytes = 0
		}
		if overdue > requestTTL {
			delete(s.requests, id)
		}
	}
}
func (s *Service) enqueue(clientID, clientName, op string, reads []vault.ReadSpec, changes []vault.Change) (Result, error) {
	s.expire()
	if len(s.requests) >= 32 {
		return Result{}, errors.New("待处理请求过多，请稍后重试")
	}
	encoded, _ := json.Marshal(map[string]any{"reads": reads, "changes": changes})
	if s.buffered()+len(encoded) > 32<<20 {
		return Result{}, errors.New("审批缓冲区已满，请先处理现有请求")
	}
	p := &Pending{ID: ID(), ClientID: clientID, ClientName: clientName, Operation: op, Reads: clone(reads), Changes: clone(changes), CreatedAt: s.now(), ExpiresAt: s.now().Add(requestTTL), State: "pending", generation: s.Vault.Generation()}
	p.bufferedBytes = len(encoded)
	s.requests[p.ID] = p
	return Result{RequestID: p.ID, State: "pending", ApprovalURL: s.approvalURL, Message: "请在 harness 确认框或本地网页批准本次请求，再查询结果。"}, nil
}
func (s *Service) Read(clientID, clientName string, reads []vault.ReadSpec) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pinned, needs, err := s.Vault.PrepareReads(reads)
	if err != nil {
		return Result{}, err
	}
	if needs {
		return s.enqueue(clientID, clientName, "read", pinned, nil)
	}
	out, err := s.Vault.ReadMany(pinned, false)
	return Result{State: "complete", Reads: out}, err
}
func (s *Service) Replace(clientID, clientName string, changes []vault.Change) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	needs, err := s.Vault.CheckChanges(changes, false)
	if err != nil {
		return Result{}, err
	}
	if needs {
		return s.enqueue(clientID, clientName, "replace", nil, changes)
	}
	out, err := s.Vault.Commit(changes, false, false)
	return Result{State: "complete", Changes: out}, err
}

// Decide is called only by an authenticated human UI or a trusted elicitation response.
// It is deliberately NOT exposed as an MCP tool.
func (s *Service) Decide(id string, accept bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expire()
	p, ok := s.requests[id]
	if !ok {
		return errors.New("请求不存在")
	}
	if p.State != "pending" {
		return errors.New("请求已处理或过期")
	}
	if !accept {
		p.State = "denied"
		p.result = Result{RequestID: id, State: "denied", Message: "用户拒绝本次操作"}
		p.Reads = nil
		p.Changes = nil
		p.bufferedBytes = 0
		return nil
	}
	result := Result{RequestID: id, State: "complete"}
	var err error
	if p.Operation == "read" {
		result.Reads, err = s.Vault.ReadMany(p.Reads, true)
	} else {
		result.Changes, err = s.Vault.Commit(p.Changes, false, true)
	}
	if err == nil && len(result.Reads) > 0 {
		encoded, _ := json.Marshal(result.Reads)
		if s.buffered()-p.bufferedBytes+len(encoded) > 32<<20 {
			err = errors.New("结果缓冲区已满，请稍后重新请求")
		} else {
			p.bufferedBytes = len(encoded)
		}
	} else {
		p.bufferedBytes = 0
	}
	if err != nil {
		result = Result{RequestID: id, State: "failed", Message: err.Error()}
		p.bufferedBytes = 0
	}
	p.State = result.State
	p.result = result
	p.Reads = nil
	p.Changes = nil
	return err
}
func (s *Service) Result(clientID, id string) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expire()
	p, ok := s.requests[id]
	if !ok || p.ClientID != clientID {
		return Result{}, errors.New("请求不存在")
	}
	if p.State == "pending" {
		return Result{RequestID: id, State: "pending", ApprovalURL: s.approvalURL}, nil
	}
	if p.State == "consumed" {
		return Result{RequestID: id, State: "consumed", Message: "结果已领取，读取内容需要重新请求"}, nil
	}
	if s.Vault.Locked() {
		return Result{}, vault.ErrLocked
	}
	// Recheck pinned metadata before releasing already-approved read results.
	if p.State == "complete" && len(p.result.Reads) > 0 {
		specs := make([]vault.ReadSpec, len(p.result.Reads))
		for i, r := range p.result.Reads {
			specs[i] = vault.ReadSpec{Key: r.Key, Revision: r.Revision, IncludeValue: true}
		}
		if _, _, err := s.Vault.PrepareReads(specs); err != nil {
			p.result = Result{RequestID: id, State: "failed", Message: err.Error()}
			p.State = "failed"
		}
	}
	out := p.result
	p.result = Result{}
	p.bufferedBytes = 0
	p.State = "consumed"
	return out, nil
}
func (s *Service) Pending() []Pending {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expire()
	out := []Pending{}
	for _, p := range s.requests {
		if p.State == "pending" {
			out = append(out, clone(*p))
		}
	}
	return out
}
func (s *Service) Preview(clientID, id string) (Pending, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expire()
	p, ok := s.requests[id]
	if !ok || p.ClientID != clientID || p.State != "pending" {
		return Pending{}, errors.New("请求不存在或已处理")
	}
	return clone(*p), nil
}
func (s *Service) RevokeClient(clientID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.requests {
		if p.ClientID == clientID {
			p.State = "denied"
			p.Reads = nil
			p.Changes = nil
			p.result = Result{RequestID: p.ID, State: "denied", Message: "客户端接入已撤销"}
			p.bufferedBytes = 0
		}
	}
}
func (s *Service) buffered() int {
	total := 0
	for _, p := range s.requests {
		total += p.bufferedBytes
	}
	return total
}
func (s *Service) Sweep() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expire()
}
func (s *Service) Lock() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Vault.Lock()
	s.requests = map[string]*Pending{}
}
