package auth

import (
	"errors"
	"fmt"
	"strings"

	"github.com/alexedwards/argon2id"
)

// The OWASP minimum profile `auth/passwords.py` pins and argues: memory-hard
// enough to matter, small enough (19 MiB a call) that a handful of concurrent
// logins cannot OOM the 1GB instance. Not a default to tune; the decision is
// ADR 5's.
const (
	MemoryCostKiB = 19_456
	TimeCost      = 2
	Parallelism   = 1
	saltLength    = 16 // argon2-cffi's default
	keyLength     = 32 // argon2-cffi's default
)

// MaxPasswordBytes bounds the input a hash will be computed over, and
// MinPasswordLength is the floor -- length, not complexity.
const (
	MaxPasswordBytes  = 1024
	MinPasswordLength = 12
)

// ErrWeakPassword is raised rather than storing a password that should have
// been refused.
var ErrWeakPassword = errors.New("weak password")

var params = &argon2id.Params{
	Memory:      MemoryCostKiB,
	Iterations:  TimeCost,
	Parallelism: Parallelism,
	SaltLength:  saltLength,
	KeyLength:   keyLength,
}

// CheckStrength refuses a password that must not be stored.
func CheckStrength(password string) error {
	if len(password) > MaxPasswordBytes {
		return fmt.Errorf("%w: password is longer than %d bytes",
			ErrWeakPassword, MaxPasswordBytes)
	}
	if len([]rune(password)) < MinPasswordLength {
		return fmt.Errorf("%w: password must be at least %d characters "+
			"-- length beats punctuation, so a short phrase is a fine answer",
			ErrWeakPassword, MinPasswordLength)
	}
	return nil
}

// HashPassword hashes for storage, in the PHC string form argon2-cffi writes
// (`$argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>`), so a hash made here
// verifies there and the other way round.
func HashPassword(password string) (string, error) {
	if err := CheckStrength(password); err != nil {
		return "", err
	}
	return argon2id.CreateHash(password, params)
}

// Verify asks whether password is the one behind storedHash. A nil hash --
// an invited account with no password yet -- costs a dummy verification and
// answers false, so an unclaimed account is not identifiable by how fast it
// is refused. Every failure mode collapses to false, as in Python: a
// mismatch, a corrupt hash, a hash from some other library.
func Verify(storedHash *string, password string) bool {
	if storedHash == nil {
		VerifyDummy(password)
		return false
	}
	ok, err := argon2id.ComparePasswordAndHash(password, *storedHash)
	return err == nil && ok
}

// dummyHash is computed once per process, like `passwords._dummy_hash`:
// computing it per call would cost two hashes on an unknown account against
// one on a known one, the same timing signal pointing the other way.
var dummyHash = func() string {
	h, err := argon2id.CreateHash("mtglab-dummy-password-for-timing-parity", params)
	if err != nil {
		panic(err)
	}
	return h
}()

// VerifyDummy burns a verification against a hash of nothing, for unknown
// accounts. The result is thrown away; the work is the point.
func VerifyDummy(password string) {
	_, _ = argon2id.ComparePasswordAndHash(password, dummyHash)
}

// NeedsRehash reports whether a stored hash was made with weaker parameters
// than the ones above -- checked at login, when the plaintext is in hand.
func NeedsRehash(storedHash string) bool {
	p, _, _, err := argon2id.DecodeHash(storedHash)
	if err != nil {
		return false
	}
	return p.Memory < MemoryCostKiB || p.Iterations < TimeCost ||
		p.Parallelism < Parallelism || !strings.HasPrefix(storedHash, "$argon2id$")
}
