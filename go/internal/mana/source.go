package mana

import "embed"

// SourceFS is this package's own source, embedded at build time.
//
// It exists for one caller and one reason: ADR 18 keys a cached simulation on
// **a fingerprint of the code that produced it**, so that no engine change can
// serve a pre-change number -- "not even one nobody remembered to declare".
// A built binary has no source on disk at runtime, so the source comes with
// it. See
// `internal/sim/cache.Fingerprint`, which is the only reader.
//
// **The list is explicit and a test holds it complete.** `*.go` would have
// been shorter and is wrong twice over: it matches `_test.go` as well, so
// every test edit would empty the deployed simulation cache, and the test
// files -- larger than the package -- would ride into the image. The guard is
// `TestEveryFingerprintedPackageEmbedsItsWholeSelf`, which reads each of these
// directories and fails by name when a new file is not listed here.
//
//go:embed mana.go solver.go source.go
var SourceFS embed.FS
