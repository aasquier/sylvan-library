package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/alexedwards/argon2id"
	"golang.org/x/crypto/argon2"
)

// The OWASP minimum profile, pinned: memory-hard
// enough to matter, small enough (19 MiB a call) that a handful of concurrent
// logins cannot OOM the 1GB instance. Not a default to tune; the decision is
// ADR 5's.
const (
	MemoryCostKiB = 19_456
	TimeCost      = 2
	Parallelism   = 1
	saltLength    = 16 // the recorded hashes' salt length
	keyLength     = 32 // the recorded hashes' digest length
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
		return failf("%w: password is longer than %d bytes",
			ErrWeakPassword, MaxPasswordBytes)
	}
	if len([]rune(password)) < MinPasswordLength {
		return failf("%w: password must be at least %d characters "+
			"-- length beats punctuation, so a short phrase is a fine answer",
			ErrWeakPassword, MinPasswordLength)
	}
	return nil
}

// HashPassword hashes for storage, in the recorded PHC string form
// (`$argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>`), so a hash made today
// is interchangeable with every hash already in the file.
//
// The salt is drawn here and the encoding is `hashWithSalt` below rather than
// the library's own `CreateHash`, and that is deliberate: it makes the salt an
// *argument*, which is what lets `testdata/crypto.json` pin this function's
// output **byte for byte** against the recorded corpus instead of settling
// for a round trip. A hash written here has to verify for the rest of the
// file's life, whatever verifies it next.
func HashPassword(password string) (string, error) {
	if err := CheckStrength(password); err != nil {
		return "", err
	}
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("no entropy for a password salt: %w", err)
	}
	return hashWithSalt(password, salt), nil
}

// hashWithSalt is the encoder: Argon2id at the profile above, written as the
// recorded PHC string.
//
// Every detail here is the format's decision, copied rather than
// chosen. The version field is `v=19` (0x13), the parameter order is `m,t,p`,
// and the salt and digest are **unpadded** standard base64 -- not the URL
// alphabet, which is the one mistake that would produce a string a strict
// PHC parser accepts for some salts and rejects for others, so the corpus
// carries a salt with both `+` and `/` in it.
func hashWithSalt(password string, salt []byte) string {
	sum := argon2.IDKey([]byte(password), salt, TimeCost, MemoryCostKiB,
		Parallelism, keyLength)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, MemoryCostKiB, TimeCost, Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(sum))
}

// Verify asks whether password is the one behind storedHash. A nil hash --
// an invited account with no password yet -- costs a dummy verification and
// answers false, so an unclaimed account is not identifiable by how fast it
// is refused. Every failure mode collapses to false: a
// mismatch, a corrupt hash, a hash from some other library.
func Verify(storedHash *string, password string) bool {
	if storedHash == nil {
		VerifyDummy(password)
		return false
	}
	ok, err := argon2id.ComparePasswordAndHash(password, *storedHash)
	return err == nil && ok
}

// dummyHash is computed once per process:
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
