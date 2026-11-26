package platform

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ncruces/zenity"
)

func TestDirectoryPickerValidationAndCancellation(t *testing.T) {
	root := t.TempDir()
	selected := filepath.Join(root, "资料 % --folder")
	if err := os.Mkdir(selected, 0700); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	got, err := pickDirectory(ctx, filepath.Join(root, "new", "child"), func(_ context.Context, initial string) (string, error) {
		if initial != root {
			t.Fatalf("nearest parent: %q", initial)
		}
		return selected, nil
	})
	if err != nil || got != selected {
		t.Fatalf("selection %q %v", got, err)
	}
	got, err = pickDirectory(ctx, "", func(context.Context, string) (string, error) { return "", zenity.ErrCanceled })
	if got != "" || !errors.Is(err, ErrDirectoryCanceled) {
		t.Fatalf("cancel: %q %v", got, err)
	}
	_, err = pickDirectory(ctx, "", func(context.Context, string) (string, error) { return "", errors.New("unavailable helper") })
	if !errors.Is(err, ErrDirectoryUnavailable) {
		t.Fatal(err)
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"", "relative", "file:///tmp/folder", file, filepath.Join(root, "missing")} {
		if _, err := pickDirectory(ctx, "", func(context.Context, string) (string, error) { return invalid, nil }); err == nil {
			t.Fatalf("accepted %q", invalid)
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := pickDirectory(cancelled, "", func(context.Context, string) (string, error) { t.Fatal("called after cancel"); return "", nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	timeout, stop := context.WithTimeout(ctx, 10*time.Millisecond)
	defer stop()
	if _, err := pickDirectory(timeout, "", func(ctx context.Context, _ string) (string, error) { <-ctx.Done(); return selected, nil }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("late path accepted", err)
	}
}
