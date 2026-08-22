package library_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/library"
)

func TestTheFileTierCannotBeWalkedOutOfItsRoot(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "real"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "real", "deck.yaml"), []byte("name: Real\n"), 0o644)
	// A deck.yaml one level up, which `<root>/../deck.yaml` would reach.
	_ = os.WriteFile(filepath.Join(filepath.Dir(root), "deck.yaml"), []byte("name: Above\n"), 0o644)
	src := library.NewFileSource(root, true)
	ctx := context.Background()
	if d, err := src.Get(ctx, "real"); err != nil || d.Name != "Real" {
		t.Fatalf("%v %v", d, err)
	}
	for _, bad := range []string{"..", ".", "...", "", "../x", "a/b", `a\b`} {
		_, err := src.Get(ctx, bad)
		var missing library.ErrNotFound
		if !errors.As(err, &missing) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	slugs, _ := src.Slugs(ctx)
	if len(slugs) != 1 || slugs[0] != "real" {
		t.Fatalf("slugs %v", slugs)
	}
}
