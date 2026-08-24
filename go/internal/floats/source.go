package floats

import "embed"

// SourceFS is this package's own source, embedded at build time. See
// `internal/mana.SourceFS` for why, and `internal/sim/cache.Fingerprint` for
// the only reader.
//
// Fingerprinted where Python's counterpart is not, and for the same reason
// `internal/mt19937` is: in Python this arithmetic is CPython's -- `math.fsum`
// and `round` are the interpreter's own and cannot change under a running
// process -- so `sim/cache.py` has nothing to hash. Here it is **ours**, and a
// one-character change to Shewchuk's summation or to the ties-to-even rounding
// changes what every tier answers while `tier1/*.go` and `mana/*.go` sit
// untouched.
//
// It arrived in this package on 2026-08-22 (#249) from `internal/sim`, whose
// embed list it was already in. The move is the reason that list has a comment
// about it: a file leaving a package is the one edit the per-package
// completeness test cannot see.
//
// `repr.go` joined on 2026-08-23, arriving from `internal/sim/tier1` when the
// Forge result needed CPython's float rendering — and it was
// `TestEveryFingerprintedPackageEmbedsItsWholeSelf` that noticed, not the
// author. A file ARRIVING is the edit the per-package test *can* see, which is
// the half of the pair that works.
//
//go:embed floats.go repr.go source.go
var SourceFS embed.FS
