package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/shenjiecode/keyfolio/internal/fsutil"
	"github.com/shenjiecode/keyfolio/internal/platform"
)

// The sidecar never opens the vault, starts a browser, or reads launcher credentials.
func runStdio(args []string) error {
	config, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("foliochest mcp", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	device := flags.String("device-dir", filepath.Join(config, "keyfolio"), "本机配置目录")
	if err = flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("不支持额外参数")
	}
	*device, err = platform.Canonical(*device)
	if err != nil {
		return err
	}
	conn, reader, err := connectStdio(*device)
	if err != nil {
		return err
	}
	defer conn.Close()
	// stdin EOF terminates only this connection; the shared application keeps running.
	go func() { _, _ = io.Copy(conn, os.Stdin); _ = conn.Close() }()
	_, err = io.Copy(os.Stdout, reader)
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}
func connectStdio(device string) (net.Conn, *bufio.Reader, error) {
	fail := errors.New("无法连接本机 stdio MCP；请先启动资料匣，在设置中选择 stdio，并在网页解锁资料库")
	raw, err := fsutil.ReadBounded(filepath.Join(device, "mcp-stdio.json"), 4096)
	if err != nil {
		return nil, nil, fail
	}
	var info instanceInfo
	if json.Unmarshal(raw, &info) != nil || len(info.Token) != 48 {
		return nil, nil, fail
	}
	u, err := url.Parse(info.URL)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Port() == "" || !net.ParseIP(u.Hostname()).IsLoopback() {
		return nil, nil, fail
	}
	conn, err := net.DialTimeout("tcp", u.Host, 3*time.Second)
	if err != nil {
		return nil, nil, fail
	}
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	req, _ := http.NewRequest("GET", info.URL+"/internal/mcp/stdio", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "foliochest-stdio")
	req.Header.Set("X-FolioChest-Stdio", info.Token)
	if err = req.Write(conn); err != nil {
		conn.Close()
		return nil, nil, fail
	}
	reader := bufio.NewReader(conn)
	res, err := http.ReadResponse(reader, req)
	if err != nil || res.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()
		return nil, nil, fail
	}
	_ = conn.SetDeadline(time.Time{})
	return conn, reader, nil
}
