package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shenjiecode/keyfolio/internal/fsutil"
)

// No invented destination on first launch. Existing installations retain their
// recorded location (even when missing), or their existing default directory.
func resolveVaultDirectory(explicit, device string) (string, error) {
	if explicit != "" {
		return filepath.Abs(explicit)
	}
	raw, err := fsutil.ReadBounded(filepath.Join(device, "location.json"), 64<<10)
	if err == nil {
		var location struct {
			Directory string `json:"directory"`
		}
		if json.Unmarshal(raw, &location) != nil || !filepath.IsAbs(location.Directory) {
			return "", errors.New("本机资料库位置记录无效，请用 --vault 指定正确目录")
		}
		return filepath.Clean(location.Directory), nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("读取本机资料库位置: %w", err)
	}
	previous := filepath.Join(device, "vault")
	if _, err := os.Lstat(previous); err == nil {
		return previous, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	return "", nil
}
