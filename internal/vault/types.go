package vault

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	FormatVersion      = 2
	MaxFileBytes       = 16 << 20
	MaxPayloadBytes    = 8 << 20
	MaxAttachmentBytes = 2 << 20
	MaxBatchEntries    = 20
)

type Policy string

const (
	Allow Policy = "allow"
	Ask   Policy = "ask"
	Deny  Policy = "deny"
)

func (p Policy) Valid() bool { return p == Allow || p == Ask || p == Deny }

var (
	ErrLocked      = errors.New("资料库尚未解锁")
	ErrChanged     = errors.New("资料库配置已变化，请重新解锁")
	ErrConflict    = errors.New("条目版本已变化或存在冲突，请重新读取版本")
	ErrUnavailable = errors.New("条目暂不可用，请检查文件同步状态")
	ErrDenied      = errors.New("该操作已被条目权限拒绝")
	ErrApproval    = errors.New("该操作需要用户确认")
	ErrNotFound    = errors.New("条目不存在")
	ErrUpgrade     = errors.New("此操作需要资料库格式 2，请先备份并在设置页确认升级")
	ErrDeleted     = errors.New("条目已删除，请从回收站恢复")
)

type Attachment struct {
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
	Data      []byte `json:"data"`
}
type Document struct {
	Value       string       `json:"value"`
	Format      string       `json:"format"`
	Attachments []Attachment `json:"attachments"`
}
type Metadata struct {
	EntryID     string   `json:"entry_id"`
	Revision    string   `json:"revision"`
	Parents     []string `json:"parents"`
	Key         string   `json:"key"`
	Description string   `json:"description"`
	ReadPolicy  Policy   `json:"read_policy"`
	WritePolicy Policy   `json:"write_policy"`
	UpdatedAt   string   `json:"updated_at"`
	Deleted     bool     `json:"deleted,omitempty"`
}
type Summary struct {
	EntryID     string `json:"entry_id"`
	Deleted     bool   `json:"deleted,omitempty"`
	Key         string `json:"key"`
	Description string `json:"description"`
	Revision    string `json:"revision"`
	ReadPolicy  Policy `json:"read_policy"`
	WritePolicy Policy `json:"write_policy"`
	State       string `json:"state"`
	Versions    int    `json:"versions"`
}
type Entry struct {
	Metadata
	Document
}
type Change struct {
	Key              string        `json:"key"`
	ExpectedRevision string        `json:"expected_revision"`
	Description      *string       `json:"description,omitempty"`
	Value            *string       `json:"value,omitempty"`
	Format           *string       `json:"format,omitempty"`
	Attachments      *[]Attachment `json:"attachments,omitempty"`
	ReadPolicy       *Policy       `json:"read_policy,omitempty"`
	WritePolicy      *Policy       `json:"write_policy,omitempty"`
}
type ReadSpec struct {
	Key            string   `json:"key"`
	Revision       string   `json:"revision,omitempty"`
	IncludeValue   bool     `json:"include_value"`
	Attachments    []string `json:"attachments,omitempty"`
	AllAttachments bool     `json:"all_attachments,omitempty"`
}
type ReadResult struct {
	Key         string       `json:"key"`
	Revision    string       `json:"revision"`
	Value       *string      `json:"value,omitempty"`
	Format      string       `json:"format,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}
type Status struct {
	Format    int      `json:"format"`
	Directory string   `json:"directory"`
	Exists    bool     `json:"exists"`
	Unlocked  bool     `json:"unlocked"`
	Entries   int      `json:"entries"`
	Issues    []string `json:"issues"`
}

func validateMetadata(m Metadata) error {
	if !validID(m.EntryID) || !validID(m.Revision) || len(m.Parents) > MaxBatchEntries {
		return errors.New("无效条目标识")
	}
	for _, p := range m.Parents {
		if !validID(p) {
			return errors.New("无效父版本")
		}
	}
	if strings.TrimSpace(m.Key) == "" || utf8.RuneCountInString(m.Key) > 160 || strings.ContainsAny(m.Key, "\x00\r\n") {
		return errors.New("key 需为 1–160 个字符的单行名称")
	}
	if strings.TrimSpace(m.Description) == "" || utf8.RuneCountInString(m.Description) > 1000 {
		return errors.New("description 需为 1–1000 个字符")
	}
	if !m.ReadPolicy.Valid() || !m.WritePolicy.Valid() {
		return errors.New("无效权限策略")
	}
	return nil
}
func validateDocument(d Document) error {
	if d.Format != "text" && d.Format != "markdown" && d.Format != "json" {
		return errors.New("不支持的正文格式")
	}
	if len(d.Value) > 1<<20 {
		return errors.New("正文最大为 1 MiB")
	}
	if len(d.Attachments) > 16 {
		return errors.New("每个条目最多 16 个附件")
	}
	names := map[string]bool{}
	for _, a := range d.Attachments {
		if a.Name == "" || len(a.Name) > 240 || strings.ContainsAny(a.Name, "/\\\x00\r\n") || a.Name == "." || a.Name == ".." {
			return errors.New("无效附件名称")
		}
		if names[a.Name] {
			return errors.New("附件名称重复")
		}
		names[a.Name] = true
		if len(a.Data) > MaxAttachmentBytes {
			return fmt.Errorf("附件 %s 超过 2 MiB", a.Name)
		}
		if len(a.MediaType) > 128 {
			return errors.New("无效附件类型")
		}
	}
	return nil
}
func validID(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
