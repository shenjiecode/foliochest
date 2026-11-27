package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestTargetsAndRedirects(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{{"ID=fedora", "rpm"}, {"ID=linuxmint\nID_LIKE=\"ubuntu debian\"", "deb"}, {"ID=arch", ""}} {
		if got := DetectLinuxKind(tc.raw); got != tc.want {
			t.Fatal(got, tc.want)
		}
	}
	if _, err := Filename("../evil", "windows", "amd64", "exe"); err == nil {
		t.Fatal("invalid version")
	}
	if _, err := Filename("1.0.0", "linux", "386", "rpm"); err == nil {
		t.Fatal("unsupported arch")
	}
	for _, address := range []string{"http://github.com/x", "https://evil.example/x", "https://github.com.evil.example/x", "https://user@github.com/x", "https://github.com:444/x"} {
		u, _ := url.Parse(address)
		if allowedURL(u) {
			t.Fatal(address)
		}
		if err := NewClient().CheckRedirect(&http.Request{URL: u}, nil); err == nil {
			t.Fatal("redirect allowed", address)
		}
	}
	u, _ := url.Parse("https://release-assets.githubusercontent.com/example")
	if !allowedURL(u) {
		t.Fatal("blocked release CDN")
	}
}
func TestDownloadAndTamper(t *testing.T) {
	raw := []byte("synthetic installer, never execute")
	hash := sha256.Sum256(raw)
	sum := hex.EncodeToString(hash[:])
	name, _ := Filename("1.2.3", "linux", "amd64", "rpm")
	cache := t.TempDir()
	client := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.Body != nil {
			t.Fatal("credential leak")
		}
		body := string(raw)
		if strings.HasSuffix(r.URL.Path, ".txt") {
			body = sum + "  " + name + "\n"
		}
		return &http.Response{StatusCode: 200, ContentLength: int64(len(body)), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	p, err := Download(context.Background(), client, cache, "1.2.3", "linux", "amd64", "rpm", int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Verify(); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(p.Path, []byte(strings.Repeat("x", len(raw))), 0600); err != nil {
		t.Fatal(err)
	}
	if err = p.Verify(); err == nil {
		t.Fatal("tamper accepted")
	}
	if err = os.Remove(p.Path); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target")
	if err = os.WriteFile(target, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(target, p.Path); err == nil {
		if err = p.Verify(); err == nil {
			t.Fatal("symlink accepted")
		}
	}
}
func TestDownloadRejectsChecksumAndTruncation(t *testing.T) {
	for _, tc := range []struct {
		sum, body string
		size      int64
	}{
		{strings.Repeat("0", 64), "synthetic", 9},
		{"not-a-digest", "synthetic", 9},
		{strings.Repeat("0", 64), "short", 10},
	} {
		cache := t.TempDir()
		name, _ := Filename("1.0.0", "linux", "amd64", "deb")
		client := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
			body := tc.body
			if strings.HasSuffix(r.URL.Path, ".txt") {
				body = tc.sum + "  " + name
			}
			return &http.Response{StatusCode: 200, ContentLength: -1, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		if _, err := Download(context.Background(), client, cache, "1.0.0", "linux", "amd64", "deb", tc.size); err == nil {
			t.Fatal("bad package accepted")
		}
		entries, _ := os.ReadDir(cache)
		if len(entries) != 0 {
			t.Fatal("failed download retained staging files")
		}
	}
}
func TestDuplicateChecksumRejected(t *testing.T) {
	line := strings.Repeat("0", 64) + "  synthetic.rpm\n"
	if _, err := checksum([]byte(line+line), "synthetic.rpm"); err == nil {
		t.Fatal("duplicate accepted")
	}
}
