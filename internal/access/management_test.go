package access

import (
	"github.com/shenjiecode/keyfolio/internal/vault"
	"testing"
)

func TestManagementInvalidatesPendingAndBufferedReads(t *testing.T) {
	for _, operation := range []string{"rename", "delete"} {
		for _, approved := range []bool{false, true} {
			t.Run(operation+map[bool]string{false: "-pending", true: "-buffered"}[approved], func(t *testing.T) {
				service, entry := setup(t)
				request, err := service.Read("client", "Synthetic", []vault.ReadSpec{{Key: "demo", IncludeValue: true}})
				if err != nil {
					t.Fatal(err)
				}
				write, err := service.Replace("writer", "Writer", []vault.Change{{Key: "demo", ExpectedRevision: entry.Revision, Value: ptr("AI proposal")}})
				if err != nil {
					t.Fatal(err)
				}
				if approved {
					if err = service.Decide(request.RequestID, true); err != nil {
						t.Fatal(err)
					}
				}
				if operation == "rename" {
					_, err = service.Vault.Rename(vault.RenameChange{Key: "demo", NewKey: "renamed", ExpectedRevision: entry.Revision})
				} else {
					_, err = service.Vault.Delete(vault.DeleteChange{Key: "demo", ExpectedHeads: []string{entry.Revision}})
				}
				if err != nil {
					t.Fatal(err)
				}
				if !approved {
					if err = service.Decide(request.RequestID, true); err == nil {
						t.Fatal("old approval accepted")
					}
				}
				result, err := service.Result("client", request.RequestID)
				if err != nil || result.State != "failed" || len(result.Reads) != 0 {
					t.Fatal("old data released", result, err)
				}
				if err = service.Decide(write.RequestID, true); err == nil {
					t.Fatal("old write applied")
				}
			})
		}
	}
}
