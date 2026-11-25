package vault

import "sort"

// Identity heads, not historical names, determine the current namespace.
// A divergent rename reserves EVERY candidate name until explicit resolution.
func (v *Vault) indexVersions(groups map[string][]*version, versions map[string]*version, invalid map[string]bool, ready func(*batch) bool) {
	identities := map[string]*item{}
	for id, all := range groups {
		parents := map[string]bool{}
		state := "ready"
		for _, x := range all {
			if !ready(x.batch) {
				state = "incomplete"
			}
			for _, p := range x.meta.Parents {
				parents[p] = true
				if pv, ok := versions[p]; !ok || pv.meta.EntryID != id {
					state = "incomplete"
				}
			}
		}
		heads := []*version{}
		for _, x := range all {
			if !parents[x.meta.Revision] {
				heads = append(heads, x)
			}
		}
		sort.Slice(heads, func(i, j int) bool { return heads[i].meta.Revision < heads[j].meta.Revision })
		if len(heads) == 0 {
			heads = all[:1]
			state = "conflict"
		}
		if len(heads) > 1 {
			state = "conflict"
		}
		if invalid[id] {
			state = "invalid"
		}
		if state == "ready" && heads[0].meta.Deleted {
			state = "deleted"
		}
		identities[id] = &item{heads: heads, state: state, count: len(all)}
	}
	// Keep missing-head knowledge by stable identity, including deleted/renamed
	// heads. A disappearing tombstone must not expose its live ancestor again.
	for id, old := range v.identities {
		missing := []*version{}
		for _, head := range old.heads {
			if _, ok := versions[head.meta.Revision]; !ok {
				missing = append(missing, head)
			}
		}
		if len(missing) == 0 {
			continue
		}
		current := identities[id]
		if current == nil {
			current = &item{count: old.count}
			identities[id] = current
		}
		current.state = "missing"
		current.heads = append(current.heads, missing...)
	}
	next := map[string]*item{}
	for _, it := range identities {
		names := map[string]bool{}
		for _, head := range it.heads {
			names[head.meta.Key] = true
		}
		for key := range names {
			if previous := next[key]; previous != nil {
				previous.state = "conflict"
				previous.heads = append(previous.heads, it.heads...)
				previous.count += it.count
			} else {
				next[key] = &item{heads: append([]*version{}, it.heads...), state: it.state, count: it.count}
			}
		}
	}
	for _, it := range next {
		sort.Slice(it.heads, func(i, j int) bool { return it.heads[i].meta.Revision < it.heads[j].meta.Revision })
	}
	v.identities = identities
	v.items = next
}
