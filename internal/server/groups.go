package server

import (
	"github.com/shenjiecode/keyfolio/internal/vault"
	"net/http"
)

// Invoked only after webpage authentication and the shared CSRF/mutation guard.
func (s *Server) serveGroups(w http.ResponseWriter, r *http.Request) bool {
	var out any
	var err error
	switch r.URL.Path {
	case "/api/groups":
		if !method(w, r, "GET") {
			return true
		}
		out, err = s.Vault.Groups()
	case "/api/groups/move":
		if !method(w, r, "POST") {
			return true
		}
		var in vault.GroupMove
		if !decode(w, r, &in) {
			return true
		}
		out, err = s.Vault.MoveGroup(in)
	case "/api/groups/edit":
		if !method(w, r, "POST") {
			return true
		}
		var in vault.GroupEdit
		if !decode(w, r, &in) {
			return true
		}
		out, err = s.Vault.EditGroup(in)
	default:
		return false
	}
	if err != nil {
		apiError(w, err, 409)
	} else {
		if r.Method == "POST" {
			s.activity()
		}
		writeJSON(w, out)
	}
	return true
}
