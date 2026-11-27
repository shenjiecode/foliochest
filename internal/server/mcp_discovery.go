package server

import (
	"errors"
	"slices"
	"sort"

	"github.com/shenjiecode/keyfolio/internal/vault"
)

// Shadow Description so callers can omit it without changing the shared web DTO.
type discoveredEntry struct {
	vault.Summary
	Description *string `json:"description,omitempty"`
}

func (s *Server) discoverEntries(args discoverArgs) (discoverResult, error) {
	out := discoverResult{Entries: []discoveredEntry{}}
	if args.Offset < 0 || args.Limit < 0 || args.Limit > 200 || len(args.Keys) > 100 {
		return out, errors.New("offset must be nonnegative; limit <= 200; keys <= 100")
	}
	limit := args.Limit
	if limit == 0 {
		limit = 100
	}
	rows, err := s.Vault.List(vault.ListOptions{Query: args.Query, Group: args.Group})
	if err != nil {
		return out, err
	}
	for _, row := range rows {
		if len(args.Keys) > 0 && !slices.Contains(args.Keys, row.Key) {
			continue
		}
		index := out.Total
		out.Total++
		if index < args.Offset || len(out.Entries) >= limit {
			continue
		}
		item := discoveredEntry{Summary: row}
		if args.IncludeDescription == nil || *args.IncludeDescription {
			description := row.Description
			item.Description = &description
		}
		out.Entries = append(out.Entries, item)
	}
	if args.Offset < out.Total && len(out.Entries) < out.Total-args.Offset {
		next := args.Offset + len(out.Entries)
		out.NextOffset = &next
	}
	return out, nil
}

type listGroupsArgs struct {
	IncludeEmpty bool `json:"include_empty,omitempty" jsonschema:"Include device-local empty groups (not synced), default false"`
}
type mcpGroup struct {
	Name      string `json:"name"`
	Count     int    `json:"count"`
	LocalOnly bool   `json:"local_only,omitempty"`
}
type listGroupsResult struct {
	Groups []mcpGroup `json:"groups"`
}

func (s *Server) listMCPGroups(args listGroupsArgs) (listGroupsResult, error) {
	s.groupMu.Lock()
	defer s.groupMu.Unlock()
	out := listGroupsResult{Groups: []mcpGroup{}}
	gen := s.Vault.Generation()
	rows, err := s.Vault.List(vault.ListOptions{})
	if err != nil {
		return out, err
	}
	counts := map[string]int{}
	for _, row := range rows {
		groups := row.CandidateGroups
		if len(groups) == 0 {
			groups = []string{row.Group}
		}
		for _, name := range groups {
			counts[name]++
		}
	}
	if args.IncludeEmpty && s.Vault.Status().Format >= 3 {
		_, reg, _, err := s.loadGroupRegistry()
		if err != nil {
			return out, err
		}
		for _, name := range reg.Names {
			if _, ok := counts[name]; !ok {
				counts[name] = 0
			}
		}
	}
	for name, count := range counts {
		out.Groups = append(out.Groups, mcpGroup{Name: name, Count: count, LocalOnly: count == 0})
	}
	sort.Slice(out.Groups, func(i, j int) bool { return out.Groups[i].Name < out.Groups[j].Name })
	if gen != s.Vault.Generation() || s.Vault.Locked() {
		return listGroupsResult{}, vault.ErrLocked
	}
	return out, nil
}
