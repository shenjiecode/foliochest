package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/shenjiecode/keyfolio/internal/fsutil"
	"github.com/shenjiecode/keyfolio/internal/vault"
	"os"
	"path/filepath"
	"sort"
)

// Local UI metadata only: never changes vault records, permissions or backups.
type groupRegistry struct {
	Names []string `json:"names"`
}

func (s *Server) loadGroupRegistry() (string, groupRegistry, uint64, error) {
	gen := s.Vault.Generation()
	b, err := s.Vault.Binding()
	if err != nil {
		return "", groupRegistry{}, gen, err
	}
	status := s.Vault.Status()
	if gen != s.Vault.Generation() || !status.Unlocked {
		return "", groupRegistry{}, gen, vault.ErrLocked
	}
	if status.Format < 3 {
		return "", groupRegistry{}, gen, vault.ErrGroupUpgrade
	}
	hash := sha256.Sum256([]byte(b.VaultID + "\x00" + status.Directory))
	path := filepath.Join(s.deviceDir, "group-catalogs", hex.EncodeToString(hash[:])+".json")
	reg := groupRegistry{Names: []string{}}
	raw, err := fsutil.ReadBounded(path, 256<<10)
	if os.IsNotExist(err) {
		return path, reg, gen, nil
	}
	if err != nil {
		return "", reg, gen, err
	}
	if err = json.Unmarshal(raw, &reg); err != nil {
		return "", reg, gen, errors.New("本机分组目录损坏，请先检查配置文件")
	}
	if len(reg.Names) > 512 {
		return "", reg, gen, errors.New("本机分组超过上限")
	}
	seen := map[string]bool{}
	for _, name := range reg.Names {
		if err := vault.ValidateGroupName(name); err != nil {
			return "", reg, gen, err
		}
		if seen[name] {
			return "", reg, gen, errors.New("本机分组目录包含重复名称")
		}
		seen[name] = true
	}
	return path, reg, gen, nil
}
func (s *Server) saveGroupRegistry(path string, reg groupRegistry, gen uint64) error {
	if gen != s.Vault.Generation() || s.Vault.Locked() {
		return vault.ErrLocked
	}
	if len(reg.Names) > 512 {
		return errors.New("本机分组最多 512 个，请先清理空组")
	}
	sort.Strings(reg.Names)
	raw, err := json.Marshal(reg)
	if err != nil {
		return err
	}
	return fsutil.WriteFile(path, raw)
}
func mergeGroupRegistry(groups []vault.GroupInfo, reg *groupRegistry) ([]vault.GroupInfo, bool) {
	names := map[string]bool{}
	for _, name := range reg.Names {
		names[name] = true
	}
	current := map[string]bool{}
	changed := false
	for _, g := range groups {
		current[g.Name] = true
		if g.Name != "" && !names[g.Name] {
			reg.Names = append(reg.Names, g.Name)
			names[g.Name] = true
			changed = true
		}
	}
	for _, name := range reg.Names {
		if !current[name] {
			groups = append(groups, vault.GroupInfo{Name: name, Members: []vault.GroupMember{}})
		}
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })
	return groups, changed
}
