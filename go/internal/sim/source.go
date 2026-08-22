package sim

import "embed"

// SourceFS is this package's own source, embedded at build time. See
// `internal/mana.SourceFS` for why, and `internal/sim/cache.Fingerprint` for
// the only reader.
//
// Fingerprinted for the same reason `internal/pyrand` is, one level down: in
// Python the compiled card lives inside `engine.py` and the float arithmetic
// is CPython's own, so `sim/cache.py` covers both by hashing one file. Here
// `Card`, `Fsum` and `Round` are a package of their own, and a change to any
// of them changes what the engine answers while the engine's own files do not
// move.
//
//go:embed sim.go pyfloat.go equal.go source.go
var SourceFS embed.FS
