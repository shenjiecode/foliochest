package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/shenjiecode/keyfolio/internal/access"
	"github.com/shenjiecode/keyfolio/internal/fsutil"
)

type Client struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	TokenHash string    `json:"token_hash,omitempty"`
}
type clients struct {
	mu   sync.Mutex
	path string
	list []Client
}

func newClients(dir string) (*clients, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	c := &clients{path: filepath.Join(dir, "clients.json"), list: []Client{}}
	b, err := os.ReadFile(c.path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	if len(b) > 1<<20 {
		return nil, errors.New("客户端配置过大")
	}
	if err = json.Unmarshal(b, &c.list); err != nil {
		return nil, errors.New("本机客户端配置损坏，请先备份后处理")
	}
	return c, nil
}
func (c *clients) save() error {
	b, err := json.Marshal(c.list)
	if err != nil {
		return err
	}
	return fsutil.WriteFile(c.path, b)
}
func (c *clients) All() []Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := append([]Client{}, c.list...)
	for i := range out {
		out[i].TokenHash = ""
	}
	return out
}
func (c *clients) Add(name string) (Client, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if strings.TrimSpace(name) == "" || len(name) > 120 || len(c.list) >= 32 {
		return Client{}, "", errors.New("客户端名称为空、过长或数量已达上限")
	}
	token := "kf_" + access.ID()
	hash := sha256.Sum256([]byte(token))
	item := Client{ID: access.ID(), Name: name, TokenHash: hex.EncodeToString(hash[:]), CreatedAt: time.Now().UTC()}
	c.list = append(c.list, item)
	if err := c.save(); err != nil {
		c.list = c.list[:len(c.list)-1]
		return Client{}, "", err
	}
	item.TokenHash = ""
	return item, token, nil
}
func (c *clients) Delete(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	old := append([]Client{}, c.list...)
	next := []Client{}
	for _, item := range c.list {
		if item.ID != id {
			next = append(next, item)
		}
	}
	c.list = next
	if err := c.save(); err != nil {
		c.list = old
		return err
	}
	return nil
}
func (c *clients) Authenticate(token string) (Client, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	hash := sha256.Sum256([]byte(token))
	encoded := hex.EncodeToString(hash[:])
	for _, item := range c.list {
		if subtle.ConstantTimeCompare([]byte(item.TokenHash), []byte(encoded)) == 1 {
			item.TokenHash = ""
			return item, true
		}
	}
	return Client{}, false
}
