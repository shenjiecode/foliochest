package vault

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptedBackupRoundTrip(t *testing.T) {
	v := testVault(t)
	first := add(t, v, "entry", Ask, Deny)
	attachments := []Attachment{{Name: "test.txt", MediaType: "text/plain", Data: []byte("SYNTHETIC_ATTACHMENT_CONTENT")}}
	_, err := v.Commit([]Change{{Key: "entry", ExpectedRevision: first.Revision, Attachments: &attachments}}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	data, err := v.Backup()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("SYNTHETIC_SECRET")) || bytes.Contains(data, attachments[0].Data) {
		t.Fatal("plaintext in archive")
	}
	destination := filepath.Join(t.TempDir(), "restored")
	if err = RestoreBackup(data, destination, "wrong"); err == nil {
		t.Fatal("wrong password accepted")
	}
	if _, err = os.Stat(destination); !os.IsNotExist(err) {
		t.Fatal("failed restore installed data")
	}
	if err = RestoreBackup(data, destination, testPassword); err != nil {
		t.Fatal(err)
	}
	restored := New(destination)
	if err = restored.Unlock(testPassword); err != nil {
		t.Fatal(err)
	}
	entry, err := restored.Get("entry")
	if err != nil || !bytes.Equal(entry.Attachments[0].Data, attachments[0].Data) {
		t.Fatal(err)
	}
	history, err := restored.History("entry")
	if err != nil || len(history.Versions) != 2 {
		t.Fatal(history, err)
	}
	if err = RestoreBackup(data, destination, testPassword); err == nil {
		t.Fatal("nonempty directory overwritten")
	}
	empty := t.TempDir()
	if err = RestoreBackup(data, empty, testPassword); err != nil {
		t.Fatal(err)
	}
}
func zipTestFiles(t *testing.T, names []string, contents [][]byte) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for i, name := range names {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write(contents[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestRestoreRejectsUnsafeArchivesAndTamper(t *testing.T) {
	v := testVault(t)
	add(t, v, "entry", Ask, Ask)
	config, err := os.ReadFile(filepath.Join(v.root, "vault.json"))
	if err != nil {
		t.Fatal(err)
	}
	cases := [][]string{{"vault.json", "../escape"}, {"vault.json", "records/../../escape"}, {"vault.json", "vault.json"}, {"vault.json", "/absolute"}, {"vault.json", "records\\escape"}}
	for _, names := range cases {
		data := zipTestFiles(t, names, [][]byte{config, config})
		if err := RestoreBackup(data, filepath.Join(t.TempDir(), "target"), testPassword); err == nil {
			t.Fatal("accepted", names)
		}
	}
	raw, err := os.ReadFile(records(t, v)[0])
	if err != nil {
		t.Fatal(err)
	}
	var b batch
	if err = json.Unmarshal(raw, &b); err != nil {
		t.Fatal(err)
	}
	b.Changes[0].Description = "tampered"
	raw, _ = json.Marshal(b)
	data := zipTestFiles(t, []string{"vault.json", "records/" + b.BatchID + ".json"}, [][]byte{config, raw})
	if err = RestoreBackup(data, filepath.Join(t.TempDir(), "target"), testPassword); err == nil {
		t.Fatal("tampered archive accepted")
	}
}
func TestRestoreRejectsSymlinkMembersAndOversizedHeaders(t *testing.T) {
	for _, symlink := range []bool{true, false} {
		var buffer bytes.Buffer
		writer := zip.NewWriter(&buffer)
		header := &zip.FileHeader{Name: "vault.json", Method: zip.Store}
		if symlink {
			header.SetMode(os.ModeSymlink | 0600)
		} else {
			header.SetMode(0600)
			header.UncompressedSize64 = uint64(MaxFileBytes) + 1
		}
		member, err := writer.CreateRaw(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = member.Write(nil); err != nil {
			t.Fatal(err)
		}
		if err = writer.Close(); err != nil {
			t.Fatal(err)
		}
		if err = RestoreBackup(buffer.Bytes(), filepath.Join(t.TempDir(), "target"), testPassword); err == nil {
			t.Fatal("unsafe archive accepted", symlink)
		}
	}
}
func TestEmptyBackupAndLockedExport(t *testing.T) {
	v := testVault(t)
	archive, err := v.Backup()
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "empty")
	if err = RestoreBackup(archive, destination, testPassword); err != nil {
		t.Fatal(err)
	}
	restored := New(destination)
	if err = restored.Unlock(testPassword); err != nil {
		t.Fatal(err)
	}
	list, err := restored.List("")
	if err != nil || len(list) != 0 {
		t.Fatal(list, err)
	}
	v.Lock()
	if _, err = v.Backup(); err == nil {
		t.Fatal("locked backup allowed")
	}
}

func TestBackupRejectsMissingDependencies(t *testing.T) {
	v := testVault(t)
	first := add(t, v, "entry", Ask, Ask)
	parent := records(t, v)[0]
	if _, err := v.Commit([]Change{{Key: "entry", ExpectedRevision: first.Revision, Value: ptr("next")}}, true, true); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(parent); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Backup(); err == nil {
		t.Fatal("incomplete backup accepted")
	}
}
