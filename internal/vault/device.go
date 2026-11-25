package vault

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type Binding struct {
	VaultID    string `json:"vault_id"`
	ConfigHash string `json:"config_hash"`
}
type deviceCredential struct {
	Version int `json:"version"`
	Binding
	Key   []byte `json:"key"`
	Proof []byte `json:"proof"`
}

func binding(c config, raw []byte) Binding {
	hash := sha256.Sum256(raw)
	return Binding{VaultID: c.VaultID, ConfigHash: hex.EncodeToString(hash[:])}
}
func deviceProof(key []byte, b Binding) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte("keyfolio/device/v1\x00" + b.VaultID + "\x00" + b.ConfigHash))
	return h.Sum(nil)
}
func (v *Vault) Binding() (Binding, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.key) == 0 {
		return Binding{}, ErrLocked
	}
	if err := v.scan(); err != nil {
		return Binding{}, err
	}
	return binding(v.cfg, v.configBytes), nil
}
func (v *Vault) verifyPassword(password string) error {
	if len(v.key) == 0 {
		return ErrLocked
	}
	if err := v.scan(); err != nil {
		return err
	}
	key, err := v.cfg.unlock(password)
	if err != nil {
		return err
	}
	defer clear(key)
	if subtle.ConstantTimeCompare(key, v.key) != 1 {
		return errors.New("当前解锁密钥与主密码不一致，请锁定后重新打开")
	}
	return nil
}
func (v *Vault) VerifyPassword(password string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.verifyPassword(password)
}

// DeviceCredential must ONLY be persisted through an OS credential provider.
func (v *Vault) DeviceCredential(password string) (Binding, []byte, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.verifyPassword(password); err != nil {
		return Binding{}, nil, err
	}
	b := binding(v.cfg, v.configBytes)
	raw, err := json.Marshal(deviceCredential{Version: 1, Binding: b, Key: v.key, Proof: deviceProof(v.key, b)})
	return b, raw, err
}

// UnlockDevice is used only once during startup, never after idle/manual locking.
func (v *Vault) UnlockDevice(raw []byte) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.key) != 0 {
		return errors.New("资料库已解锁")
	}
	if len(raw) > 4096 {
		return errors.New("设备凭据不受支持")
	}
	var credential deviceCredential
	defer func() { clear(credential.Key) }()
	if err := decodeStrict(raw, &credential); err != nil {
		return errors.New("设备凭据损坏")
	}
	if credential.Version != 1 || len(credential.Key) != 32 || !hmac.Equal(credential.Proof, deviceProof(credential.Key, credential.Binding)) {
		return errors.New("设备凭据校验失败")
	}
	data, err := readBounded(filepath.Join(v.root, "vault.json"), 16<<10)
	if err != nil {
		return err
	}
	var c config
	if err = decodeStrict(data, &c); err != nil || !c.valid() {
		return ErrChanged
	}
	if binding(c, data) != credential.Binding {
		return errors.New("资料库配置已变化，请使用主密码重新解锁并授权这台设备")
	}
	v.lock()
	v.cfg = c
	v.configBytes = data
	v.key = bytes.Clone(credential.Key)
	if err = v.scan(); err != nil {
		v.lock()
		return err
	}
	return nil
}

// ChangePassword rewraps the SAME data key. Old backups and copied data keys
// remain usable. It does not claim to revoke an offline device cryptographically.
func (v *Vault) ChangePassword(current, next string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.verifyPassword(current); err != nil {
		return err
	}
	if current == next {
		return errors.New("新主密码不能与当前主密码相同")
	}
	if err := v.completeSnapshot(); err != nil {
		return err
	}
	c, err := wrapConfig(next, v.cfg.VaultID, v.key)
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(v.root, "vault.json")
	f, err := os.CreateTemp(v.root, ".pending-config-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(raw)
	}
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err == nil {
		err = ce
	}
	if err != nil {
		return err
	}
	before, err := readBounded(path, 16<<10)
	if err != nil || !bytes.Equal(before, v.configBytes) {
		v.lock()
		return ErrChanged
	}
	// No delete-then-rename fallback: failure must leave the old config in place.
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	v.lock()
	if d, e := os.Open(v.root); e == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	after, err := readBounded(path, 16<<10)
	if err != nil || !bytes.Equal(after, raw) {
		return ErrChanged
	}
	return nil
}
