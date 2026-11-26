//go:build !linux && !darwin && !windows

package app

import (
	"errors"
	"os"
)

func lockInstance(*os.File) (bool, error) {
	return false, errors.New("当前平台尚不支持单实例启动")
}
