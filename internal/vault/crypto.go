package vault

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/shenjiecode/keyfolio/internal/fsutil"
	"golang.org/x/crypto/argon2"
)

// The v1 parameters are fixed and bounded before reading an untrusted config.
const kdfMemory = 64 * 1024
const kdfTime = 3
const kdfThreads = 1

type config struct {
	Format      int    `json:"format"`
	VaultID     string `json:"vault_id"`
	KDF         string `json:"kdf"`
	Salt        []byte `json:"salt"`
	Memory      uint32 `json:"memory_kib"`
	Iterations  uint32 `json:"iterations"`
	Parallelism uint8  `json:"parallelism"`
	WrappedKey  []byte `json:"wrapped_key"`
}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func seal(key, plain, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, aad), nil
}
func unseal(key, sealed, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(sealed) < gcm.NonceSize()+gcm.Overhead() {
		return nil, errors.New("密文不完整")
	}
	return gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], aad)
}
func (c config) aad() []byte {
	c.WrappedKey = nil
	b, _ := json.Marshal(c)
	return append([]byte("keyfolio/config/v1\x00"), b...)
}
func (c config) valid() bool {
	return c.Format >= 1 && c.Format <= FormatVersion && validID(c.VaultID) && c.KDF == "argon2id" && len(c.Salt) == 16 && c.Memory == kdfMemory && c.Iterations == kdfTime && c.Parallelism == kdfThreads && len(c.WrappedKey) == 60
}
func ValidatePassword(password string) error {
	if utf8.RuneCountInString(password) < 12 || len(password) > 1024 {
		return errors.New("主密码至少 12 个字符且最多 1024 字节，建议使用长口令")
	}
	return nil
}
func wrapConfig(password, vaultID string, key []byte) (config, error) {
	return wrapVersion(password, vaultID, key, FormatVersion)
}
func wrapVersion(password, vaultID string, key []byte, format int) (config, error) {
	if err := ValidatePassword(password); err != nil {
		return config{}, err
	}
	c := config{Format: format, VaultID: vaultID, KDF: "argon2id", Salt: make([]byte, 16), Memory: kdfMemory, Iterations: kdfTime, Parallelism: kdfThreads}
	if _, err := rand.Read(c.Salt); err != nil {
		return c, err
	}
	kek := argon2.IDKey([]byte(password), c.Salt, c.Iterations, c.Memory, c.Parallelism, 32)
	defer clear(kek)
	var err error
	c.WrappedKey, err = seal(kek, key, c.aad())
	return c, err
}
func newConfig(password string) (config, []byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return config{}, nil, err
	}
	c, err := wrapConfig(password, newID(), key)
	if err != nil {
		clear(key)
		return c, nil, err
	}
	return c, key, nil
}
func (c config) unlock(password string) ([]byte, error) {
	if !c.valid() {
		return nil, errors.New("资料库格式或加密参数不受支持")
	}
	if len(password) > 1024 {
		return nil, errors.New("密码过长")
	}
	kek := argon2.IDKey([]byte(password), c.Salt, c.Iterations, c.Memory, c.Parallelism, 32)
	defer clear(kek)
	key, err := unseal(kek, c.WrappedKey, c.aad())
	if err != nil {
		return nil, errors.New("主密码不正确或资料库配置已损坏")
	}
	return key, nil
}
func decodeStrict(data []byte, dst any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("JSON 包含额外内容")
	}
	return nil
}
func publish(dir, name string, data []byte) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if info, err := os.Lstat(dir); err != nil || !info.IsDir() {
		return errors.New("记录目录类型不受支持")
	}
	if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
		return errors.New("目标文件已存在，拒绝覆盖")
	}
	tmp, err := fsutil.WriteTemp(dir, ".pending-*", data)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	if err = os.Rename(tmp, filepath.Join(dir, name)); err != nil {
		return fmt.Errorf("发布文件失败: %w", err)
	}
	fsutil.SyncDir(dir)
	return nil
}
