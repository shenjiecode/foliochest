package server

import (
	"bytes"
	"io/fs"
	"net/http"

	"github.com/shenjiecode/keyfolio/internal/access"
)

func contentSecurityPolicy(nonce string) string {
	style := "style-src 'self'"
	if nonce != "" {
		style += " 'nonce-" + nonce + "'"
	}
	return "default-src 'self'; script-src 'self'; " + style + "; img-src 'self' blob:; connect-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"
}

// Only the document receives a fresh style nonce. Scripts remain external/self.
func (s *Server) servePage(w http.ResponseWriter, r *http.Request) {
	page, err := fs.ReadFile(s.static, "index.html")
	if err != nil {
		http.Error(w, "page unavailable", http.StatusInternalServerError)
		return
	}
	nonce := access.ID()
	w.Header().Set("Content-Security-Policy", contentSecurityPolicy(nonce))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	page = bytes.ReplaceAll(page, []byte("{{STYLE_NONCE}}"), []byte(nonce))
	if r.Method != http.MethodHead {
		_, _ = w.Write(page)
	}
}
