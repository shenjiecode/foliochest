//go:build windows

package platform

import (
	"context"
	"errors"
	"github.com/danieljoos/wincred"
)

type nativeCredentials struct{}

func NativeCredentials() Credentials { return nativeCredentials{} }
func CredentialProviderName() string { return "Windows Credential Manager（仅本机当前用户）" }
func (nativeCredentials) Get(ctx context.Context, id string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validCredentialID(id) {
		return nil, ErrUnavailable
	}
	credential, err := wincred.GetGenericCredential(ServiceName + "/" + id)
	if errors.Is(err, wincred.ErrElementNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, ErrUnavailable
	}
	if len(credential.CredentialBlob) > 4096 {
		clear(credential.CredentialBlob)
		return nil, ErrUnavailable
	}
	return credential.CredentialBlob, nil
}
func (nativeCredentials) Set(ctx context.Context, id string, value []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validCredentialID(id) || len(value) > 2560 {
		return ErrUnavailable
	}
	credential := wincred.NewGenericCredential(ServiceName + "/" + id)
	credential.Persist = wincred.PersistLocalMachine
	credential.UserName = "FolioChest"
	credential.CredentialBlob = value
	if err := credential.Write(); err != nil {
		return ErrUnavailable
	}
	return nil
}
func (nativeCredentials) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validCredentialID(id) {
		return ErrUnavailable
	}
	credential := wincred.NewGenericCredential(ServiceName + "/" + id)
	err := credential.Delete()
	if errors.Is(err, wincred.ErrElementNotFound) {
		return nil
	}
	if err != nil {
		return ErrUnavailable
	}
	return nil
}
