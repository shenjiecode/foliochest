package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeLocation(t *testing.T, device, directory string) {
	t.Helper()
	raw, _ := json.Marshal(map[string]string{"directory": directory})
	if err := os.WriteFile(filepath.Join(device, "location.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestInitialLocationIsUnselectedAndDoesNotCreateVault(t *testing.T) {
	device := t.TempDir()
	got, err := resolveVaultDirectory("", device)
	if err != nil || got != "" {
		t.Fatalf("initial: %q %v", got, err)
	}
	items, _ := os.ReadDir(device)
	if len(items) != 0 {
		t.Fatal("startup wrote a default vault")
	}
	explicit := filepath.Join(t.TempDir(), "chosen")
	got, err = resolveVaultDirectory(explicit, device)
	if err != nil || got != explicit {
		t.Fatalf("explicit: %q %v", got, err)
	}
	if _, err := os.Stat(explicit); !os.IsNotExist(err) {
		t.Fatal("explicit path created")
	}
}
func TestRememberedMissingAndExistingDefaultArePreserved(t *testing.T) {
	device := t.TempDir()
	previous := filepath.Join(device, "vault")
	if err := os.Mkdir(previous, 0700); err != nil {
		t.Fatal(err)
	}
	got, err := resolveVaultDirectory("", device)
	if err != nil || got != previous {
		t.Fatalf("existing default: %q %v", got, err)
	}
	missing := filepath.Join(t.TempDir(), "no-longer-here")
	// Marshal is also needed for backslashes on Windows.
	writeLocation(t, device, missing)
	got, err = resolveVaultDirectory("", device)
	if err != nil || got != missing {
		t.Fatalf("remembered missing: %q %v", got, err)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("missing vault recreated")
	}
	for _, raw := range []string{"{", `{"directory":""}`, `{"directory":"relative"}`} {
		if err := os.WriteFile(filepath.Join(device, "location.json"), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := resolveVaultDirectory("", device); err == nil {
			t.Fatal("malformed location silently ignored")
		}
		if got, err := resolveVaultDirectory(missing, device); err != nil || got != missing {
			t.Fatal("explicit recovery failed")
		}
	}
}
