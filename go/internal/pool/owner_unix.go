//go:build unix

package pool

import (
	"io/fs"
	"syscall"
)

// ownerOf reads the numeric owner off a stat result so a rebuilt pool can
// keep the served one's ownership across the rename.
//
// It is split by platform for one reason: `syscall.Stat_t` is not a thing
// everywhere, and the pool has no business failing to compile on a platform
// nobody deploys to just because it wanted a uid. On anything without it the
// other half returns false and the chown is simply not attempted.
func ownerOf(info fs.FileInfo) (uid, gid int, ok bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return 0, 0, false
	}
	return int(stat.Uid), int(stat.Gid), true
}
