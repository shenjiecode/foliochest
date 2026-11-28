package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/shenjiecode/keyfolio/internal/access"
	"github.com/shenjiecode/keyfolio/internal/fsutil"
)

type instanceInfo struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}
type instance struct {
	file *os.File
	path string
	info instanceInfo
}

// The lock file is never unlinked: deleting it permits two owners on different inodes.
func acquireInstance(directory string) (*instance, bool, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, false, err
	}
	path := filepath.Join(directory, "launcher.lock")
	if st, err := os.Lstat(path); err == nil && !st.Mode().IsRegular() {
		return nil, false, errors.New("启动锁文件类型无效")
	} else if err != nil && !os.IsNotExist(err) {
		return nil, false, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, false, err
	}
	locked, err := lockInstance(f)
	if err != nil || !locked {
		f.Close()
		return nil, locked, err
	}
	return &instance{file: f, path: filepath.Join(directory, "launcher.json")}, true, nil
}
func (i *instance) publish(baseURL string) error {
	i.info = instanceInfo{URL: baseURL, Token: access.ID()}
	raw, err := json.Marshal(i.info)
	if err != nil {
		return err
	}
	return fsutil.WriteFile(i.path, raw)
}
func (i *instance) close() { _ = os.Remove(i.path); _ = i.file.Close() }

// validateLocalURL accepts only loopback http URLs with an explicit port and no path, query, fragment or user info.
func validateLocalURL(u *url.URL) bool {
	return u != nil && u.Scheme == "http" && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == "" && u.Port() != "" && net.ParseIP(u.Hostname()).IsLoopback()
}

func existingInstance(directory string) (string, error) {
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for attempt := 0; attempt < 30; attempt++ {
		raw, err := fsutil.ReadBounded(filepath.Join(directory, "launcher.json"), 4096)
		var info instanceInfo
		if err == nil && json.Unmarshal(raw, &info) == nil && len(info.Token) == 48 {
			u, e := url.Parse(info.URL)
			if e == nil && validateLocalURL(u) {
				req, _ := http.NewRequest("GET", info.URL+"/api/launcher", nil)
				req.Header.Set("X-FolioChest-Launcher", info.Token)
				response, e := client.Do(req)
				if e == nil {
					response.Body.Close()
					if response.StatusCode == http.StatusNoContent {
						return info.URL, nil
					}
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return "", fmt.Errorf("已有资料匣进程，但暂时无法确认其界面地址；请稍后重试，不要删除启动锁文件")
}
