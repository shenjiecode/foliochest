package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
)

const ServiceName = "io.github.shenjiecode.keyfolio"

var ErrUnavailable = errors.New("系统凭据服务不可用或已锁定，请先在系统中解锁凭据库，或使用主密码")
var ErrNotFound = errors.New("系统中没有这份设备凭据")

type Credentials interface {
	Get(context.Context, string) ([]byte, error)
	Set(context.Context, string, []byte) error
	Delete(context.Context, string) error
}
type StartupStatus struct {
	Supported bool `json:"supported"`
	Enabled   bool `json:"enabled"`
	Current   bool `json:"current"`
}
type Startup interface {
	Status() (StartupStatus, error)
	Set(bool) error
}
type NativeStartup struct{ DeviceDir, Executable, Address, ConfigDir string }

func NewStartup(device, address string) (Startup, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return &NativeStartup{DeviceDir: device, Executable: exe, Address: address, ConfigDir: config}, nil
}
func DeviceID(dir string) string {
	abs, _ := filepath.Abs(dir)
	if runtime.GOOS == "windows" {
		abs = strings.ToLower(abs)
	}
	hash := sha256.Sum256([]byte(filepath.Clean(abs)))
	return hex.EncodeToString(hash[:12])
}
func validCredentialID(id string) bool {
	if len(id) < 32 || len(id) > 96 {
		return false
	}
	for _, c := range id {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}
func (s *NativeStartup) args() ([]string, error) {
	if !filepath.IsAbs(s.Executable) || !filepath.IsAbs(s.DeviceDir) {
		return nil, errors.New("启动程序和本机配置目录必须使用绝对路径")
	}
	for _, value := range []string{s.Executable, s.DeviceDir, s.Address} {
		if strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return nil, errors.New("启动参数包含不支持的控制字符")
		}
	}
	for _, part := range strings.Split(filepath.ToSlash(s.Executable), "/") {
		if strings.HasPrefix(part, "go-build") {
			return nil, errors.New("请先构建并运行固定位置的程序，再开启自动启动；不能注册 go run 临时程序")
		}
	}
	return []string{s.Executable, "--device-dir", s.DeviceDir, "--listen", s.Address}, nil
}
func Canonical(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(absolute)
	if parent == absolute {
		return "", err
	}
	resolved, err = Canonical(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(absolute)), nil
}
func SeparateDirectories(vaultDir, deviceDir string) error {
	root, err := Canonical(vaultDir)
	if err != nil {
		return err
	}
	local, err := Canonical(deviceDir)
	if err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		root = strings.ToLower(root)
		local = strings.ToLower(local)
	}
	// Windows vaults commonly live on another drive than the user profile.
	if !strings.EqualFold(filepath.VolumeName(root), filepath.VolumeName(local)) {
		return nil
	}
	relative, err := filepath.Rel(root, local)
	if err != nil {
		return err
	}
	if relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("本机配置目录不能放在资料库目录中，请将设备状态留在同步目录之外")
	}
	return nil
}
