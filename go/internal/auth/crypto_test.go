package auth

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The compatibility claim the accounts flip rests on, held to Python's own
// output.
//
// ADR 38 promises "Argon2id PHC hashes verify as-is". Phase 2 proved the easy
// half -- Go reads a hash Python wrote -- and that was enough while the door
// only ever *resolved* a session. Phase 4 makes Go a writer: a password set
// through the Go claim route produces a hash that `argon2-cffi` has to accept
// for the rest of the file's life, including after a rollback to a
// Python-only door, and including for `mtglab users` on the machine.
//
// A round trip in each direction would not settle that. Go verifying Python's
// hash and Python verifying Go's would both pass even if the two encoders
// disagreed about, say, base64 padding for some salts and not others. So the
// oracle is stronger and the test is simpler: `tests/go_fixtures.py` records
// the exact PHC string argon2-cffi produces for a password **and a fixed
// salt**, and `hashWithSalt` -- the production encoder, which is why
// `HashPassword` takes the salt as an argument at all -- must reproduce it
// character for character. Two implementations that agree on every byte for a
// given input are the same function, and the only free variable left in a real
// hash is the salt, which travels inside the string.
//
// Regenerate with `python tests/go_fixtures.py`; `tests/test_go_fixtures.py`
// fails if the committed file is not what Python writes today.

type cryptoOracle struct {
	Argon2ID struct {
		Params struct {
			MemoryCostKiB int `json:"memory_cost_kib"`
			TimeCost      int `json:"time_cost"`
			Parallelism   int `json:"parallelism"`
			SaltBytes     int `json:"salt_bytes"`
			HashBytes     int `json:"hash_bytes"`
		} `json:"params"`
		MinPasswordLength int `json:"min_password_length"`
		MaxPasswordBytes  int `json:"max_password_bytes"`
		Cases             []struct {
			Note     string `json:"note"`
			Password string `json:"password"`
			SaltB64  string `json:"salt_b64"`
			Hash     string `json:"hash"`
		} `json:"cases"`
	} `json:"argon2id"`
	SHA256Hex []struct {
		Input  string `json:"input"`
		Digest string `json:"digest"`
	} `json:"sha256_hex"`
}

func loadCryptoOracle(t *testing.T) cryptoOracle {
	t.Helper()
	raw, err := os.ReadFile("testdata/crypto.json")
	if err != nil {
		t.Fatalf("read the crypto oracle: %v", err)
	}
	var oracle cryptoOracle
	if err := json.Unmarshal(raw, &oracle); err != nil {
		t.Fatalf("parse the crypto oracle: %v", err)
	}
	if len(oracle.Argon2ID.Cases) == 0 {
		t.Fatal("the crypto oracle has no Argon2 cases; regenerate it with " +
			"`python tests/go_fixtures.py`")
	}
	return oracle
}

// The parameters are the decision, not an implementation detail, and a corpus
// recorded under different ones would prove nothing about this build. Checked
// first so a mismatch reports as "the profiles differ" rather than as a wall
// of unequal hashes.
func TestTheArgon2ProfileIsPythons(t *testing.T) {
	p := loadCryptoOracle(t).Argon2ID
	for _, c := range []struct {
		name      string
		got, want int
	}{
		{"memory_cost", MemoryCostKiB, p.Params.MemoryCostKiB},
		{"time_cost", TimeCost, p.Params.TimeCost},
		{"parallelism", Parallelism, p.Params.Parallelism},
		{"salt length", saltLength, p.Params.SaltBytes},
		{"key length", keyLength, p.Params.HashBytes},
		{"min password length", MinPasswordLength, p.MinPasswordLength},
		{"max password bytes", MaxPasswordBytes, p.MaxPasswordBytes},
	} {
		if c.got != c.want {
			t.Errorf("%s is %d here and %d in `auth/passwords.py`", c.name, c.got, c.want)
		}
	}
}

// Go → Python. The production encoder must write the bytes argon2-cffi wrote,
// which is what makes a hash Go stores one Python will still verify.
func TestTheEncoderWritesPythonsBytes(t *testing.T) {
	for _, c := range loadCryptoOracle(t).Argon2ID.Cases {
		salt, err := base64.StdEncoding.DecodeString(c.SaltB64)
		if err != nil {
			t.Fatalf("%s: the recorded salt is not base64: %v", c.Note, err)
		}
		if got := hashWithSalt(c.Password, salt); got != c.Hash {
			t.Errorf("%s:\n go     %s\n python %s", c.Note, got, c.Hash)
		}
	}
}

// Python → Go. The read side Phase 2 proved, run again over the wider corpus
// -- and with the negative, because an encoder that ignored the password would
// pass the test above and this one's first half.
func TestPythonsHashesVerifyHere(t *testing.T) {
	for _, c := range loadCryptoOracle(t).Argon2ID.Cases {
		hash := c.Hash
		if !Verify(&hash, c.Password) {
			t.Errorf("%s: a hash argon2-cffi wrote did not verify", c.Note)
		}
		if Verify(&hash, c.Password+"-not") {
			t.Errorf("%s: the wrong password verified", c.Note)
		}
		if NeedsRehash(hash) {
			t.Errorf("%s: a hash at this build's own parameters was called stale", c.Note)
		}
	}
}

// And the round trip, which is the weakest of the three and is here because it
// is the one that would catch a salt generator that produced a salt the
// encoder could not encode.
func TestAHashWrittenHereVerifiesHere(t *testing.T) {
	const password = "a passphrase long enough to store"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("not the PHC shape argon2-cffi writes: %s", hash)
	}
	if !Verify(&hash, password) {
		t.Fatal("a freshly written hash did not verify")
	}
	second, err := HashPassword(password)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if second == hash {
		t.Fatal("two hashes of one password are identical, so the salt is not random")
	}
}

// The strength floor, which is a refusal rather than advice: a route answers
// 422 with this sentence, so both halves have to agree on where it falls.
// Measured in *runes* on the low side and *bytes* on the high side, exactly as
// `check_strength` measures them.
func TestTheStrengthFloorIsPythons(t *testing.T) {
	oracle := loadCryptoOracle(t).Argon2ID
	short := strings.Repeat("é", oracle.MinPasswordLength-1)
	if err := CheckStrength(short); err == nil {
		t.Error("a password one character below the floor was accepted")
	}
	// The same string one rune longer is fine, though it is twice as many
	// bytes -- which is the whole reason the two ends are measured differently.
	if err := CheckStrength(short + "é"); err != nil {
		t.Errorf("a password at the floor was refused: %v", err)
	}
	if err := CheckStrength(strings.Repeat("x", oracle.MaxPasswordBytes+1)); err == nil {
		t.Error("a password over the byte ceiling was accepted")
	}
}

// The token hash. One line on each side, and recorded anyway: it is the other
// thing that would make a Go-minted session or invite invisible to Python.
func TestTheTokenHashIsPythons(t *testing.T) {
	for _, c := range loadCryptoOracle(t).SHA256Hex {
		if got := HashToken(c.Input); got != c.Digest {
			t.Errorf("HashToken(%q) = %s, python = %s", c.Input, got, c.Digest)
		}
	}
}

// `secrets.token_urlsafe(32)` is 43 characters from the URL alphabet with no
// padding. The alphabet matters as much as the entropy: a session token rides
// in a cookie and an auth token rides in a URL fragment, and `+` and `/`
// survive neither reliably.
func TestTokensAreShapedLikePythons(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 64; i++ {
		token := TokenURLSafe(TokenBytes)
		if len(token) != 43 {
			t.Fatalf("token is %d characters, not 43: %q", len(token), token)
		}
		if strings.ContainsAny(token, "+/=") {
			t.Fatalf("token is not URL-safe and unpadded: %q", token)
		}
		if seen[token] {
			t.Fatalf("two tokens collided, so the source is not random: %q", token)
		}
		seen[token] = true
	}
}
