package mt19937

import "embed"

// SourceFS is this package's own source, embedded at build time. See
// `internal/mana.SourceFS` for why, and `internal/sim/cache.Fingerprint` for
// the only reader.
//
// This package is fingerprinted where Python's counterpart is not, and that is
// a deliberate difference rather than an oversight. `sim/cache.py` hashes
// `engine.py` and `mana.py` and leaves `random` alone, because `random` is
// CPython's and cannot change under a running interpreter. Here the generator
// is **ours**: a one-character change to `_randbelow`'s rejection loop changes
// every game the engine plays while `tier1/*.go` and `mana/*.go` sit
// untouched. That is exactly the hole ADR 18's consequence 2 is written
// against, so the port closes it rather than inheriting a reason that stopped
// being true when the code moved languages.
//
//go:embed mt19937.go random.go source.go
var SourceFS embed.FS
