package server

import (
	"github.com/shenjiecode/keyfolio/internal/vault"
	"testing"
)

func TestGroupRegistryMergePreservesEmptyAndAuthoritativeMembers(t *testing.T) {
	reg := groupRegistry{Names: []string{"empty", "populated"}}
	live := []vault.GroupInfo{{Name: "populated", Count: 1, Members: []vault.GroupMember{{Key: "synthetic", ExpectedRevision: "revision"}}}, {Name: "new", DeletedCount: 1}}
	groups, changed := mergeGroupRegistry(live, &reg)
	if !changed || len(groups) != 3 || len(reg.Names) != 3 {
		t.Fatalf("unexpected merge: %#v %#v", groups, reg)
	}
	for _, g := range groups {
		if g.Name == "empty" && (g.Count != 0 || g.DeletedCount != 0 || len(g.Members) != 0) {
			t.Fatal("empty group gained members")
		}
		if g.Name == "populated" && (g.Count != 1 || len(g.Members) != 1 || g.Members[0].Key != "synthetic") {
			t.Fatal("authoritative membership lost")
		}
	}
	_, changed = mergeGroupRegistry(live, &reg)
	if changed {
		t.Fatal("unchanged catalog would be rewritten")
	}
}
func TestEmptyGroupNamesUseVaultValidation(t *testing.T) {
	for _, name := range []string{"", "未分组", "bad/name", "bad\\name", " spaced ", "bad\nname"} {
		if vault.ValidateGroupName(name) == nil {
			t.Fatalf("accepted invalid group %q", name)
		}
	}
	if err := vault.ValidateGroupName("Synthetic group"); err != nil {
		t.Fatal(err)
	}
}
