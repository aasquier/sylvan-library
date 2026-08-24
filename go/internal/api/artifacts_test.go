package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/artifacts"
	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/deck"
)

// The rebuild route's own tests. `internal/artifacts` already proves the
// *bytes* against its golden over 18 fixture decks; what these prove is the
// layer above -- that the two refusals land on the right status in the right
// order, that nothing is written when one fires, that the shelf a build
// answers with is the shelf a reader asks for, and that a build is not an
// edit: no activity-log entry, and no `deck.yaml` touched.

// plant writes a deck straight onto the file tier, which the fixtures cannot
// quite serve here: the build looks its own answer up under the deck's
// **declared** slug (see TestABuildReportsUnderTheDecksOwnSlug below), and the
// only gate-clean fixture is `mono-green-clean`, whose file says
// `slug: mono-green`. So the clean deck these tests build is that fixture with
// its slug line agreeing with its directory, which is the state every deck in
// the real library is in.
func (r *writeRig) plant(t *testing.T, slug string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "gate", "testdata", "mono-green-clean.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(raw), "slug: mono-green\n", "slug: "+slug+"\n", 1)
	if text == string(raw) {
		t.Fatal("the fixture's slug line moved; this helper is out of date")
	}
	dir := filepath.Join(r.decks, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deck.yaml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return text
}

// built lists what landed in a deck's artifacts directory.
func (r *writeRig) built(t *testing.T, slug string) map[string]string {
	t.Helper()
	dir := filepath.Join(r.decks, slug, "artifacts")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return map[string]string{}
	}
	out := map[string]string{}
	for _, e := range entries {
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = string(raw)
	}
	return out
}

func names(body map[string]any) []string {
	list, _ := body["artifacts"].([]any)
	out := []string{}
	for _, item := range list {
		if row, ok := item.(map[string]any); ok {
			out = append(out, row["name"].(string))
		}
	}
	return out
}

// The whole happy path: four deliverables and a baseline on a first build,
// the shelf reporting them, and the files on disk being the renderer's own.
func TestABuildWritesTheDeliverablesAndAnswersTheShelf(t *testing.T) {
	rig := newWriteRig(t)
	defer rig.close()

	before := rig.plant(t, "clean-build")
	status, body, raw := rig.do(t, alice, "POST", "/api/decks/alice/clean-build/artifacts", `{}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	// A first build has nothing to diff, so there is no `swaps.md` -- which
	// is the ordinary state of a first build and not a failure.
	files := rig.built(t, "clean-build")
	want := []string{"primer-quick.md", "primer-advanced.md",
		"decklist-annotated.md", "moxfield.txt", artifacts.Snapshot}
	for _, name := range want {
		if _, present := files[name]; !present {
			t.Errorf("no %s was written; got %v", name, sortedNames(files))
		}
	}
	if _, present := files["swaps.md"]; present {
		t.Error("a first build has no baseline and must write no swaps.md")
	}
	if len(files) != len(want) {
		t.Errorf("the build wrote %v", sortedNames(files))
	}

	// The snapshot is the deck, dumped -- which is what makes the *next*
	// build's `swaps.md` possible and what `baseline` compares against.
	snapshot, err := deck.FromText(files[artifacts.Snapshot], "clean-build")
	if err != nil {
		t.Fatalf("the snapshot does not parse: %v", err)
	}
	if !snapshot.SameAs(mustParse(t, before, "clean-build")) {
		t.Error("the snapshot is not the deck it was built from")
	}
	if !strings.HasPrefix(files["primer-quick.md"], "# ") ||
		!strings.Contains(files["moxfield.txt"], "SIDEBOARD:") {
		t.Errorf("the deliverables do not look like the renderer's:\n%s", files["primer-quick.md"])
	}

	// The shelf, in the shape the two GETs answer with, plus the build's own
	// two fields.
	if got := names(body); len(got) != 4 {
		t.Errorf("the shelf lists %v", got)
	}
	if body["baseline"] != "current" {
		t.Errorf("a deck just built is current, not %v", body["baseline"])
	}
	if body["buildable"] != true || body["stage"] != "curated" {
		t.Errorf("buildable/stage are %v/%v", body["buildable"], body["stage"])
	}
	if body["forced"] != false {
		t.Errorf("a clean build must not claim to have been forced")
	}
	issues, ok := body["issues"].(map[string]any)
	if !ok {
		t.Fatalf("no issues in %s", raw)
	}
	if errs, _ := issues["errors"].([]any); len(errs) != 0 {
		t.Errorf("the clean fixture reported errors: %v", errs)
	}

	// A build derives files *from* a deck; it does not edit one.
	after, _ := rig.read(t, "clean-build")
	if after != before {
		t.Error("a build rewrote deck.yaml")
	}
	// And it is deliberately outside ADR 28's log -- `service.build_artifacts`
	// is outside `_commit`, and keeping it outside is the decision, because
	// adding it means a second call site and one call site is the log's whole
	// design.
	if entries := rig.history(t, "clean-build", nil); len(entries) != 0 {
		t.Errorf("a build must not be logged; found %d entries", len(entries))
	}
}

// The build looks its own answer up under the deck's **declared** slug while
// writing under the URL's, which is `service.build_artifacts` passing its
// `slug` argument to `write_artifacts` and `_artifacts_json` asking
// `deck.slug`. Every deck in the library has the two equal and this has never
// differed in practice -- so it is reproduced rather than tidied, and pinned
// here so the next person to notice finds the reason instead of a surprise.
//
// Measured on the live wire before it was written: a directory whose file
// declares a slug that is not a deck at all answers **404 after a successful
// write**, the files sitting on disk under the name that was asked for.
func TestABuildReportsUnderTheDecksOwnSlug(t *testing.T) {
	rig := newWriteRig(t)
	defer rig.close()

	// `mono-green-clean`'s file says `slug: mono-green`, and `mono-green` is
	// a real fixture -- so the write lands under the directory and the shelf
	// describes the other deck's (empty) one.
	status, body, raw := rig.do(t, alice, "POST", cleanDeck+"/artifacts", `{}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if got := names(body); len(got) != 0 || body["baseline"] != "unknown" {
		t.Errorf("the shelf described %v / %v; it reads the declared slug's directory",
			got, body["baseline"])
	}
	if files := rig.built(t, "mono-green-clean"); len(files) != 5 {
		t.Errorf("the write goes under the URL's slug: %v", sortedNames(files))
	}

	// And where the declared slug is no deck at all, the shelf's own lookup
	// raises -- a 404 on a request that has already written five files.
	dir := filepath.Join(rig.decks, "on-disk")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	text := "slug: in-the-file\nname: Mismatched\nstatus: built\nstage: curated\n" +
		"commander:\n  - Gyome, Master Chef\n" +
		"cards:\n  - name: Sol Ring\n    category: ramp\n    why: Two mana.\n"
	if err := os.WriteFile(filepath.Join(dir, "deck.yaml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	status, _, raw = rig.do(t, alice, "POST", "/api/decks/alice/on-disk/artifacts", `{"force":true}`)
	if status != 404 {
		t.Fatalf("a declared slug that is no deck answered %d: %s", status, raw)
	}
	if files := rig.built(t, "on-disk"); len(files) != 5 {
		t.Errorf("the write happened before the lookup failed: %v", sortedNames(files))
	}
}

// The shelf a build answers with and the shelf a reader asks for are one
// function (`service._artifacts_json`), so they cannot describe one build two
// ways.
func TestTheBuildAndTheShelfAgree(t *testing.T) {
	rig := newWriteRig(t)
	defer rig.close()

	_, built, raw := rig.do(t, alice, "POST", cleanDeck+"/artifacts", `{}`)
	status, shelf, shelfRaw := rig.do(t, alice, "GET", cleanDeck+"/artifacts", "")
	if status != 200 {
		t.Fatalf("%d %s", status, shelfRaw)
	}
	for _, key := range []string{"baseline", "buildable", "stage"} {
		if built[key] != shelf[key] {
			t.Errorf("%s: the build said %v and the shelf says %v", key, built[key], shelf[key])
		}
	}
	if a, b := names(built), names(shelf); strings.Join(a, ",") != strings.Join(b, ",") {
		t.Errorf("the build listed %v and the shelf lists %v", a, b)
	}
	// The build's two extra fields are its own and must not leak into the
	// read: a GET has nothing to force and no gate run behind it.
	for _, key := range []string{"issues", "forced"} {
		if _, present := shelf[key]; present {
			t.Errorf("the shelf grew a %q: %s", key, shelfRaw)
		}
		if _, present := built[key]; !present {
			t.Errorf("the build lost its %q: %s", key, raw)
		}
	}
}

// The gate's errors are refused by default and `force` overrides them, which
// mirrors `mtglab decks build --force`. A refused build writes nothing.
func TestTheGateRefusesAndForceOverrides(t *testing.T) {
	rig := newWriteRig(t)
	defer rig.close()

	const banned = "/api/decks/alice/mono-green" // the fixture with Primeval Titan
	status, body, raw := rig.do(t, alice, "POST", banned+"/artifacts", `{}`)
	if status != 422 {
		t.Fatalf("a deck failing the gate answered %d: %s", status, raw)
	}
	detail := fmtDetail(body)
	for _, want := range []string{"the gate reports", "error(s) on mono-green",
		"build again with force if you know better"} {
		if !strings.Contains(detail, want) {
			t.Errorf("the refusal does not say %q: %s", want, detail)
		}
	}
	if files := rig.built(t, "mono-green"); len(files) != 0 {
		t.Errorf("a refused build wrote %v", sortedNames(files))
	}

	status, body, raw = rig.do(t, alice, "POST", banned+"/artifacts", `{"force":true}`)
	if status != 200 {
		t.Fatalf("a forced build answered %d: %s", status, raw)
	}
	if body["forced"] != true {
		t.Error("a build that overrode the gate must say so")
	}
	issues, _ := body["issues"].(map[string]any)
	errs, _ := issues["errors"].([]any)
	if len(errs) == 0 {
		t.Error("a forced build must still report what it overrode")
	}
	if files := rig.built(t, "mono-green"); len(files) != 5 {
		t.Errorf("a forced build wrote %v", sortedNames(files))
	}
}

// A draft is refused by the renderer and **no flag here reaches it** (ADR 13).
// `force` overrides the gate's errors, which are things the deck got wrong; a
// draft is not wrong but unfinished, and the way out is to write the
// rationales and promote it.
func TestADraftIsRefusedAndForceDoesNotReachIt(t *testing.T) {
	rig := newWriteRig(t)
	defer rig.close()

	// The `draft` fixture also carries a banned card, which the gate refuses
	// *first* -- the gate runs before the renderer. So the flag is
	// needed to get past the gate at all, and then the draft rule refuses it
	// anyway. That is the whole shape of ADR 13's exception: `force`
	// overrides what the deck got *wrong*, and a draft is not wrong but
	// unfinished.
	status, answer, raw := rig.do(t, alice, "POST", "/api/decks/alice/draft/artifacts", `{}`)
	if status != 422 || !strings.Contains(fmtDetail(answer), "the gate reports") {
		t.Fatalf("the gate should refuse first: %d %s", status, raw)
	}

	status, answer, raw = rig.do(t, alice, "POST", "/api/decks/alice/draft/artifacts", `{"force":true}`)
	if status != 422 {
		t.Fatalf("a forced draft answered %d: %s", status, raw)
	}
	detail := fmtDetail(answer)
	if !strings.Contains(detail, "is a draft, and the artifacts are the shareable surface") {
		t.Errorf("the refusal is not the renderer's: %s", detail)
	}
	if !strings.Contains(detail, "need a `why`") &&
		!strings.Contains(detail, "every card is justified") {
		t.Errorf("the refusal does not name what is owed: %s", detail)
	}
	if files := rig.built(t, "draft"); len(files) != 0 {
		t.Errorf("a refused draft wrote %v", sortedNames(files))
	}
}

// A rebuild prunes the deliverables it did not produce. The previous build's
// `swaps.md` describes a diff that no longer exists -- stale in the one way
// indistinguishable from current -- and the snapshot beside it is not a
// deliverable and must survive.
func TestARebuildPrunesAStaleSwapList(t *testing.T) {
	rig := newWriteRig(t)
	defer rig.close()

	rig.plant(t, "clean-build")
	const target = "/api/decks/alice/clean-build/artifacts"
	dir := filepath.Join(rig.decks, "clean-build", "artifacts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"swaps.md", "notes-of-my-own.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("from before"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// No snapshot, so this build has no baseline and writes no `swaps.md`.
	if status, _, raw := rig.do(t, alice, "POST", target, `{}`); status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	files := rig.built(t, "clean-build")
	if _, present := files["swaps.md"]; present {
		t.Error("the previous build's swaps.md survived a build that wrote none")
	}
	// Only `Deliverables` are pruned. A file a person put there is theirs.
	if files["notes-of-my-own.md"] != "from before" {
		t.Error("the rebuild swept away a file that was not a deliverable")
	}
	if _, present := files[artifacts.Snapshot]; !present {
		t.Error("the snapshot is not a deliverable and must not be pruned")
	}

	// Now there *is* a baseline, so the next build diffs against it -- and
	// because nothing changed, `swaps.md` is an empty diff rather than absent.
	if status, body, raw := rig.do(t, alice, "POST", target, `{}`); status != 200 {
		t.Fatalf("%d %s", status, raw)
	} else if got := names(body); len(got) != 5 {
		t.Errorf("a build with a baseline lists %v", got)
	}
	if swaps := rig.built(t, "clean-build")["swaps.md"]; !strings.Contains(swaps, "0 out / 0 in") {
		t.Errorf("the swap list is not a diff of nothing:\n%s", swaps)
	}
}

// `baseline` is computed by comparing the stored snapshot against the deck,
// never a file timestamp -- so an edit makes it `different` and reverting the
// edit makes it `current` again.
func TestTheBaselineFollowsTheDeckAndNotTheClock(t *testing.T) {
	rig := newWriteRig(t)
	defer rig.close()

	rig.plant(t, "clean-build")
	const deckPath = "/api/decks/alice/clean-build"
	if status, _, raw := rig.do(t, alice, "POST", deckPath+"/artifacts", `{}`); status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if _, body, _ := rig.do(t, alice, "GET", deckPath+"/artifacts", ""); body["baseline"] != "current" {
		t.Fatalf("just built and already %v", body["baseline"])
	}

	if status, _, raw := rig.do(t, alice, "PATCH", deckPath,
		`{"field":"status","value":"built"}`); status != 200 {
		t.Fatalf("the edit answered %d: %s", status, raw)
	}
	if _, body, _ := rig.do(t, alice, "GET", deckPath+"/artifacts", ""); body["baseline"] != "different" {
		t.Errorf("an edited deck reads %v", body["baseline"])
	}
	// Reverted, and current again -- which a timestamp could never say.
	if status, _, raw := rig.do(t, alice, "PATCH", deckPath,
		`{"field":"status","value":"theoretical"}`); status != 200 {
		t.Fatalf("the revert answered %d: %s", status, raw)
	}
	if _, body, _ := rig.do(t, alice, "GET", deckPath+"/artifacts", ""); body["baseline"] != "current" {
		t.Errorf("a reverted deck reads %v, and its artifacts really are current",
			body["baseline"])
	}
}

// ADR 5 and ADR 22 at a build, which is a write: absence hides, refusal
// explains. The deliverables are the *shareable* surface, so a reader may
// have every one of them -- and may not rebuild them.
func TestWhoMayBuildIsDecidedByTheSource(t *testing.T) {
	rig := newWriteRig(t)
	defer rig.close()

	// `rich` says `shared: false`, so bob's view of the file tier does not
	// contain it at all: 404, never 403, which would confirm it exists.
	if status, _, _ := rig.do(t, bob, "POST", "/api/decks/alice/rich/artifacts", `{}`); status != 404 {
		t.Errorf("a deck bob cannot see answered %d, expected 404", status)
	}
	// One he can see and may not write is a 403.
	status, body, raw := rig.do(t, bob, "POST", cleanDeck+"/artifacts", `{}`)
	if status != 403 {
		t.Fatalf("a shared deck bob may not build answered %d: %s", status, raw)
	}
	if !strings.Contains(fmtDetail(body), "mono-green-clean") {
		t.Errorf("the refusal does not name the deck: %v", body)
	}
	// He may still *read* what somebody else built, which is the whole point
	// of the deliverables being the shareable surface.
	if status, _, _ := rig.do(t, bob, "GET", cleanDeck+"/artifacts", ""); status != 200 {
		t.Errorf("bob may read a shared deck's shelf; got %d", status)
	}
	if status, _, _ := rig.do(t, alice, "POST", "/api/decks/nobody/whatever/artifacts", `{}`); status != 404 {
		t.Errorf("an unknown owner answered %d, expected 404", status)
	}
	if files := rig.built(t, "mono-green-clean"); len(files) != 0 {
		t.Errorf("a refused build wrote %v", sortedNames(files))
	}
}

// The body is validated before the deck is resolved -- the recorded order,
// so a malformed body is a 422 before any 404 about the deck it was aimed
// at.
func TestAMalformedBodyIsRefusedBeforeTheDeck(t *testing.T) {
	rig := newWriteRig(t)
	defer rig.close()

	// A deck that does not exist, so a 404 is what would come next -- and a
	// 422 proves the body was looked at first.
	for _, body := range []string{``, `[]`, `null`, `{`} {
		status, _, raw := rig.do(t, alice, "POST", "/api/decks/alice/no-such-deck/artifacts", body)
		if status != 422 {
			t.Errorf("body %q answered %d, expected 422 before the deck's 404: %s",
				body, status, raw)
		}
	}
	// A good body on the same path reaches the deck and gets its 404.
	if status, _, _ := rig.do(t, alice, "POST", "/api/decks/alice/no-such-deck/artifacts",
		`{}`); status != 404 {
		t.Errorf("a good body on a missing deck answered %d, expected 404", status)
	}
}

// `force` is read with the recorded truthiness, not a Go cast:
// `"no"` is true and `0` is false, and a route that used `.(bool)` would read
// both as false.
func TestForceIsReadWithTheRecordedTruthiness(t *testing.T) {
	rig := newWriteRig(t)
	defer rig.close()

	const banned = "/api/decks/alice/mono-green/artifacts"
	for body, forced := range map[string]bool{
		`{"force":"no"}`: true, `{"force":0}`: false, `{"force":1}`: true,
		`{"force":[]}`: false, `{"force":null}`: false, `{}`: false,
	} {
		status, _, raw := rig.do(t, alice, "POST", banned, body)
		if forced && status != 200 {
			t.Errorf("%s answered %d, expected a forced build: %s", body, status, raw)
		}
		if !forced && status != 422 {
			t.Errorf("%s answered %d, expected the gate's refusal: %s", body, status, raw)
		}
	}
}

// The SQL tier builds too, which the file tier's tests cannot prove: bob
// rebuilding his own deck writes rows, not files -- and the sweep that prunes
// a stale `swaps.md` from a directory prunes it from the table.
func TestTheSQLTierBuildsToo(t *testing.T) {
	rig := newWriteRig(t)
	defer rig.close()

	// Built the way a person would, every step a route: create a deck in
	// bob's own library, justify a card, promote it, then force past the gate
	// (one card is not 99). That is the SQL tier's whole write path rather
	// than a fixture arranged to succeed -- and it starts from a create
	// because a created deck's YAML is `Deck.dump`'s, which the surgical
	// editor can serve.
	const bobsDeck = "/api/decks/bob/bobs-build"
	if status, _, raw := rig.do(t, bob, "POST", "/api/decks",
		`{"slug":"bobs-build","name":"Bobs Build","commander":["Gyome, Master Chef"]}`); status != 200 {
		t.Fatalf("the create answered %d: %s", status, raw)
	}
	if status, _, raw := rig.do(t, bob, "POST", bobsDeck+"/cards",
		`{"name":"Sol Ring","category":"ramp","why":"Two mana on turn one."}`); status != 200 {
		t.Fatalf("the card answered %d: %s", status, raw)
	}
	if status, _, raw := rig.do(t, bob, "PATCH", bobsDeck,
		`{"field":"stage","value":"curated"}`); status != 200 {
		t.Fatalf("the promotion answered %d: %s", status, raw)
	}

	const bobs = bobsDeck + "/artifacts"
	status, body, raw := rig.do(t, bob, "POST", bobs, `{"force":true}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if got := names(body); len(got) == 0 {
		t.Fatalf("the owned tier built nothing: %s", raw)
	}
	if body["baseline"] != "current" {
		t.Errorf("baseline is %v", body["baseline"])
	}
	// Read one back through the route, so the assertion is about what the app
	// serves rather than about a column.
	status, one, raw := rig.do(t, bob, "GET", bobs+"/moxfield.txt", "")
	if status != 200 {
		t.Fatalf("reading it back answered %d: %s", status, raw)
	}
	if text, _ := one["text"].(string); !strings.Contains(text, "SIDEBOARD:") {
		t.Errorf("the stored deliverable is not the renderer's: %q", text)
	}
	// Nothing landed on the file tier, and nothing landed in the log.
	if _, err := os.Stat(filepath.Join(rig.decks, "bobs-build")); err == nil {
		t.Error("an owned deck's build reached the file tier")
	}
	// The two edits above are logged; neither the create nor the build is --
	// both are outside `service._commit`, which is ADR 28's one call site.
	owner := int64(2)
	entries := rig.history(t, "bobs-build", &owner)
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Summary), "artifact") ||
			strings.Contains(strings.ToLower(e.Summary), "built") ||
			strings.Contains(strings.ToLower(e.Summary), "created") {
			t.Errorf("something outside `_commit` reached the log: %q", e.Summary)
		}
	}
	if len(entries) != 2 { // the added card, the promotion
		t.Errorf("the log holds %d entries, expected the two edits: %+v", len(entries), entries)
	}
}

// An instance with no card pool -- a fresh one, before `mtglab data refresh`.
//
// **The two surfaces agree, and that they agree is the assertion.** Until
// 2026-08-22 they did not: the gate was handed an **empty** map here and a nil
// one by the validate route, because `service.build_artifacts` wrote
// `_pool_for(deck, con)` where `service.validate_deck` wrote `_pool_for(deck,
// con) if con is not None else None`, and `_pool_for` answered `{}` for a
// missing connection. An empty map is not a nil one: the gate reads nil as "the
// pool was never consulted" and warns `unverified` once, and reads empty as a
// pool that has never heard of these cards and calls all 99 unknown. So one
// deck had three answers on one instance -- validate warned, a rebuild refused
// with 99 errors, and every edit came back `ok: false`.
//
// The wart was kept at first rather than ruled on, and this test pinned the
// disagreement. It has since been ruled: no pool means a nil map, every
// caller that feeds the gate passes it through, and `mtglab decks build` --
// which handled the no-pool case correctly throughout -- is
// what the route now matches, as rule 3 requires.
func TestWithNoPoolTheBuildAndTheGateAgree(t *testing.T) {
	decks := decksDir(t)
	// An app.db, because the owner segment is resolved through it -- but no
	// Pool, which is the state a fresh instance is in before `data refresh`.
	db, err := auth.Open(appDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a := New(Config{DecksDir: decks, AdminEmail: "alice@example.com", AppDB: db})
	scope := alice

	// The validate route: one warning, no errors.
	status, body, raw := callAs(t, a, scope, "GET", cleanDeck+"/validate", "")
	if status != 200 {
		t.Fatalf("validate answered %d: %s", status, raw)
	}
	if errs, _ := body["errors"].([]any); len(errs) != 0 {
		t.Errorf("validate reports errors with no pool: %v", errs)
	}
	warns, _ := body["warnings"].([]any)
	if len(warns) != 1 {
		t.Fatalf("validate reports %d warnings, expected the one `unverified`: %s", len(warns), raw)
	}

	// The build: it goes through, unforced, and reports the same one warning
	// the validate route reported. No `force` is needed, because there is
	// nothing to override -- which is the whole of the fix.
	status, body, raw = callAs(t, a, scope, "POST", cleanDeck+"/artifacts", `{}`)
	if status != 200 {
		t.Fatalf("the build answered %d with no pool: %s", status, raw)
	}
	if body["forced"] != false {
		t.Error("nothing was overridden, so the build must not say it forced anything")
	}
	issues, _ := body["issues"].(map[string]any)
	if errs, _ := issues["errors"].([]any); len(errs) != 0 {
		t.Errorf("the build reports errors with no pool: %v", errs)
	}
	buildWarns, _ := issues["warnings"].([]any)
	if len(buildWarns) != len(warns) {
		t.Errorf("the build reports %d warnings where validate reported %d; "+
			"the two surfaces must say the same thing about one deck",
			len(buildWarns), len(warns))
	}

	// And the third surface: an ordinary edit, whose verdict comes back through
	// `commit`. This half was nil-correct before the build route was, so on
	// a pool-less instance two writes to the same deck once disagreed --
	// a divergence no rigged suite could see, because the rigs always build
	// the 21-card pool.
	status, body, raw = callAs(t, a, scope, "PATCH", cleanDeck,
		`{"field":"bracket","value":3}`)
	if status != 200 {
		t.Fatalf("an edit with no pool answered %d: %s", status, raw)
	}
	if body["ok"] != true {
		t.Errorf("an edit with no pool reports the deck as failing: %s", raw)
	}
	if errs, _ := body["errors"].([]any); len(errs) != 0 {
		t.Errorf("an edit with no pool reports %d errors: %v", len(errs), errs)
	}
	if editWarns, _ := body["warnings"].([]any); len(editWarns) != len(warns) {
		t.Errorf("the edit reports %d warnings where validate reported %d",
			len(editWarns), len(warns))
	}
	// The deliverables still render; they simply carry no mana costs, which is
	// what `cards={}` means to the annotated list.
	arts := filepath.Join(decks, "mono-green-clean", "artifacts", "decklist-annotated.md")
	text, readErr := os.ReadFile(arts)
	if readErr != nil {
		t.Fatalf("the forced build wrote nothing: %v", readErr)
	}
	if strings.Contains(string(text), "` `") {
		t.Error("a build with no pool should print no mana costs")
	}
}

func sortedNames(files map[string]string) []string {
	out := make([]string, 0, len(files))
	for name := range files {
		out = append(out, name)
	}
	return out
}

func mustParse(t *testing.T, text, slug string) *deck.Deck {
	t.Helper()
	d, err := deck.FromText(text, slug)
	if err != nil {
		t.Fatalf("parsing %s: %v", slug, err)
	}
	return d
}

// The route answers its keys in the shelf's own order, then the
// build's two. Not a shape the goldens can see -- they sort -- and the reason
// ordered marshalling exists at all.
func TestTheAnswerKeepsTheRecordedKeyOrder(t *testing.T) {
	rig := newWriteRig(t)
	defer rig.close()

	_, _, raw := rig.do(t, alice, "POST", cleanDeck+"/artifacts", `{}`)
	var order []string
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	if _, err := decoder.Token(); err != nil { // the opening brace
		t.Fatal(err)
	}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			t.Fatal(err)
		}
		order = append(order, key.(string))
		var discard json.RawMessage
		if err := decoder.Decode(&discard); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"artifacts", "baseline", "buildable", "stage", "issues", "forced"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("keys are %v, the record says %v", order, want)
	}
}
