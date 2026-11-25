//go:build !linux && !windows

package platform

import "errors"

func (s *NativeStartup) Status() (StartupStatus, error) { return StartupStatus{}, nil }
func (s *NativeStartup) Set(bool) error                 { return errors.New("当前平台暂不支持自动启动") }
