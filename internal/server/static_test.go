package server

import (
	"bytes"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/shenjiecode/keyfolio/web"
)

func TestReactDocumentNonceAndSingleEntrypoint(t *testing.T) {
	b := setupHTTP(t)
	b.app.static = web.Files()
	script, err := fs.ReadFile(b.app.static, "main.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"SYNTHETIC_", "exampleEntries", "previewOptions", "界面预览 · 示例资料"} {
		if bytes.Contains(script, []byte(marker)) {
			t.Fatalf("sample implementation in production bundle: %s", marker)
		}
	}
	seen := map[string]bool{}
	for _, path := range []string{"/", "/index.html", "/"} {
		r := httptest.NewRequest(http.MethodGet, b.url+path, nil)
		w := httptest.NewRecorder()
		b.app.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
		matches := regexp.MustCompile(`<meta name="style-nonce" content="([0-9a-f]{48})"`).FindStringSubmatch(w.Body.String())
		if len(matches) != 2 || seen[matches[1]] {
			t.Fatal("missing or reused document nonce")
		}
		seen[matches[1]] = true
		policy := w.Header().Get("Content-Security-Policy")
		if !strings.Contains(policy, "style-src 'self' 'nonce-"+matches[1]+"'") || !strings.Contains(policy, "script-src 'self';") || strings.Contains(policy, "unsafe-") {
			t.Fatal("CSP relaxed", policy)
		}
		if !strings.Contains(w.Body.String(), "/main.js") || !strings.Contains(w.Body.String(), "/styles.css") || strings.Contains(w.Body.String(), "{{STYLE_NONCE}}") {
			t.Fatal("invalid React document")
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("document may be cached")
		}
	}
	for _, path := range []string{"/style.css", "/main.css", "/preview/", "/api/not-present"} {
		r := httptest.NewRequest(http.MethodGet, b.url+path, nil)
		w := httptest.NewRecorder()
		b.app.ServeHTTP(w, r)
		if w.Code == 200 {
			t.Fatal("old or unknown resource is served", path)
		}
	}
	r := httptest.NewRequest(http.MethodHead, b.url+"/", nil)
	w := httptest.NewRecorder()
	b.app.ServeHTTP(w, r)
	if w.Code != 200 || w.Body.Len() != 0 {
		t.Fatal("invalid HEAD")
	}
	// Obsolete UI aliases must not bypass the one current CSRF header.
	req, _ := http.NewRequest(http.MethodPost, b.url+"/api/activity", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Keyfolio-CSRF", b.csrf)
	response, err := b.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("obsolete CSRF alias accepted", response.StatusCode)
	}
}
