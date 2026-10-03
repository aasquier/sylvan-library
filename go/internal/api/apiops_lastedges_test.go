package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
)

// Two last edges, both of them a guard doing its job rather than a fault.

// **A claim has its own bucket and the bucket is enforced.**
//
// The link in somebody's mail is a password-setting endpoint reachable with no
// session, so it is rate limited by address like the login is — and the refusal
// has to happen *before* the token is resolved and before anything is hashed,
// because Argon2 at the production profile is 19 MiB a call and a claim endpoint
// that hashed first would be a denial of service with a password field on it.
func TestAClaimPastItsBudgetIsRefusedBeforeAnythingIsHashed(t *testing.T) {
	t.Parallel()
	rig := faultyAuth(t)
	db, ok := rig.api.accountsDB()
	if !ok {
		t.Fatal("the rig built no accounts database")
	}
	// The claim bucket spent for real, by address. `clientAddress` reads the
	// request's own remote address, which `httptest.NewRequest` sets to
	// 192.0.2.1 — the documentation range, and the address the rig's requests
	// therefore arrive from.
	key := auth.AddressKey("192.0.2.1", "claim")
	for i := 0; i <= auth.ClaimPerAddress.Failures; i++ {
		if _, err := auth.RecordFailure(context.Background(), db, key,
			auth.ClaimPerAddress); err != nil {
			t.Fatal(err)
		}
	}

	status, payload, raw, _ := postSignIn(t, rig.api, "/api/auth/claim",
		`{"token":"`+rig.token+`","password":"`+goodPassword+`"}`, "")
	if status != http.StatusTooManyRequests {
		t.Fatalf("a claim past its budget answered %d: %s", status, raw)
	}
	if said, _ := payload["detail"].(string); strings.TrimSpace(said) == "" {
		t.Errorf("it answered %d with nothing to read: %s", status, raw)
	}
	// And the invite is intact: a refusal at the limiter must not have spent the
	// token, or a flood would burn somebody's only link.
	rig.fault.Heal()
	resolved, err := auth.LookupToken(context.Background(), db, rig.token, "")
	if err != nil || resolved == nil {
		t.Errorf("the invite was consumed by a request the limiter refused: %v", err)
	}
}

// **A deck with no commander has no dossier to fill**, and the intake's dossier
// step says so rather than carrying the refusal up as a crash.
//
// A dossier is a reading of the commander, so a headless deck is the one input
// the check refuses outright — and on an import it is a real input: a paste whose
// commander line the parser could not read lands exactly here.
func TestTheIntakesDossierStepSaysSoWhenThereIsNoCommander(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()
	// `headless` is the rig's own deck with `commander: []`.
	run := intakeRunOver(t, rig, "headless", true, noCredential)

	step := run.dossier(t.Context())
	if changed := stepValue(t, step, "changed"); changed != 0 {
		t.Errorf("a headless deck's dossier reported %v changed", changed)
	}
	note, _ := stepValue(t, step, "note").(string)
	if strings.TrimSpace(note) == "" {
		t.Fatalf("the dossier step reported a silent zero: %+v", step)
	}
}
