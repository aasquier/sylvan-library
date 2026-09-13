//go:build !unix

package pool

import "io/fs"

// ownerOf has nothing to read on a platform without a POSIX stat, so a
// rebuild there keeps the mode and leaves ownership to the filesystem. See
// the unix half for why this is split at all.
func ownerOf(fs.FileInfo) (uid, gid int, ok bool) { return 0, 0, false }
