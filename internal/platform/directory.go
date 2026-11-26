package platform

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ncruces/zenity"
)

var ErrDirectoryCanceled = errors.New("已取消目录选择")
var ErrDirectoryUnavailable = errors.New("无法打开系统文件夹选择器，请重试或手动输入绝对路径")

type DirectoryPicker func(context.Context, string) (string, error)

// Only invoked for an explicit, CSRF-checked browser request, never at startup.
func PickDirectory(ctx context.Context, initial string) (string, error) {
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		return "", ErrDirectoryUnavailable
	}
	if runtime.GOOS == "linux" && (os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" || !zenity.IsAvailable()) {
		return "", ErrDirectoryUnavailable
	}
	return pickDirectory(ctx, initial, func(ctx context.Context, folder string) (string, error) {
		options := []zenity.Option{zenity.Directory(), zenity.Title("FolioChest · 选择资料库文件夹"), zenity.Context(ctx)}
		if folder != "" {
			options = append(options, zenity.Filename(folder+string(os.PathSeparator)))
		}
		return zenity.SelectFile(options...)
	})
}

func pickDirectory(ctx context.Context, initial string, choose DirectoryPicker) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if initial != "" && (!filepath.IsAbs(initial) || strings.ContainsRune(initial, 0) || len(initial) > 32768) {
		return "", errors.New("请输入有效的绝对目录路径")
	}
	// A not-yet-created destination can still open its nearest existing parent.
	folder := initial
	for folder != "" {
		if info, err := os.Stat(folder); err == nil && info.IsDir() {
			break
		}
		parent := filepath.Dir(folder)
		if parent == folder {
			folder = ""
			break
		}
		folder = parent
	}
	path, err := choose(ctx, folder)
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if errors.Is(err, zenity.ErrCanceled) {
		return "", ErrDirectoryCanceled
	}
	if err != nil {
		return "", ErrDirectoryUnavailable
	}
	if !filepath.IsAbs(path) || strings.ContainsRune(path, 0) || len(path) > 32768 {
		return "", errors.New("系统未返回有效的文件夹路径，请重新选择")
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "", errors.New("所选文件夹已不可用，请重新选择")
	}
	return filepath.Clean(path), nil
}
