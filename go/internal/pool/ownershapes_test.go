//go:build unix

package pool

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"testing/fstest"
)

// The owner a rebuilt pool inherits, and the two ways there is nobody to
// inherit from.
//
// **A `chown` that is skipped is not the same as one that is refused**, and the
// difference matters on the instance: a refresh driven over a shell runs as
// root while the app runs as its own user, so a rebuilt pool that arrives
// root-owned is a pool the app cannot write at the *next* refresh
// ([rebuild.finish] argues why that is still better than failing the run). The
// skip is the answer for a platform with no Unix owner at all, and it has to be
// reached by asking rather than by guessing — a `FileInfo` that is not the
// kernel's is the only thing that can say so, and nothing on this machine
// produces one by accident.
func TestTheOwnerOfAFileTheKernelDescribed(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "mtg.duckdb")
	if err := os.WriteFile(path, []byte("not really a pool"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	uid, gid, ok := ownerOf(info)
	if !ok {
		t.Fatal("a file this process just wrote has no owner; the two " +
			"refusals below would then be the only answer this ever gives")
	}
	if uid != os.Getuid() || gid != os.Getgid() {
		t.Errorf("the file reads as %d:%d, and this process is %d:%d",
			uid, gid, os.Getuid(), os.Getgid())
	}
}

// A description that did not come from the kernel carries no owner. Both
// shapes, because they are two different facts: a `Sys()` of some other type is
// a filesystem that is not the disk, and a typed nil is a stat that was never
// filled in — and a nil dereferenced for a uid is a panic in the middle of a
// refresh.
func TestAFileInfoWithNoKernelStatHasNoOwner(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		sys  any
	}{
		{"a filesystem that is not the disk", nil},
		{"a stat nobody filled in", (*syscall.Stat_t)(nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			made := fstest.MapFS{"mtg.duckdb": &fstest.MapFile{
				Data: []byte("not really a pool"), Sys: tc.sys}}
			info, err := fs.Stat(made, "mtg.duckdb")
			if err != nil {
				t.Fatal(err)
			}
			uid, gid, ok := ownerOf(info)
			if ok {
				t.Errorf("a file with no kernel stat reported owner %d:%d", uid, gid)
			}
		})
	}
}
