package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/deckedit"
	"github.com/aasquier/sylvan-library/go/internal/library"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The intake's **writes**, and what each of them does when the shelf will not
// take them.
//
// `intakefailures_test.go` drives the five actions against an endpoint that
// refuses everything, which is the fault above the write. These are the ones
// below it: a source that is read-only, a deck file the process cannot read, a
// directory it cannot write into, and an editor operation that refuses one card
// out of several. All four are things the deployed instance can be in — a
// volume mounted read-only, a file somebody chmodded by hand, a deck entry the
// text scanner cannot find — and none of them had ever been driven, because the
// only call site is a job behind a route that has already proved the deck is
// writable.
//
// They are driven at `intakeRun` rather than through the route for exactly that
// reason: the route's own guards make these states unreachable from outside, and
// reaching past a guard to get at the code it guards is what an in-package test
// is for. The run is the real one — the same struct the job builds, the same
// editor, the same file tier.

// intakeRunOver is one intake in flight over a deck this test planted, with the
// api the rig built (its recorder, its pool) and nothing stubbed.
func intakeRunOver(t *testing.T, rig *jobRig, slug string, writable bool,
	set claude.Settings) *intakeRun {

	t.Helper()
	src := library.NewFileSource(rig.decks, writable)
	d, err := src.Get(t.Context(), slug)
	if err != nil {
		t.Fatalf("reading the deck this test just planted: %v", err)
	}
	stance, err := claude.Preset("collaborator")
	if err != nil {
		t.Fatal(err)
	}
	return &intakeRun{api: rig.api, src: src, slug: slug, deck: d,
		actor: "alice",
		req: claude.IntakeRequest{Endpoint: set.Endpoint,
			Ledger: rig.api.claudeLedger},
		stance: stance}
}

// freshDeck is a draft with two cards and no reasons: what an import leaves
// behind, and what every action on the sheet has something to do with.
const freshDeck = `slug: %s
name: Fresh Import
status: theoretical
stage: draft
commander:
  - Goreclaw, Terror of Qal Sisma
bracket: 2
cards:
  - name: Sol Ring
    category: utility
    why: ''
  - name: Craterhoof Behemoth
    category: utility
    why: ''
`

func plantFresh(t *testing.T, rig *jobRig, slug string) {
	t.Helper()
	plantDeck(t, rig.decks, slug, strings.Replace(freshDeck, "%s", slug, 1))
}

// A tier nothing may write to refuses the pass **before** it reads the file.
//
// This is a volume mounted read-only, and the shape of the answer matters: the
// pass has to come back as a failure rather than as "nothing needed doing",
// because the second reads as a deck that was already complete.
func TestAnIntakeWriteOverAReadOnlyTierIsRefused(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()
	plantFresh(t, rig, "read-only-tier")

	run := intakeRunOver(t, rig, "read-only-tier", false, noCredential)
	n, err := run.write(t.Context(), "why", []string{"Sol Ring"},
		func(text, name string) (string, error) {
			return deckedit.DraftRationale(text, name, "Two mana, every game.")
		})
	if err == nil {
		t.Fatal("a read-only tier took a write and said nothing")
	}
	if n != 0 {
		t.Errorf("%d cards were reported written to a tier that refuses writes", n)
	}
	// And the file is untouched.
	if strings.Contains(readDeckFile(t, rig.decks, "read-only-tier"), "why_by") {
		t.Error("a refused write marked a rationale anyway")
	}
}

// A deck file the process cannot read refuses the pass at the read.
//
// The run already holds the parsed deck — it was read before the file went
// unreadable, which is exactly the deployed shape: an intake is a job, and
// minutes pass between the route reading the deck and the job writing it.
func TestAnIntakeWriteRefusesADeckFileItCannotRead(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root reads everything, so there is no unreadable file to build")
	}
	rig := newJobRig(t, noCredential)
	defer rig.close()
	plantFresh(t, rig, "unreadable-file")
	run := intakeRunOver(t, rig, "unreadable-file", true, noCredential)

	path := filepath.Join(rig.decks, "unreadable-file", "deck.yaml")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	n, err := run.write(t.Context(), "why", []string{"Sol Ring"},
		func(text, name string) (string, error) {
			return deckedit.DraftRationale(text, name, "Two mana, every game.")
		})
	if err == nil {
		t.Fatal("a deck file that cannot be read was written anyway")
	}
	if n != 0 {
		t.Errorf("%d cards were reported written", n)
	}
}

// A directory the process may read and not write refuses the pass at the
// write — after every card's edit has been applied in memory, which is the one
// arm where work is lost and the count must still say nought.
func TestAnIntakeWriteRefusesAShelfItCannotWriteInto(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root writes everywhere, so there is no unwritable directory to build")
	}
	rig := newJobRig(t, noCredential)
	defer rig.close()
	plantFresh(t, rig, "unwritable-shelf")
	run := intakeRunOver(t, rig, "unwritable-shelf", true, noCredential)

	dir := filepath.Join(rig.decks, "unwritable-shelf")
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })

	n, err := run.write(t.Context(), "why", []string{"Sol Ring", "Craterhoof Behemoth"},
		func(text, name string) (string, error) {
			return deckedit.DraftRationale(text, name, "A reason.")
		})
	if err == nil {
		t.Fatal("a shelf that cannot be written to reported a successful write")
	}
	if n != 0 {
		t.Errorf("%d cards were reported written to a shelf that refused the file", n)
	}
}

// **A refusal costs its own card and nothing else.** `DraftRationale` refuses a
// card that already has a reason, which is the guard working rather than a
// fault, and the commonest thing that happens in a real pass: a deck arrives
// part-written. The one card that could be drafted is drafted, the one that
// could not is skipped, and the count is the cards that changed.
func TestOneRefusedCardDoesNotCostThePassTheOthers(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()
	plantDeck(t, rig.decks, "part-written", `slug: part-written
name: Part Written
status: theoretical
stage: draft
commander:
  - Goreclaw, Terror of Qal Sisma
cards:
  - name: Sol Ring
    category: utility
    why: A reason somebody actually wrote.
  - name: Craterhoof Behemoth
    category: utility
    why: ''
`)
	run := intakeRunOver(t, rig, "part-written", true, noCredential)

	n, err := run.write(t.Context(), "why",
		[]string{"Sol Ring", "Craterhoof Behemoth"},
		func(text, name string) (string, error) {
			return deckedit.DraftRationale(text, name, "Drafted for "+name+".")
		})
	if err != nil {
		t.Fatalf("a pass with one refusal in it failed whole: %v", err)
	}
	if n != 1 {
		t.Fatalf("the pass reported %d cards written, want the 1 that was blank", n)
	}
	written := readDeckFile(t, rig.decks, "part-written")
	if !strings.Contains(written, "Drafted for Craterhoof Behemoth.") {
		t.Errorf("the blank card was not drafted:\n%s", written)
	}
	if !strings.Contains(written, "why: A reason somebody actually wrote.") {
		t.Errorf("the card that already had a reason lost it:\n%s", written)
	}
	if got := strings.Count(written, "why_by: claude"); got != 1 {
		t.Errorf("%d rationales are marked, want 1:\n%s", got, written)
	}
}

// **Every card refused is no write at all.** A pass where the editor refuses
// every card must not rewrite the file: a deck file written back byte-identical
// is still a deck file rewritten, and the activity log would carry an edit that
// changed nothing (ADR 28).
func TestAPassWhereEveryCardIsRefusedWritesNothing(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()
	plantFresh(t, rig, "all-refused")
	run := intakeRunOver(t, rig, "all-refused", true, noCredential)
	before := readDeckFile(t, rig.decks, "all-refused")

	n, err := run.write(t.Context(), "why",
		[]string{"A Card This Deck Does Not Hold", "Another One"},
		func(text, name string) (string, error) {
			return deckedit.DraftRationale(text, name, "A reason.")
		})
	if err != nil {
		t.Fatalf("a pass that could place nothing reported a failure: %v", err)
	}
	if n != 0 {
		t.Errorf("%d cards were reported written out of two the deck does not hold", n)
	}
	if after := readDeckFile(t, rig.decks, "all-refused"); after != before {
		t.Errorf("a pass that changed nothing rewrote the file:\n%s", after)
	}
}

// ---- the description pass --------------------------------------------------

// describing is a scripted endpoint that answers one description.
func describing(t *testing.T) claude.Settings {
	t.Helper()
	script := &scriptedClaude{replies: []string{
		answer("end_turn", said(`{"strategy":"Ramp into big green creatures and `+
			`swing.","themes":["stompy","ramp"],"fact":"Goreclaw discounts creatures."}`)),
	}}
	return script.start(t)
}

// The description pass, over the same three shelves the rationale pass meets.
//
// Each of these has already **paid for the call** by the time it fails, which is
// the reason the step has to say so: a person who watched the sheet spend a call
// and then saw "0 of 1, nothing said" would reasonably read it as a deck that
// needed no description.
func TestTheDescriptionPassSaysSoWhenTheTierWillNotTakeIt(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()
	plantFresh(t, rig, "describe-read-only")
	run := intakeRunOver(t, rig, "describe-read-only", false, noCredential)
	run.req.Endpoint = describing(t).Endpoint

	assertFailedStep(t, run.describe(t.Context()))
	if strings.Contains(readDeckFile(t, rig.decks, "describe-read-only"), "strategy:") {
		t.Error("a read-only tier was written to anyway")
	}
}

// The same pass against a deck file the process cannot read: the call has been
// paid for and the answer cannot be placed.
func TestTheDescriptionPassSaysSoWhenTheFileCannotBeRead(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root reads everything, so there is no unreadable file to build")
	}
	rig := newJobRig(t, noCredential)
	defer rig.close()
	plantFresh(t, rig, "describe-unreadable")
	run := intakeRunOver(t, rig, "describe-unreadable", true, noCredential)
	run.req.Endpoint = describing(t).Endpoint

	path := filepath.Join(rig.decks, "describe-unreadable", "deck.yaml")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	assertFailedStep(t, run.describe(t.Context()))
}

// A deck file that is not a mapping at all refuses the editor's write while the
// description itself arrived fine — the fault between a good answer and a file
// nothing can edit.
func TestTheDescriptionPassSaysSoWhenTheFileCannotBeEdited(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()
	plantFresh(t, rig, "describe-unparseable")
	run := intakeRunOver(t, rig, "describe-unparseable", true, noCredential)
	run.req.Endpoint = describing(t).Endpoint

	// The run holds the deck it read; the file underneath is replaced with
	// something the editor cannot open. A half-finished hand edit is this.
	plantDeck(t, rig.decks, "describe-unparseable", "\t- this is not a deck\n")

	// **The sentence here carries the file's own parse error, and this lane
	// reports that rather than redecorating it**: the intake has no
	// `forgeTrouble` of its own, so `intakeFailed` hands `claude.Explain` an
	// error it has no arm for and the raw text reaches the step note. What is
	// asserted is the part that is not in question — the step failed, it said
	// so, and the deck kept the description it had.
	step := run.describe(t.Context())
	if changed := stepValue(t, step, "changed"); changed != 0 {
		t.Errorf("a refused step reported %v changed", changed)
	}
	if note, _ := stepValue(t, step, "note").(string); strings.TrimSpace(note) == "" {
		t.Fatalf("a refused step reported a silent zero: %+v", step)
	}
	if strings.Contains(readDeckFile(t, rig.decks, "describe-unparseable"), "strategy:") {
		t.Error("a file the editor could not open was written to anyway")
	}
}

// The description arrived, the editor placed it, and the **save** is refused.
//
// The last arm of the pass and the most expensive: a call paid for, a file
// edited in memory, and a shelf the process may read and not write — which on
// the instance is a volume remounted read-only under a running process.
func TestTheDescriptionPassSaysSoWhenTheSaveIsRefused(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root writes everywhere, so there is no unwritable directory to build")
	}
	rig := newJobRig(t, noCredential)
	defer rig.close()
	plantFresh(t, rig, "describe-unwritable")
	run := intakeRunOver(t, rig, "describe-unwritable", true, noCredential)
	run.req.Endpoint = describing(t).Endpoint

	dir := filepath.Join(rig.decks, "describe-unwritable")
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })

	assertFailedStep(t, run.describe(t.Context()))
	if strings.Contains(readDeckFile(t, rig.decks, "describe-unwritable"), "strategy:") {
		t.Error("the description reached a file the process cannot write")
	}
}

// And the pass that works: the description reaches the file as the deck's own
// `strategy`, the themes land beside it, and both are recorded.
func TestTheDescriptionPassWritesTheStrategyAndTheThemes(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()
	plantFresh(t, rig, "describe-happy")
	run := intakeRunOver(t, rig, "describe-happy", true, noCredential)
	run.req.Endpoint = describing(t).Endpoint

	step := run.describe(t.Context())
	if changed := stepValue(t, step, "changed"); changed != 1 {
		t.Fatalf("the description pass reported %v changed: %+v", changed, step)
	}
	written := readDeckFile(t, rig.decks, "describe-happy")
	if !strings.Contains(written, "Ramp into big green creatures") {
		t.Errorf("the description did not reach the file:\n%s", written)
	}
	if !strings.Contains(written, "stompy") {
		t.Errorf("the themes did not reach the file:\n%s", written)
	}
}

// assertFailedStep is the one thing every refused step owes a reader: a zero
// with a sentence beside it, in words that name no machinery (commandment 10).
func assertFailedStep(t *testing.T, step wire.OrderedMap) {
	t.Helper()
	if changed := stepValue(t, step, "changed"); changed != 0 {
		t.Errorf("a refused step reported %v changed", changed)
	}
	note, _ := stepValue(t, step, "note").(string)
	if strings.TrimSpace(note) == "" {
		t.Fatalf("a refused step reported a silent zero: %+v", step)
	}
	for _, machinery := range []string{"permission denied", "no such file",
		"yaml:", "/var/folders", "deck.yaml"} {
		if strings.Contains(strings.ToLower(note), machinery) {
			t.Errorf("the sentence a player reads carries %q: %q", machinery, note)
		}
	}
}

func stepValue(t *testing.T, step wire.OrderedMap, key string) any {
	t.Helper()
	for _, kv := range step {
		if kv.Key == key {
			return kv.Value
		}
	}
	return nil
}
