//go:build !linux && !windows

package platform

import "context"

type nativeCredentials struct{}

func NativeCredentials() Credentials                                  { return nativeCredentials{} }
func CredentialProviderName() string                                  { return "当前平台尚不支持设备解锁" }
func (nativeCredentials) Get(context.Context, string) ([]byte, error) { return nil, ErrUnavailable }
func (nativeCredentials) Set(context.Context, string, []byte) error   { return ErrUnavailable }
func (nativeCredentials) Delete(context.Context, string) error        { return ErrUnavailable }
