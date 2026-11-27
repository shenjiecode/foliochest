// Package updater stages verified installers; it never changes vault files or installs packages.
package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const MaxPackageBytes int64 = 256 << 20

var stableVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type Package struct {
	Version string `json:"version"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
}

func Filename(version, system, arch, kind string) (string, error) {
	if !stableVersion.MatchString(version) {
		return "", errors.New("仅支持正式版本安装包")
	}
	if system == "windows" && (arch == "amd64" || arch == "386") && kind == "exe" {
		return fmt.Sprintf("foliochest-%s-windows-%s-setup.exe", version, arch), nil
	}
	if system == "linux" && (arch == "amd64" || arch == "arm64") && (kind == "rpm" || kind == "deb") {
		return fmt.Sprintf("foliochest-%s-linux-%s.%s", version, arch, kind), nil
	}
	return "", errors.New("当前系统或架构不支持引导更新，请使用下载页")
}

// DetectLinuxKind reads ID/ID_LIKE values only, never sources shell content.
func DetectLinuxKind(raw string) string {
	fields := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && (key == "ID" || key == "ID_LIKE") {
			fields[key] = strings.Trim(value, "\"'")
		}
	}
	for _, id := range strings.Fields(fields["ID"] + " " + fields["ID_LIKE"]) {
		switch id {
		case "fedora", "rhel", "centos", "rocky", "almalinux", "opensuse", "suse":
			return "rpm"
		case "debian", "ubuntu":
			return "deb"
		}
	}
	return ""
}

func allowedURL(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	switch u.Hostname() {
	case "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
		return true
	}
	return false
}
func NewClient() *http.Client {
	return &http.Client{Timeout: 100 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !allowedURL(r.URL) {
			return errors.New("拒绝非 GitHub HTTPS 下载跳转")
		}
		return nil
	}}
}
func fetch(ctx context.Context, client *http.Client, address string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", address, nil)
	if err != nil || !allowedURL(req.URL) {
		return nil, errors.New("无效更新地址")
	}
	req.Header.Set("User-Agent", "FolioChest-update-download")
	res, err := client.Do(req)
	if err != nil {
		return nil, errors.New("无法下载 GitHub 安装包，请检查网络后重试")
	}
	if res.StatusCode != 200 {
		res.Body.Close()
		return nil, fmt.Errorf("GitHub 下载失败（HTTP %d）", res.StatusCode)
	}
	return res, nil
}
func checksum(raw []byte, name string) (string, error) {
	found := ""
	for _, line := range strings.Split(string(raw), "\n") {
		parts := strings.Fields(line)
		if len(parts) != 2 || strings.TrimPrefix(parts[1], "*") != name {
			continue
		}
		digest, err := hex.DecodeString(parts[0])
		if err != nil || len(digest) != sha256.Size || found != "" {
			return "", errors.New("安装包校验清单无效或重复")
		}
		found = strings.ToLower(parts[0])
	}
	if found == "" {
		return "", errors.New("校验清单缺少所选安装包")
	}
	return found, nil
}

// Download accepts a locally constructed asset name, never a remote-supplied URL.
func Download(ctx context.Context, client *http.Client, cache, version, system, arch, kind string, expectedSize int64) (Package, error) {
	name, err := Filename(version, system, arch, kind)
	if err != nil {
		return Package{}, err
	}
	if expectedSize <= 0 || expectedSize > MaxPackageBytes {
		return Package{}, errors.New("安装包大小无效或超过 256 MiB")
	}
	base := "https://github.com/shenjiecode/foliochest/releases/download/v" + version + "/"
	res, err := fetch(ctx, client, base+"SHA256SUMS-"+version+".txt")
	if err != nil {
		return Package{}, err
	}
	sums, err := io.ReadAll(io.LimitReader(res.Body, (64<<10)+1))
	res.Body.Close()
	if err != nil || len(sums) > 64<<10 {
		return Package{}, errors.New("无法读取校验清单")
	}
	hash, err := checksum(sums, name)
	if err != nil {
		return Package{}, err
	}
	if err = os.MkdirAll(cache, 0700); err != nil {
		return Package{}, err
	}
	info, err := os.Lstat(cache)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return Package{}, errors.New("更新缓存目录无效")
	}
	stage, err := os.MkdirTemp(cache, "update-")
	if err != nil {
		return Package{}, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(stage)
		}
	}()
	res, err = fetch(ctx, client, base+name)
	if err != nil {
		return Package{}, err
	}
	defer res.Body.Close()
	if res.ContentLength > MaxPackageBytes || (res.ContentLength >= 0 && res.ContentLength != expectedSize) {
		return Package{}, errors.New("安装包大小与发布信息不一致")
	}
	file, err := os.OpenFile(filepath.Join(stage, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return Package{}, err
	}
	h := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(file, h), io.LimitReader(res.Body, expectedSize+1))
	syncErr := file.Sync()
	closeErr := file.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil || size != expectedSize || hex.EncodeToString(h.Sum(nil)) != hash {
		return Package{}, errors.New("安装包下载不完整或 SHA-256 不匹配，已丢弃，请重试")
	}
	keep = true
	return Package{Version: version, Name: name, Path: filepath.Join(stage, name), SHA256: hash, Size: size}, nil
}
func (p Package) Verify() error {
	if !filepath.IsAbs(p.Path) || filepath.Base(p.Path) != p.Name || p.Size <= 0 || p.Size > MaxPackageBytes {
		return errors.New("安装包路径或大小无效")
	}
	info, err := os.Lstat(p.Path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != p.Size {
		return errors.New("安装包已变化，请重新下载")
	}
	f, err := os.Open(p.Path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	size, err := io.Copy(h, io.LimitReader(f, MaxPackageBytes+1))
	if err != nil || size != p.Size || hex.EncodeToString(h.Sum(nil)) != p.SHA256 {
		return errors.New("安装包校验失败，拒绝启动安装器")
	}
	return nil
}
