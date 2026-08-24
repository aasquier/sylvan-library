package tier1

import "embed"

// SourceFS is this package's own source, embedded at build time. See
// `internal/mana.SourceFS` for why, and `internal/sim/cache.Fingerprint` for
// the only reader.
//
// This is the engine's entry in `cache.engineSources`, and `repr.go`
// is in it even though nothing serves those strings: a change there cannot
// move a number, but excluding a file is a claim about behaviour that can go
// stale, and the cost of being wrong about it is a stale figure on a deck page
// rather than a recomputation.
//
//go:embed tier1.go run.go repr.go source.go
var SourceFS embed.FS
