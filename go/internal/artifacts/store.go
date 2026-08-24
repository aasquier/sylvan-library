package artifacts

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Store writes a rendered build into a directory, and
// prunes what it did not make.
//
// The pruning is the part worth explaining. A build with no baseline writes no
// `swaps.md`, so a rebuild used to leave the *previous* build's swap list
// sitting in the directory describing a diff that no longer exists -- an
// artifact that is not merely stale but wrong, and indistinguishable from a
// current one. Found on 2026-08-21 by the parity test across the two deck
// sources: the memory tier replaced the set and the file tier merged into it,
// and only one of them could be right.
//
// Only `Deliverables` are pruned. Anything else a person has put in that
// directory is theirs and is left alone -- the snapshot included, which is
// written by this same call and must survive a build that produces no
// `swaps.md`.
//
// Shared by every writer of a build for one reason: the CLI
// and the API cannot be allowed to disagree about what a rebuild leaves
// behind, and one function is the only way to guarantee they do not.
func Store(files Files, dir string) ([]string, error) {
	// 0755 and 0644 are what the container's 022 umask leaves, and
	// `library.FileSource.Create` writes a
	// `deck.yaml` the same way for the same reason: every writer works this
	// directory as the same user, nothing else reads it, and a library whose
	// files wear two sets of permissions depending on which surface built
	// them is a difference somebody has to explain to a volume backup.
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // the library's standing 022-umask mode
		return nil, err
	}
	written := make([]string, 0, len(files))
	// `RenderAll` places the snapshot last and this loop preserves that order.
	for _, file := range files {
		path := filepath.Join(dir, file.Name)
		if err := os.WriteFile(path, []byte(file.Text), 0o644); err != nil { //nolint:gosec // the library's standing 022-umask mode
			return nil, err
		}
		written = append(written, file.Name)
	}
	for _, name := range Deliverables {
		if files.Has(name) {
			continue
		}
		// A missing file is fine here: a deliverable this build did not
		// produce and the last one did not either is simply absent, which is
		// the ordinary state of `swaps.md` on a first build.
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	return written, nil
}
