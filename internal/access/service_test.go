package access

import (
	"errors"
	"github.com/shenjiecode/keyfolio/internal/vault"
	"testing"
	"time"
)

func ptr[T any](x T) *T { return &x }
func setup(t *testing.T) (*Service, vault.Summary) {
	t.Helper()
	v := vault.New(t.TempDir())
	if err := v.Init("synthetic-test-password"); err != nil {
		t.Fatal(err)
	}
	rows, err := v.Commit([]vault.Change{{Key: "demo", Description: ptr("public metadata"), Value: ptr("SYNTHETIC_SECRET")}}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	return New(v, "http://127.0.0.1:7463"), rows[0]
}
func TestApprovalIsBoundToClientAndOneRead(t *testing.T) {
	s, _ := setup(t)
	r, err := s.Read("client-a", "Client A", []vault.ReadSpec{{Key: "demo", IncludeValue: true}})
	if err != nil || r.State != "pending" || len(r.Reads) != 0 {
		t.Fatal(r, err)
	}
	if _, err = s.Result("client-b", r.RequestID); err == nil {
		t.Fatal("other client accessed request")
	}
	if err = s.Decide(r.RequestID, true); err != nil {
		t.Fatal(err)
	}
	out, err := s.Result("client-a", r.RequestID)
	if err != nil || len(out.Reads) != 1 || *out.Reads[0].Value != "SYNTHETIC_SECRET" {
		t.Fatal(out, err)
	}
	out, err = s.Result("client-a", r.RequestID)
	if err != nil || out.State != "consumed" || len(out.Reads) != 0 {
		t.Fatal(out, err)
	}
	if err = s.Decide(r.RequestID, true); err == nil {
		t.Fatal("replayed approval")
	}
}
func TestStaleApprovalDoesNotWrite(t *testing.T) {
	s, entry := setup(t)
	r, err := s.Replace("client", "Client", []vault.Change{{Key: "demo", ExpectedRevision: entry.Revision, Value: ptr("AI proposal")}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Vault.Commit([]vault.Change{{Key: "demo", ExpectedRevision: entry.Revision, Value: ptr("human edit")}}, true, true); err != nil {
		t.Fatal(err)
	}
	if err = s.Decide(r.RequestID, true); !errors.Is(err, vault.ErrConflict) {
		t.Fatal(err)
	}
	got, _ := s.Vault.Get("demo")
	if got.Value != "human edit" {
		t.Fatal("stale approval overwrote current data")
	}
}
func TestDeniedExpiredAndLockedRequests(t *testing.T) {
	s, _ := setup(t)
	r, _ := s.Read("client", "Client", []vault.ReadSpec{{Key: "demo", IncludeValue: true}})
	if err := s.Decide(r.RequestID, false); err != nil {
		t.Fatal(err)
	}
	out, _ := s.Result("client", r.RequestID)
	if out.State != "denied" {
		t.Fatal(out)
	}
	r, _ = s.Read("client", "Client", []vault.ReadSpec{{Key: "demo", IncludeValue: true}})
	s.now = func() time.Time { return time.Now().Add(6 * time.Minute) }
	if err := s.Decide(r.RequestID, true); err == nil {
		t.Fatal("expired approval accepted")
	}
	s.now = time.Now
	r, _ = s.Read("client", "Client", []vault.ReadSpec{{Key: "demo", IncludeValue: true}})
	s.Lock()
	if _, err := s.Result("client", r.RequestID); err == nil {
		t.Fatal("result accessible after lock")
	}
}
func TestWriteDoesNotReturnOldValue(t *testing.T) {
	s, entry := setup(t)
	deny := vault.Deny
	rows, err := s.Vault.Commit([]vault.Change{{Key: "demo", ExpectedRevision: entry.Revision, ReadPolicy: &deny}}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Replace("client", "Client", []vault.Change{{Key: "demo", ExpectedRevision: rows[0].Revision, Value: ptr("new secret")}})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Decide(r.RequestID, true); err != nil {
		t.Fatal(err)
	}
	out, err := s.Result("client", r.RequestID)
	if err != nil || len(out.Reads) != 0 || len(out.Changes) != 1 {
		t.Fatal(out, err)
	}
}
