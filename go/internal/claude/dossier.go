package claude

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/aasquier/sylvan-library/go/internal/claude/ledger"
	"github.com/aasquier/sylvan-library/go/internal/claude/tools"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/deckread"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The commander dossier: who this character is, and who says so.
//
// `claude/dossier.py`, and the first mode whose facts do not all come from the
// pool -- which is the whole reason ADR 19 exists. Three sources, each with a
// narrow jurisdiction: **card facts come from the pool** (`Brief` assembles
// them here, before the model is called), **the meta and the history come
// from a hosted web search** with the page shown next to the claim, and
// **Claude supplies voice and framing** and carries no factual weight.
//
// Three things enforce that and none of them is the system prompt. The
// schema keeps prose and sources in different fields (it crossed as generated
// data, in modes.json). **A cited page must be one the search actually
// returned** -- `KeepSources`, in sources.go, because with a response schema
// in play the API attaches no citations and a URL in the payload is just a
// string the model typed. And **every card the dossier names is looked up**:
// competitors come back as pool rows or they do not come back at all.
//
// **If no source survives, the dossier is refused** rather than shown: an
// unsourced dossier renders as exactly the blended paragraph ADR 19 rejected,
// arrived at by accident.
//
// # The cache, and why its key is Python's byte for byte
//
// A dossier is about a *character*, so it is cached on the commander's
// `oracle_id` -- two decks led by Gyome are two lists and one Gyome -- plus a
// fingerprint of the prompt, the schema and the model id, so that no prompt
// change can serve text written under a different one and a seat granted
// Opus neither reads nor overwrites what Sonnet wrote.
//
// Unlike ADR 18's simulation cache, **a Go-written row and a Python-written
// row for the same commander are the SAME row**, deliberately. The sim cache
// keeps the two runtimes' rows apart because a one-ulp divergence would
// otherwise serve one runtime's number under the other's name; a dossier is
// the model's prose after a source check both runtimes hold to one corpus,
// and the day this route flips, every commander already written would
// otherwise cost a four-minute paid search to write again. So `Fingerprint`
// reproduces `dossier._fingerprint` exactly -- the same version, the same
// instruction bytes (they cross in modes.json), the same
// `json.dumps(schema, sort_keys=True)` (pyjson.go) and the same model id --
// and a corpus holds the keys to Python's. That is arrangeable honestly where
// the sim cache's was not: nothing here has to read Python source at runtime.
//
// It writes nothing to a deck. `boundary_test.go` covers this file.

// DossierVersion is `DOSSIER_VERSION`: bumped when a stored dossier's *shape*
// changes, so old rows are missed rather than rendered into a UI that expects
// something else. 3 is "allies" joining the story's rivals as cited prose.
const DossierVersion = 3

// DossierNever is the promise the payload carries about itself.
const DossierNever = "This is Claude's writing over cited pages. The card facts " +
	"beside it are the pool's."

// ErrNoCommander is a deck with no commander the pool can find: the deck is
// fine and there is simply nothing to write a dossier about. Its own type
// because the caller's answer (422) differs from a missing deck's (404).
//
// `Slug` is the slug the caller asked by, which is Python's `brief(slug)` --
// not the deck file's own, which can differ on the file tier.
type ErrNoCommander struct{ Slug string }

func (e *ErrNoCommander) Error() string {
	return fmt.Sprintf("%s has no commander the pool can find, so there is nothing "+
		"to write a dossier about. (A deck with no card pool loaded reaches "+
		"this too -- run `mtglab data refresh`.)", e.Slug)
}

// ---------------------------------------------------------------- the facts

// DossierBrief is `dossier.brief`: what the pool knows about this deck's
// commander, before the call. `service.commander_dossier`'s payload, reused
// rather than re-queried, so the counted strip on the deck page and the facts
// handed to the model are one query and the prose cannot disagree with the
// strip. ErrNoCommander when `card` is null -- no commander, a commander the
// pool lacks, or no pool at all.
func DossierBrief(ctx context.Context, conn *pool.Conn, slug string, d *deck.Deck) (wire.OrderedMap, error) {
	facts, err := deckread.CommanderDossier(ctx, conn, d)
	if err != nil {
		return nil, err
	}
	card, _ := kv(facts, "card").(wire.OrderedMap)
	if len(card) == 0 {
		return nil, &ErrNoCommander{Slug: slug}
	}
	return facts, nil
}

// dossierOpening is `_ask_for`: the facts, then the ask. The brief is
// rendered as Python renders it -- `json.dumps(..., indent=2)` -- which is why
// a corpus row can hold the whole message to Python's bytes rather than its
// key order alone.
func dossierOpening(facts wire.OrderedMap) string {
	card, _ := kv(facts, "card").(wire.OrderedMap)
	name := asString(kv(card, "name"))
	body := wire.OrderedMap{
		{Key: "commander", Value: card},
		{Key: "supertypes", Value: kv(facts, "supertypes")},
		{Key: "subtypes_and_how_rare_they_are", Value: kv(facts, "subtypes")},
		{Key: "other_cards_carrying_this_name", Value: kv(facts, "other_cards")},
		{Key: "printing_history", Value: kv(facts, "printings")},
	}
	return strings.Join([]string{
		"Here is everything the pool knows about this commander. All of it is a " +
			"query rather than a recollection, and it is the authority on what " +
			"the card does.",
		"",
		pyDumps(body, pyDumpOptions{Indent: 2}),
		"",
		fmt.Sprintf("Write the dossier for %s. Search the web for the character's "+
			"story, the archetype, the competitors, the rivals and the standing; "+
			"take the card's own facts from above.", name),
	}, "\n")
}

// ---------------------------------------------------------------- the cache

// Fingerprint is `dossier._fingerprint(tier)`: a hash of what would change
// the answer's shape or content -- the version, the prompt, the schema and
// the model id -- so editing the instructions misses every stored row, and
// two tiers never share a commander's dossier. See the package note on why
// these are Python's bytes exactly.
func Fingerprint(tier string) (string, error) {
	mode, err := GetMode(ModeCommanderDossier)
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	digest.Write([]byte(strconv.Itoa(DossierVersion)))
	digest.Write([]byte(mode.Instructions))
	digest.Write([]byte(pyDumps(mode.ResponseSchema, pyDumpOptions{SortKeys: true})))
	digest.Write([]byte(ModelFor(tier)))
	return hex.EncodeToString(digest.Sum(nil))[:16], nil
}

// CacheKey is `dossier.cache_key`: the key for one commander's dossier, or ""
// when there is no oracle id -- which disables caching for that deck rather
// than colliding every uncatalogued commander onto one row.
func CacheKey(oracleID, tier string) (string, error) {
	if oracleID == "" {
		return "", nil
	}
	fp, err := Fingerprint(tier)
	if err != nil {
		return "", err
	}
	return oracleID + ":" + fp, nil
}

// DossierStore is the `dossier_cache` table: one row per commander and
// fingerprint, and the same rules every other app.db writer here follows. It
// **never fails the feature** -- a miss is a call, not a 500, and a write
// that fails is a warning -- and it opens nothing: it is handed the
// `mode=rw` handle the door already holds, because the ladder runs once at
// boot and a file this created would be a database at version zero. A nil
// handle is an instance with no app.db yet; every read misses and every
// write warns. (Python's `get` would *create* app.db there, since
// `auth.db.connect` makes the file and runs the ladder; a reader that
// acquires a database is the one thing the Go side refuses to be.)
type DossierStore struct {
	db  *sql.DB
	log *slog.Logger
}

// NewDossierStore is a store over an app.db handle somebody else opened, or
// over nil.
func NewDossierStore(db *sql.DB, logger *slog.Logger) *DossierStore {
	if logger == nil {
		logger = slog.Default()
	}
	return &DossierStore{db: db, log: logger}
}

// DossierHit is one stored dossier: the body as it was stored and when.
//
// **The body is raw JSON and stays raw.** A stored dossier is served back
// byte for byte -- into a report, and out of `GET .../dossier` -- because
// decoding it into a `map[string]any` and re-encoding would alphabetise its
// keys, which is the Notes-tab regression of v159-v166 wearing another hat.
type DossierHit struct {
	Result    json.RawMessage
	CreatedAt string
}

// Get is `dossier.get`: a stored dossier, or nil. Never raises.
func (s *DossierStore) Get(ctx context.Context, key string) *DossierHit {
	if s == nil || s.db == nil || key == "" {
		return nil
	}
	var blob, createdAt string
	err := s.db.QueryRowContext(ctx,
		"SELECT result_json, created_at FROM dossier_cache WHERE key = ?", key).
		Scan(&blob, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		s.log.Warn("dossier cache read failed", "err", err)
		return nil
	}
	if !json.Valid([]byte(blob)) {
		return nil
	}
	return &DossierHit{Result: json.RawMessage(blob), CreatedAt: createdAt}
}

// Put is `dossier.put`: store one dossier. Never raises.
func (s *DossierStore) Put(ctx context.Context, key, oracleID, commander string, result any) {
	if s == nil || s.db == nil || key == "" {
		return
	}
	// Compact, as Python's `separators=(",", ":")` is. Non-ASCII stays UTF-8
	// where Python would write `\uXXXX`; both parse to the same value, and the
	// blob is served raw rather than hashed, so the bytes need not agree.
	blob, err := wire.Marshal(result)
	if err != nil {
		s.log.Warn("dossier is not storable", "err", err)
		return
	}
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO dossier_cache (key, oracle_id, commander, result_json, "+
			"created_at) VALUES (?, ?, ?, ?, ?) ON CONFLICT(key) DO UPDATE SET "+
			"result_json = excluded.result_json, created_at = excluded.created_at",
		key, oracleID, commander, string(blob), now()); err != nil {
		s.log.Warn("dossier cache write failed", "err", err)
	}
}

// ---------------------------------------------------------------- the answer

// Archetype is the deckbuilding half: a name and a cited passage.
type Archetype struct {
	Name      string   `json:"name"`
	Prose     string   `json:"prose"`
	SourceIDs []string `json:"source_ids"`
}

// DossierBody is the stored and served dossier, in Python's key order.
type DossierBody struct {
	Who         Passage      `json:"who"`
	Archetype   Archetype    `json:"archetype"`
	Competitors []Competitor `json:"competitors"`
	// The story's allies and rivals are prose like `who` and `standing`, not
	// card lists: a plot line is not a pool row, and Bolas has a dozen
	// printings none of which is the point.
	Allies   Passage  `json:"allies"`
	Rivals   Passage  `json:"rivals"`
	Standing Passage  `json:"standing"`
	Sources  []Source `json:"sources"`
	// Surfaced rather than swallowed. A number that climbs is a prompt that
	// has started inventing citations, and nobody checks a number they
	// cannot see.
	SourcesDropped     int `json:"sources_dropped"`
	CompetitorsDropped int `json:"competitors_dropped"`
	Searched           int `json:"searched"`
}

// Usage is the three counters every report carries. The cache figure is
// deliberate: zero across repeated calls means the prefix is drifting.
type Usage struct {
	InputTokens     int `json:"input_tokens"`
	OutputTokens    int `json:"output_tokens"`
	CacheReadTokens int `json:"cache_read_tokens"`
}

// emptyObject is Python's `{}` -- what `dossier` and `research` carry until
// there is a body, so a client never has to tell a missing key from an
// absent answer.
var emptyObject = json.RawMessage("{}")

// DossierReport is one response shape for every outcome, including not
// asking at all, in Python's key order.
//
// `answered_by` is ADR 14's third boundary as a field, and `sources` sits
// beside the prose rather than under it for the same reason: a caller that
// has to infer which system produced a sentence will eventually infer wrong.
// `Dossier` is `any` because it is one of three things -- a fresh
// `DossierBody`, a stored row's raw bytes, or `{}` -- and the middle one must
// not pass through a Go map on its way out.
type DossierReport struct {
	AnsweredBy string        `json:"answered_by"`
	Mode       string        `json:"mode"`
	Model      string        `json:"model"`
	Slug       string        `json:"slug"`
	Commander  string        `json:"commander"`
	Asked      bool          `json:"asked"`
	Reason     string        `json:"reason"`
	Stance     StanceReadout `json:"stance"`
	Dossier    any           `json:"dossier"`
	// GeneratedAt is when it was written: ADR 19's honest substitute for a
	// freshness guarantee it cannot make, in the payload rather than only in
	// the database.
	GeneratedAt string `json:"generated_at"`
	Cached      bool   `json:"cached"`
	Usage       Usage  `json:"usage"`
	Never       string `json:"never"`
}

func dossierReport(turn *Turn, slug, commander string, effective Stance,
	body any, asked bool, reason, cachedAt string) DossierReport {
	report := DossierReport{
		AnsweredBy: "claude", Mode: ModeCommanderDossier, Slug: slug,
		Commander: commander, Asked: asked, Reason: reason,
		Stance: Describe(effective), Dossier: body, GeneratedAt: cachedAt,
		Cached: cachedAt != "", Never: DossierNever,
	}
	if body == nil {
		report.Dossier = emptyObject
	}
	if cachedAt == "" {
		report.GeneratedAt = now()
	}
	if turn != nil {
		report.Model = turn.Model
		report.Usage = Usage{InputTokens: turn.InputTokens,
			OutputTokens: turn.OutputTokens, CacheReadTokens: turn.CacheReadTokens}
	}
	return report
}

// ------------------------------------------------------------- the two halves

// DossierRequest is what a dossier is asked with, beyond the deck.
type DossierRequest struct {
	// Requested is the stance in any form Resolve accepts; nil is the deck's
	// own default.
	Requested any
	// Refresh skips the store and writes the dossier again.
	Refresh bool
	// Tier is the asking seat's model grant; empty is the house model.
	Tier string
	// Limit is the deployment's ceiling; nil reads it from the environment.
	Limit *Stance
	// Store is the `dossier_cache` table; nil caches nothing.
	Store *DossierStore
}

// DossierPlan is `DossierRequest` in Python: what `CheckDossier` settled and
// everything `RunDossier` needs.
//
// The split exists for one measured reason -- a dossier took **236 seconds**
// on the deployed instance, so everything free and everything refusable is
// decided in the request and only the Anthropic call is left for a worker.
// `Answer` set means there is nothing to call for: a stored dossier, or a
// stance that forbids calls. Those are answers, not refusals, which is why
// they travel as a result -- a job born finished -- and the client's response
// shape never forks.
type DossierPlan struct {
	Slug      string
	Facts     wire.OrderedMap
	Commander string
	OracleID  string
	// Key is the cache key, and it is exactly the right identity for "somebody
	// is already writing this" too (`jobs.Plan.Key`). Empty when the pool has
	// no oracle id, which disables both, honestly.
	Key       string
	Effective Stance
	Tier      string
	Store     *DossierStore
	Answer    *DossierReport
}

// NeedsCall reports whether anything still has to be asked of Anthropic.
func (p *DossierPlan) NeedsCall() bool { return p.Answer == nil }

// CheckDossier is `dossier.check_dossier`: everything that can be decided
// without spending anything. ErrNoCommander, ErrStanceRejected (wrapped) and
// a pool error come back to the caller, which is what keeps their 422 rather
// than collapsing them into a job error four minutes later.
func CheckDossier(ctx context.Context, conn *pool.Conn, slug string, d *deck.Deck,
	req DossierRequest) (*DossierPlan, error) {
	facts, err := DossierBrief(ctx, conn, slug, d)
	if err != nil {
		return nil, err
	}
	card, _ := kv(facts, "card").(wire.OrderedMap)
	name := asString(kv(card, "name"))
	oracleID := ""
	if id, ok := kv(card, "oracle_id").(*string); ok && id != nil {
		oracleID = *id
	}
	effective, err := Resolve(req.Requested, statusOnly{d.Status}, req.Limit)
	if err != nil {
		return nil, err
	}
	key, err := CacheKey(oracleID, req.Tier)
	if err != nil {
		return nil, err
	}
	plan := &DossierPlan{Slug: slug, Facts: facts, Commander: name, OracleID: oracleID,
		Key: key, Effective: effective, Tier: req.Tier, Store: req.Store}

	if !req.Refresh {
		if hit := req.Store.Get(ctx, key); hit != nil {
			// `asked: false`, because nothing was asked. A cache hit that
			// reported otherwise would make the usage figures read as a free
			// call rather than as no call -- the same species of dishonesty as
			// quoting a cached simulation as fresh (ADR 18).
			answer := dossierReport(nil, slug, name, effective, hit.Result, false,
				"Served from the store; no call was made. Regenerate to write it again.",
				hit.CreatedAt)
			plan.Answer = &answer
			return plan, nil
		}
	}
	if !effective.AllowsCalls() {
		answer := dossierReport(nil, slug, name, effective, nil, false,
			"The stance is off, so no call was made. Nothing else about this "+
				"deck is affected.", "")
		plan.Answer = &answer
	}
	return plan, nil
}

// DossierRun is what `RunDossier` needs beyond the plan: the conversation's
// reach and where its accounting lands.
type DossierRun struct {
	Deps   tools.Deps
	Ledger *ledger.Recorder
	// OnTurn is handed straight to the conversation, and exists because this
	// runs as a background job: four minutes reporting nothing is
	// indistinguishable from a wedged worker.
	OnTurn func(done, max int)
}

// RunDossier is `dossier.run_dossier`: make the call, check what came back,
// and store it if it survived.
func RunDossier(ctx context.Context, conn *pool.Conn, plan *DossierPlan, run DossierRun) (DossierReport, error) {
	if plan.Answer != nil {
		return *plan.Answer, nil
	}
	mode, err := GetMode(ModeCommanderDossier)
	if err != nil {
		return DossierReport{}, err
	}
	turn, err := Converse(ctx, mode, Request{
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(dossierOpening(plan.Facts))),
		},
		Stance: plan.Effective, Deps: run.Deps, Tier: plan.Tier, Ledger: run.Ledger,
		// A search, a look at what came back, a second search and the
		// write-up. The interview's six is tight once a paused turn can spend
		// one.
		MaxTurns: 8,
		OnTurn:   run.OnTurn,
	})
	if err != nil {
		return DossierReport{}, err
	}
	return readDossier(ctx, conn, plan, turn)
}

// readDossier is the half of `run_dossier` after the call: the source check,
// the competitors, the body, the store. Split from the call so the corpus can
// drive it with a Turn built by hand.
func readDossier(ctx context.Context, conn *pool.Conn, plan *DossierPlan, turn Turn) (DossierReport, error) {
	slug, name, effective := plan.Slug, plan.Commander, plan.Effective
	if turn.Refused {
		return dossierReport(&turn, slug, name, effective, nil, true,
			"The model declined to write this one.", ""), nil
	}
	var payload map[string]any
	if err := turn.Parsed(&payload); err != nil {
		//nolint:nilerr // an unreadable answer is a reported outcome, not a fault
		return dossierReport(&turn, slug, name, effective, nil, true,
			fmt.Sprintf("The answer did not parse (stop reason: %s). Nothing was stored.",
				turn.StopReason), ""), nil
	}

	claimed, _ := payload["sources"].([]any)
	sources, dropped := KeepSources(claimed, turn.Searched)
	if len(sources) == 0 {
		// ADR 19: an unsourced dossier renders as exactly the blended paragraph
		// the design rejected, arrived at by accident. Refusing is the only
		// answer that keeps the seams meaningful.
		return dossierReport(&turn, slug, name, effective, nil, true,
			"No source survived checking, so there is nothing to stand behind "+
				"the claims."+noSourceDetail(turn, dropped), ""), nil
	}
	allowed := map[string]bool{}
	for _, s := range sources {
		allowed[s.ID] = true
	}
	archetypeRaw, _ := payload["archetype"].(map[string]any)
	competitorsRaw, _ := payload["competitors"].([]any)
	competitors, competitorsDropped, err := Competitors(ctx, conn, competitorsRaw, allowed)
	if err != nil {
		return DossierReport{}, err
	}
	archetype := Section(archetypeRaw, allowed)
	body := DossierBody{
		Who: Section(payload["who"], allowed),
		Archetype: Archetype{
			Name:      strings.TrimSpace(pyStrOr(archetypeRaw["name"])),
			Prose:     archetype.Prose,
			SourceIDs: archetype.SourceIDs,
		},
		Competitors:        competitors,
		Allies:             Section(payload["allies"], allowed),
		Rivals:             Section(payload["rivals"], allowed),
		Standing:           Section(payload["standing"], allowed),
		Sources:            sources,
		SourcesDropped:     dropped,
		CompetitorsDropped: competitorsDropped,
		Searched:           len(turn.Searched),
	}
	plan.Store.Put(ctx, plan.Key, plan.OracleID, name, body)
	return dossierReport(&turn, slug, name, effective, body, true, "", ""), nil
}

// noSourceDetail is the tail both searching modes add to a no-source refusal:
// how many cited pages were not among the ones the search returned, and what
// the search itself reported -- because a search that returned nothing and a
// search that failed look identical from an empty page list.
func noSourceDetail(turn Turn, dropped int) string {
	detail := ""
	if dropped > 0 {
		detail = fmt.Sprintf(" %d cited page(s) were not among the %d the search returned.",
			dropped, len(turn.Searched))
	}
	if len(turn.SearchErrors) > 0 {
		detail += fmt.Sprintf(" The search itself reported: %s.",
			strings.Join(turn.SearchErrors, "; "))
	}
	return detail
}

// ---------------------------------------------------------- the free reading

// CachedDossier is `service.claude_dossier_cached`'s answer for a deck whose
// commander the pool knows: a stored dossier, or a payload saying there is
// none. Never calls Anthropic, so the deck page can ask for it on every load.
//
// **The row is looked up under the default tier's key**, which is what Python
// does -- `cache_key(oracle_id)` with no tier -- so a seat granted another
// model is served the house model's dossier here and its own from the POST.
// A wart, reproduced rather than fixed: the GET does not know who is asking
// in Python either, and harmonising one runtime would put the two out of
// step.
type CachedDossier struct {
	AnsweredBy  string  `json:"answered_by"`
	Slug        string  `json:"slug"`
	Commander   string  `json:"commander"`
	Dossier     any     `json:"dossier"`
	Cached      bool    `json:"cached"`
	GeneratedAt *string `json:"generated_at"`
}

// HeadlessDossier is the same route's answer when the deck has no commander
// the pool can find: **five keys and no `answered_by`**, which is Python's
// early-return shape and not a tidy one. Its own type because the difference
// is which keys exist, and one struct with `omitempty` would reproduce it only
// by accident.
type HeadlessDossier struct {
	Slug        string  `json:"slug"`
	Commander   string  `json:"commander"`
	Dossier     any     `json:"dossier"`
	Cached      bool    `json:"cached"`
	GeneratedAt *string `json:"generated_at"`
}

// ReadCachedDossier answers `GET .../dossier`: one of the two shapes above.
func ReadCachedDossier(ctx context.Context, conn *pool.Conn, slug string, d *deck.Deck,
	store *DossierStore) (any, error) {
	facts, err := DossierBrief(ctx, conn, slug, d)
	var headless *ErrNoCommander
	if errors.As(err, &headless) {
		return HeadlessDossier{Slug: slug, Commander: "", Dossier: emptyObject}, nil
	}
	if err != nil {
		return nil, err
	}
	card, _ := kv(facts, "card").(wire.OrderedMap)
	name := asString(kv(card, "name"))
	oracleID := ""
	if id, ok := kv(card, "oracle_id").(*string); ok && id != nil {
		oracleID = *id
	}
	key, err := CacheKey(oracleID, "")
	if err != nil {
		return nil, err
	}
	hit := store.Get(ctx, key)
	out := CachedDossier{Slug: slug, Commander: name, Dossier: emptyObject}
	if hit != nil {
		stamp := hit.CreatedAt
		out.AnsweredBy, out.Dossier, out.Cached, out.GeneratedAt = "claude", hit.Result, true, &stamp
	}
	return out, nil
}
