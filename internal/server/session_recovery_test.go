package server

import (
	"encoding/json"
	"testing"
)

func TestStateRenewsLostSessionWithoutInheritingWebAuthentication(t *testing.T) {
	for _, remember := range []bool{false, true} {
		b, _ := setupDeviceHTTP(t)
		enrollBrowser(b, remember)
		b.app.mu.Lock()
		b.app.sessions = map[string]*webSession{}
		b.app.mu.Unlock()
		code, raw := b.request("GET", "/api/state", nil, "")
		if code != 200 {
			t.Fatalf("session renewal: %d %s", code, raw)
		}
		var state struct {
			CSRF          string
			Authenticated bool
		}
		if err := json.Unmarshal(raw, &state); err != nil {
			t.Fatal(err)
		}
		if state.CSRF == "" || state.CSRF == b.csrf || state.Authenticated != remember {
			t.Fatal("unsafe session renewal", remember, state)
		}
		b.csrf = state.CSRF
		code, _ = b.request("GET", "/api/entries", nil, "")
		if (code == 200) != remember {
			t.Fatal("new session inherited authorization without remembered browser", code)
		}
	}
}
