package mt19937

import "embed"

// SourceFS is this package's own source, embedded at build time. See
// `internal/mana.SourceFS` for why, and `internal/sim/cache.Fingerprint` for
// the only reader.
//
// This package is in the fingerprint deliberately, not by habit. The
// generator is **ours**: a one-character change to `RandBelow`'s rejection
// loop changes every game the engine plays while `tier1/*.go` and
// `mana/*.go` sit untouched. That is exactly the hole ADR 18's
// consequence 2 is written against — engine code whose changes the cache
// key cannot see — so the generator hashes with the engine it drives.
//
//go:embed mt19937.go random.go source.go
var SourceFS embed.FS
