package server

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type updateTransport func(*http.Request) (*http.Response, error)

func (f updateTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestUpdateVersionComparison(t *testing.T) {
	for _, tc := range []struct{ current, latest, want string }{
		{"0.6.2", "v0.6.2", "current"}, {"0.6.2", "v0.6.3", "available"},
		{"0.6.2-rc.1", "v0.6.2", "available"}, {"0.6.3-dev", "v0.6.2", "ahead"},
		{"0.9.0", "v0.10.0", "available"}, {"unknown", "v0.6.2", "unknown"},
	} {
		if got := updateStatus(tc.current, tc.latest); got != tc.want {
			t.Errorf("%s vs %s: %s", tc.current, tc.latest, got)
		}
	}
}
func TestLatestReleaseUsesFixedPublicRequest(t *testing.T) {
	client := &http.Client{Transport: updateTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != releaseAPI || r.Method != "GET" || r.Body != nil || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Fatal("unsafe update request")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"tag_name":"v0.6.3","html_url":"https://evil.example/","body":"untrusted release notes"}`)), Header: make(http.Header)}, nil
	})}
	result, err := latestRelease(context.Background(), client, "0.6.2")
	if err != nil || result.Status != "available" || result.URL != "https://github.com/shenjiecode/foliochest/releases/tag/v0.6.3" {
		t.Fatal(result, err)
	}
}
func TestLatestReleaseRejectsInvalidResponses(t *testing.T) {
	for _, body := range []string{`{`, `{"tag_name":"v0.6.3-rc.1"}`, `{"tag_name":"v0.6.3","draft":true}`, `{"tag_name":"v0.6.3","prerelease":true}`, `{"tag_name":"../../evil"}`, strings.Repeat("x", (1<<20)+1)} {
		client := &http.Client{Transport: updateTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		if _, err := latestRelease(context.Background(), client, "0.6.2"); err == nil {
			t.Fatal("accepted invalid release")
		}
	}
	for _, code := range []int{302, 403, 404, 429, 500} {
		client := &http.Client{Transport: updateTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(""))}, nil
		})}
		if _, err := latestRelease(context.Background(), client, "0.6.2"); err == nil {
			t.Fatal("accepted failed request", code)
		}
	}
}
func TestCheckUpdateRequiresHumanAndExplicitAction(t *testing.T) {
	b := setupHTTP(t)
	for _, tc := range []struct {
		method, csrf string
		body         any
		want         int
	}{
		{"GET", "", nil, 405}, {"POST", "wrong", map[string]bool{"confirmed": true}, 403},
		{"POST", b.csrf, map[string]bool{"confirmed": false}, 400},
	} {
		code, _ := b.request(tc.method, "/api/app/check-update", tc.body, tc.csrf)
		if code != tc.want {
			t.Fatal(code, tc.want)
		}
	}
	fresh := openBrowser(t, b.app, b.url, nil)
	if code, _ := fresh.request("POST", "/api/app/check-update", map[string]bool{"confirmed": true}, fresh.csrf); code != 401 {
		t.Fatal(code)
	}
}
