package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shenjiecode/keyfolio/internal/access"
	"github.com/shenjiecode/keyfolio/internal/fsutil"
	"github.com/shenjiecode/keyfolio/internal/vault"
)

type mcpTransportState struct {
	mu          sync.Mutex
	mode        string
	token       string
	connections map[net.Conn]string
}

func (s *Server) initMCPTransport() error {
	state := &mcpTransportState{mode: "stdio", token: access.ID(), connections: map[net.Conn]string{}}
	raw, err := fsutil.ReadBounded(filepath.Join(s.deviceDir, "mcp-settings.json"), 4096)
	if err == nil {
		var saved struct {
			Mode string `json:"mode"`
		}
		if json.Unmarshal(raw, &saved) != nil || (saved.Mode != "stdio" && saved.Mode != "http") {
			return errors.New("MCP 接入设置损坏")
		}
		state.mode = saved.Mode
	} else if !os.IsNotExist(err) {
		return err
	}
	s.mcpTransport = state
	return nil
}

// Publish only at application startup, never from tests. This capability cannot
// authenticate webpage requests, unlock a vault, or manage settings.
func (s *Server) PublishStdioBridge() error {
	raw, _ := json.Marshal(map[string]string{"url": s.baseURL, "token": s.mcpTransport.token})
	return fsutil.WriteFile(filepath.Join(s.deviceDir, "mcp-stdio.json"), raw)
}
func (s *Server) CloseStdioBridge() {
	s.mcpTransport.mu.Lock()
	defer s.mcpTransport.mu.Unlock()
	for conn := range s.mcpTransport.connections {
		_ = conn.Close()
	}
	_ = os.Remove(filepath.Join(s.deviceDir, "mcp-stdio.json"))
}
func (s *Server) mcpMode() string {
	s.mcpTransport.mu.Lock()
	defer s.mcpTransport.mu.Unlock()
	return s.mcpTransport.mode
}
func (s *Server) mcpSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var in struct {
			Mode      string `json:"mode"`
			Confirmed bool   `json:"confirmed"`
		}
		if !decode(w, r, &in) {
			return
		}
		if !in.Confirmed || (in.Mode != "stdio" && in.Mode != "http") {
			apiError(w, errors.New("请选择并确认 MCP 接入方式"), 400)
			return
		}
		st := s.mcpTransport
		st.mu.Lock()
		raw, _ := json.Marshal(map[string]string{"mode": in.Mode})
		if err := fsutil.WriteFile(filepath.Join(s.deviceDir, "mcp-settings.json"), raw); err != nil {
			st.mu.Unlock()
			apiError(w, err, 500)
			return
		}
		changed := st.mode != in.Mode
		st.mode = in.Mode
		if changed {
			for conn := range st.connections {
				_ = conn.Close()
			}
		}
		st.mu.Unlock()
		if changed {
			// Invalidate queued and buffered results as well as the old webpage generation.
			s.Access.Lock()
			s.mu.Lock()
			s.mcpHandlers = map[string]http.Handler{}
			s.mu.Unlock()
		}
	} else if !method(w, r, "GET") {
		return
	}
	executable, _ := os.Executable()
	writeJSON(w, map[string]any{"mode": s.mcpMode(), "command": executable, "args": []string{"mcp", "--device-dir", s.deviceDir}, "url": s.baseURL + "/mcp"})
}

type bridgeReader struct {
	io.Reader
	io.Closer
}

func (s *Server) serveStdioBridge(w http.ResponseWriter, r *http.Request) {
	st := s.mcpTransport
	if r.Method != "GET" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-FolioChest-Stdio")), []byte(st.token)) != 1 {
		apiError(w, vault.ErrDenied, 401)
		return
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "foliochest-stdio") || !strings.EqualFold(r.Header.Get("Connection"), "upgrade") {
		apiError(w, errors.New("stdio bridge requires upgrade"), 400)
		return
	}
	st.mu.Lock()
	if st.mode != "stdio" || len(st.connections) >= 32 {
		st.mu.Unlock()
		apiError(w, errors.New("stdio 未启用或连接过多"), 409)
		return
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		st.mu.Unlock()
		apiError(w, errors.New("stdio bridge unavailable"), 500)
		return
	}
	conn, rw, err := hijacker.Hijack()
	if err != nil {
		st.mu.Unlock()
		return
	}
	client := Client{ID: "stdio-" + access.ID(), Name: "本机 stdio"}
	st.connections[conn] = client.ID
	st.mu.Unlock()
	defer func() {
		_ = conn.Close()
		s.Access.RevokeClient(client.ID)
		st.mu.Lock()
		delete(st.connections, conn)
		st.mu.Unlock()
	}()
	if _, err = fmt.Fprint(rw, "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: foliochest-stdio\r\n\r\n"); err != nil {
		return
	}
	if err = rw.Flush(); err != nil {
		return
	}
	// Raw bidirectional MCP preserves client capabilities and harness elicitation.
	session, err := s.mcpServer(client).Connect(context.Background(), &mcp.IOTransport{Reader: bridgeReader{rw.Reader, conn}, Writer: conn, MaxLineLength: vault.MaxFileBytes}, nil)
	if err != nil {
		return
	}
	defer session.Close()
	_ = session.Wait()
}
