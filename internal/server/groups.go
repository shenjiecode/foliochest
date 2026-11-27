package server

import (
	"errors"
	"github.com/shenjiecode/keyfolio/internal/vault"
	"net/http"
	"slices"
	"strings"
)

// Invoked only after webpage authentication and the shared CSRF/mutation guard.
func (s *Server) serveGroups(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/groups") {
		return false
	}
	switch r.URL.Path {
	case "/api/groups", "/api/groups/create", "/api/groups/move", "/api/groups/edit":
	default:
		return false
	}
	want := "POST"
	if r.URL.Path == "/api/groups" {
		want = "GET"
	}
	if !method(w, r, want) {
		return true
	}
	s.groupMu.Lock()
	defer s.groupMu.Unlock()
	var out any
	err := func() error {
		groups, err := s.Vault.Groups()
		if err != nil {
			return err
		}
		// Reading older formats remains supported without creating local metadata.
		if r.URL.Path == "/api/groups" && s.Vault.Status().Format < 3 {
			out = groups
			return nil
		}
		path, reg, gen, err := s.loadGroupRegistry()
		if err != nil {
			return err
		}
		groups, learned := mergeGroupRegistry(groups, &reg)
		if learned {
			if err = s.saveGroupRegistry(path, reg, gen); err != nil {
				return err
			}
		}
		switch r.URL.Path {
		case "/api/groups":
			out = groups
		case "/api/groups/create":
			var in struct {
				Name string `json:"name"`
			}
			if !decode(w, r, &in) {
				return errDecoded
			}
			if err = vault.ValidateGroupName(in.Name); err != nil {
				return err
			}
			if slices.Contains(reg.Names, in.Name) {
				return errors.New("分组已经存在")
			}
			reg.Names = append(reg.Names, in.Name)
			if err = s.saveGroupRegistry(path, reg, gen); err != nil {
				return err
			}
			out = vault.GroupInfo{Name: in.Name, Members: []vault.GroupMember{}}
		case "/api/groups/move":
			var in vault.GroupMove
			if !decode(w, r, &in) {
				return errDecoded
			}
			if in.Group != "" && !slices.Contains(reg.Names, in.Group) && len(reg.Names) >= 512 {
				return errors.New("本机分组最多 512 个，请先清理空组")
			}
			out, err = s.Vault.MoveGroup(in)
			if err != nil {
				return err
			}
			if in.Group != "" && !slices.Contains(reg.Names, in.Group) {
				reg.Names = append(reg.Names, in.Group)
			}
			if err = s.saveGroupRegistry(path, reg, gen); err != nil {
				return errors.New("资料已移动，但本机分组目录保存失败；请重新读取状态")
			}
		case "/api/groups/edit":
			var in vault.GroupEdit
			if !decode(w, r, &in) {
				return errDecoded
			}
			if in.Source == "" || in.Source == in.Target {
				return errors.New("请选择分组并填写不同的新组名")
			}
			if in.Target != "" {
				if err = vault.ValidateGroupName(in.Target); err != nil {
					return err
				}
				if slices.Contains(reg.Names, in.Target) {
					return errors.New("目标分组已经存在")
				}
			}
			var source *vault.GroupInfo
			for i := range groups {
				if groups[i].Name == in.Source {
					source = &groups[i]
					break
				}
			}
			if source == nil {
				return vault.ErrNotFound
			}
			if source.Count+source.DeletedCount == 0 {
				// Recheck membership to reject a stale empty-group review.
				current, e := s.Vault.Groups()
				if e != nil {
					return e
				}
				for _, g := range current {
					if g.Name == in.Source && g.Count+g.DeletedCount > 0 {
						return vault.ErrConflict
					}
				}
				if len(in.Expected) != 0 {
					return vault.ErrConflict
				}
				out = []vault.Summary{}
			} else {
				out, err = s.Vault.EditGroup(in)
				if err != nil {
					return err
				}
			}
			reg.Names = slices.DeleteFunc(reg.Names, func(n string) bool { return n == in.Source })
			if in.Target != "" {
				reg.Names = append(reg.Names, in.Target)
			}
			if err = s.saveGroupRegistry(path, reg, gen); err != nil {
				return errors.New("本机分组目录保存失败，请重新读取分组及资料状态")
			}
		}
		if gen != s.Vault.Generation() || s.Vault.Locked() {
			return vault.ErrLocked
		}
		return nil
	}()
	if errors.Is(err, errDecoded) {
		return true
	}
	if err != nil {
		apiError(w, err, 409)
	} else {
		if r.Method == "POST" {
			s.activity()
		}
		writeJSON(w, out)
	}
	return true
}

var errDecoded = errors.New("request decode already reported")
