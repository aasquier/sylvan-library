package sim

import "embed"

// SourceFS is this package's own source, embedded at build time. See
// `internal/mana.SourceFS` for why, and `internal/sim/cache.Fingerprint` for
// the only reader.
//
// Fingerprinted for the same reason `internal/pyrand` is, one level down: in
// Python the compiled card lives inside `engine.py`, so `sim/cache.py` covers
// it by hashing one file. Here `Card` and `Equal` are a package of their own,
// and `Equal` in particular decides which basic leaves a hand -- which
// reorders the rest and picks the next land.
//
// **`pyfloat.go` used to be in this list and is not, because #249 moved it to
// `internal/pyfloat`.** That package is fingerprinted in its own right; the
// list here shrank rather than the coverage. Worth one sentence because it is
// the one way the guard on these lists can be evaded: a test holds each
// package's embed complete against its own directory, and a file leaving the
// package entirely satisfies both sides of that check while walking out of the
// key. What caught it was the build, not the test -- the embed pattern stopped
// matching a file. Rename a file and the test speaks; move one to a new
// package and only `engineSources` can.
//
//go:embed sim.go equal.go source.go
var SourceFS embed.FS
