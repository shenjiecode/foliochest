package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/shenjiecode/keyfolio/internal/fsutil"
	"github.com/shenjiecode/keyfolio/internal/platform"
	"github.com/shenjiecode/keyfolio/internal/updater"
)

type updateManifest struct {
	Device   string          `json:"device"`
	Original string          `json:"original"`
	Package  updater.Package `json:"package"`
}

// Copy this executable outside the install directory so Windows can replace it.
// No shell, silent switches, passwords or elevation commands are used.
func prepareUpdate(device string, pkg updater.Package) error {
	if err := pkg.Verify(); err != nil {
		return err
	}
	original, err := os.Executable()
	if err != nil {
		return err
	}
	stage := filepath.Dir(pkg.Path)
	helper := filepath.Join(stage, "foliochest-update-helper")
	if runtime.GOOS == "windows" {
		helper += ".exe"
	}
	in, err := os.Open(original)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(helper, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		return errors.New("无法准备更新助手，请重新下载后重试")
	}
	_, copyErr := io.Copy(out, in)
	syncErr := out.Sync()
	closeErr := out.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil {
		_ = os.Remove(helper)
		return errors.New("无法写入更新助手")
	}
	manifest := filepath.Join(stage, "install.json")
	raw, _ := json.Marshal(updateManifest{Device: device, Original: original, Package: pkg})
	if err = fsutil.WriteFile(manifest, raw); err != nil {
		_ = os.Remove(helper)
		return err
	}
	log, err := os.OpenFile(filepath.Join(stage, "installer.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		_ = os.Remove(helper)
		return err
	}
	defer log.Close()
	cmd := exec.Command(helper, "internal-install-update", manifest)
	cmd.Stdout = log
	cmd.Stderr = log
	detachUpdate(cmd)
	if err = cmd.Start(); err != nil {
		_ = os.Remove(helper)
		return errors.New("无法启动更新助手，资料库保持锁定；可手动安装已下载文件")
	}
	_ = cmd.Process.Release()
	return nil
}

func runUpdateHelper(manifest string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return executeUpdatePlan(manifest, executable, launchInstaller, 45*time.Second)
}

func executeUpdatePlan(manifest, executable string, launch func(string) error, timeout time.Duration) error {
	raw, err := fsutil.ReadBounded(manifest, 16<<10)
	if err != nil {
		return err
	}
	var plan updateManifest
	if json.Unmarshal(raw, &plan) != nil {
		return errors.New("更新任务损坏")
	}
	stage := filepath.Dir(executable)
	if filepath.Dir(manifest) != stage || filepath.Dir(plan.Package.Path) != stage || !filepath.IsAbs(plan.Device) || !filepath.IsAbs(plan.Original) {
		return errors.New("更新任务路径无效")
	}
	name, err := updater.Filename(plan.Package.Version, runtime.GOOS, runtime.GOARCH, packageKind(plan.Package.Name))
	if err != nil || name != plan.Package.Name {
		return errors.New("安装包不适用于此系统或架构")
	}
	failure := func(err error) error {
		msg := "更新未完成：" + err.Error() + "。安装包已保留，可手动安装：" + plan.Package.Path
		_ = fsutil.WriteFile(filepath.Join(stage, "result.txt"), []byte(msg))
		return errors.New(msg)
	}
	// Wait for the actual instance lock, not a fixed sleep or a guess about the PID.
	lock, err := os.OpenFile(filepath.Join(plan.Device, "launcher.lock"), os.O_RDWR, 0600)
	if err != nil {
		return failure(err)
	}
	defer lock.Close()
	deadline := time.Now().Add(timeout)
	acquired := false
	for time.Now().Before(deadline) {
		acquired, err = lockInstance(lock)
		if err != nil {
			return failure(err)
		}
		if acquired {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !acquired {
		return failure(errors.New("旧程序没有及时退出，请关闭后重试"))
	}
	// Stdio sidecars also run the original executable. Windows installers must wait
	// until those inherited pipes close and the processes release the image.
	for !updateExecutableFree(plan.Original) {
		if time.Now().After(deadline) {
			return failure(errors.New("旧程序或 MCP 子进程仍在运行"))
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err = plan.Package.Verify(); err != nil {
		return failure(err)
	}
	if err = launch(plan.Package.Path); err != nil {
		return failure(err)
	}
	_ = fsutil.WriteFile(filepath.Join(stage, "result.txt"), []byte("已交给安装器；这不代表安装完成。完成后请从应用菜单重新启动资料匣。"))
	return nil
}
func packageKind(name string) string {
	switch filepath.Ext(name) {
	case ".exe":
		return "exe"
	case ".rpm":
		return "rpm"
	case ".deb":
		return "deb"
	}
	return ""
}
func installerError(err error) error { return fmt.Errorf("系统安装器无法打开：%w", err) }

// Keep platform error reporting out of tests that exercise only handoff preparation.
func reportUpdateFailure(err error) { platform.ShowLaunchError(err.Error()) }
