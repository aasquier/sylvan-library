package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/deckread"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The two hosted-search modes' shared instruments: what counts as a source,
// and what counts as resting on one.
//
// The dossier and research are the only modes that read the web, and the rule
// they share is not a suspicion about the model -- it is a property of the
// wire. **A response schema suppresses the API's own citations**, so a URL in
// the payload has nothing behind it but the model's word. `KeepSources` is
// what puts something behind it: the intersection with what the search
// actually returned.
//
// Past that intersection the two modes deliberately diverge, twice, and both
// divergences are ADR 26:
//
//   - **A passage versus a finding.** `Section` narrows a dossier passage's
//     source ids and keeps the prose, which is right there: a dossier section
//     may legitimately rest entirely on the pool facts in its brief.
//     `OnlyGrounded` drops a research finding whose citations all failed,
//     because that mode HAS no brief -- its subject is whatever was asked, so
//     an uncited finding is resting on the model's recall, which is rule 1's
//     original failure with a search box drawn around it.
//   - **A card dropped versus a card labelled.** A competing commander that
//     does not resolve is a card the model invented, so the dossier drops it.
//     A card that does not resolve in research may be that -- or may be one
//     spoiled since the last `data refresh`, which is one of the three things
//     that surface exists for. The two are indistinguishable from inside
//     `get_cards`, so research does not try: it keeps both and marks them.

// plain renders a JSON-decoded value in the served vocabulary's plain text.
//
// Every instrument in this family reads its fields through this rendering
// rather than through a type assertion: an int becomes "3", a float "3.0", a
// bool "True", null "None". Go's obvious `v.(string)` answers "" for all of
// them, which turns a cited charge into a dropped one and a surviving source
// id into a missing one.
//
// Reachable only when the model breaks its own schema -- every one of these
// fields is declared `"type": "string"` with `additionalProperties: false` --
// so this is a rendering that mostly cannot fire. It is pinned anyway,
// because "mostly cannot fire" is not a property anybody checks again later,
// and because the corpus can ask the question for free.
//
// Numbers rely on `Turn.Parsed` decoding with UseNumber: `json.Number` keeps
// the literal, so "3" and "3.0" stay distinguishable instead of collapsing
// through a float64.
// Exported for `internal/api`, which needs the same rendering at the
// route boundary: a `media_type` that arrives as a list renders "['a']" and
// is refused by name. `fmt.Sprint` would render that list as "[a]", which is
// not the recorded sentence.
func Plain(v any) string { return plain(v) }

func plain(v any) string {
	switch value := v.(type) {
	case nil:
		return "None"
	case string:
		return value
	case json.Number:
		return value.String()
	case bool:
		if value {
			return "True"
		}
		return "False"
	case float64:
		// A decode that did NOT use UseNumber, which in production cannot
		// happen -- `Turn.Parsed` sets it. The literal is already lost by the
		// time this branch is reached, so `3` and `3.0` are indistinguishable
		// and both render "3": the closest honest answer rather than a promise
		// that this path matches the recorded rendering. The guard against
		// needing it is UseNumber, not this.
		return strconv.FormatFloat(value, 'f', -1, 64)
	case []any, map[string]any:
		// A list or an object renders as its literal: `['a', 1, None, True]`.
		// Reachable from one place a PERSON controls -- `CheckQuestion`'s
		// plain-text read of the question field, when a body carries a list
		// where a string was expected -- which is why the rendering is pinned
		// at all.
		return literalJSON(value)
	default:
		return fmt.Sprint(value)
	}
}

// literalJSON renders a decoded JSON value as a literal, as far as it can
// be: a list keeps its order and the served spellings (`None`, `True`,
// single-quoted strings, a number's literal). **An object's keys come out
// sorted**, because `readBody` decodes an object into a Go map and the order
// the client wrote is already gone by the time anything can render it; that
// residue is documented here and pinned nowhere, since a question that is a
// JSON object is a client bug rather than anything a person types.
func literalJSON(v any) string {
	switch x := v.(type) {
	case []any:
		parts := make([]string, 0, len(x))
		for _, item := range x {
			parts = append(parts, literalJSON(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		names := make([]string, 0, len(x))
		for name := range x {
			names = append(names, name)
		}
		sort.Strings(names)
		parts := make([]string, 0, len(names))
		for _, name := range names {
			parts = append(parts, wire.Quote(name)+": "+literalJSON(x[name]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		return literalAny(v)
	}
}

// plainGet reads a field where absence and null are different answers: an
// ABSENT key is "", while a key explicitly set to null still renders and
// becomes "None".
//
// A bare Go map lookup cannot tell the two apart -- both give nil -- which is
// the same trap the interview's `card` field guards against at the route
// boundary. It bit here too: with the two spellings otherwise correct, a
// source item carrying no `url` key at all rendered "None" and matched a page
// called None.
func plainGet(row map[string]any, key string) string {
	if _, present := row[key]; !present {
		return ""
	}
	return plain(row[key])
}

// plainOr renders a FALSY value as the empty string before the plain
// rendering ever sees it, so null never renders as "None".
//
// The family reads its fields BOTH ways and the two differ on exactly one
// input: an explicit null reaches the plain rendering through `plainGet` and
// answers "None", while through this it answers "". Which one each call site
// uses is part of the recorded shape, kept call site by call site rather than
// harmonised -- the same care the interview's `card` field needed, for the
// same reason.
func plainOr(v any) string {
	if !truthy(v) {
		return ""
	}
	return plain(v)
}

// truthy is the app's truthiness over a JSON-decoded value: null, false, an
// empty string, a zero and an empty container are all false, and everything
// else is true.
func truthy(v any) bool {
	switch value := v.(type) {
	case nil:
		return false
	case bool:
		return value
	case string:
		return value != ""
	case json.Number:
		f, err := value.Float64()
		return err == nil && f != 0
	case float64:
		return value != 0
	case []any:
		return len(value) > 0
	case map[string]any:
		return len(value) > 0
	default:
		return true
	}
}

// MaxFindings caps a research answer's findings. The reader has to read every
// one; the dossier and the slot argument cap their lists for the same reason.
const MaxFindings = 6

// MaxResearchCards is enough to name the cards a question is actually about.
// `get_cards` takes 100 at a time; this is the ceiling on what comes *back* to
// the reader, who has to look at every one of them.
const MaxResearchCards = 12

// Source is one cited page that survived the check, in the recorded key
// order.
type Source struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// CanonicalURL reduces a URL to what makes two of them the same page.
//
// Shared because the theme interview checks a fun fact's source the same way
// (ADR 20), and two copies of this would be two chances to disagree about
// whether a trailing slash is a different page.
//
// **The path's case is preserved when there is a scheme and lowercased when
// there is not**, which looks like an inconsistency and is simply the
// recorded rule: the split branch lowercases scheme and host only. Pinned
// rather than tidied -- the frozen corpus rests on this exact matcher, and a
// tidier rule would be a different one, not a better one.
func CanonicalURL(url string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(url), "/")
	if scheme, rest, found := strings.Cut(trimmed, "://"); found {
		host, path, hasPath := strings.Cut(rest, "/")
		out := strings.ToLower(scheme) + "://" + strings.ToLower(host)
		if hasPath {
			out += "/" + path
		}
		return out
	}
	return strings.ToLower(trimmed)
}

// KeepSources keeps the sources the search actually returned; counts the rest.
//
// Matching ignores a trailing slash and is case-insensitive on the host,
// because those differ without meaning anything. It does **not** ignore the
// path -- a different page on the same site is a different page, and treating
// a site as a source is how a citation stops meaning anything.
func KeepSources(claimed []any, searched []Page) ([]Source, int) {
	byURL := map[string]Page{}
	for _, p := range searched {
		byURL[CanonicalURL(p.URL)] = p
	}
	kept := []Source{}
	dropped := 0
	for _, item := range claimed {
		row, ok := item.(map[string]any)
		if !ok {
			dropped++
			continue
		}
		url := strings.TrimSpace(plainGet(row, "url"))
		match, found := byURL[CanonicalURL(url)]
		if url == "" || !found {
			dropped++
			continue
		}
		id := strings.TrimSpace(plainOr(row["id"]))
		if id == "" {
			id = url
		}
		// The search's own title, not the model's. One of them is a fact about
		// the page and the other is a description of it.
		title := match.Title
		if title == "" {
			title = strings.TrimSpace(plainOr(row["title"]))
		}
		kept = append(kept, Source{ID: id, Title: title, URL: url})
	}
	return kept, dropped
}

// Passage is one dossier section: prose, and the citations that survived.
type Passage struct {
	Prose     string   `json:"prose"`
	SourceIDs []string `json:"source_ids"`
}

// Section narrows a passage's citations to the ones that survived, and keeps
// the prose either way. See the package note: a dossier section may rest on
// its brief, so an uncited one is not automatically ungrounded.
func Section(raw any, allowed map[string]bool) Passage {
	row, _ := raw.(map[string]any)
	return Passage{
		Prose:     strings.TrimSpace(plainOr(row["prose"])),
		SourceIDs: survivingIDs(row["source_ids"], allowed),
	}
}

// Finding is one research claim and the sources it rests on.
type Finding struct {
	Claim     string   `json:"claim"`
	SourceIDs []string `json:"source_ids"`
}

// OnlyGrounded keeps the findings that cite a surviving source; counts the
// rest. The family's third instrument, aimed one step past where the dossier
// aims it -- see the package note.
func OnlyGrounded(items []any, allowed map[string]bool) ([]Finding, int) {
	kept := []Finding{}
	dropped := 0
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			dropped++
			continue
		}
		claim := strings.TrimSpace(plainGet(row, "claim"))
		ids := survivingIDs(row["source_ids"], allowed)
		if claim == "" || len(ids) == 0 {
			dropped++
			continue
		}
		kept = append(kept, Finding{Claim: claim, SourceIDs: ids})
	}
	return kept, dropped
}

// survivingIDs keeps the ids that survived the source check. Each id goes
// through the plain rendering first and is filtered after, so a numeric id
// matches a string one -- a source cited as `3` survives a source declared as
// "3".
func survivingIDs(raw any, allowed map[string]bool) []string {
	list, _ := raw.([]any)
	out := []string{}
	for _, item := range list {
		id := plain(item)
		if allowed[id] {
			out = append(out, id)
		}
	}
	return out
}

// Competitor is a rival commander as the dossier serves it.
//
// **`OracleText` is last**, after `LegalCommander`, and that is the recorded
// order rather than a tidy one: the field arrived after its neighbours, for a
// reason worth keeping -- a first run described Trostani Discordant as making
// Food tokens (she makes 1/1 Soldiers), so the pool's own text is carried and
// the reader can see the card. Field order is the wire here, so last is where
// it stays.
type Competitor struct {
	Name           string   `json:"name"`
	Prose          string   `json:"prose"`
	SourceIDs      []string `json:"source_ids"`
	ManaCost       *string  `json:"mana_cost"`
	TypeLine       *string  `json:"type_line"`
	ColorIdentity  []string `json:"color_identity"`
	Image          *string  `json:"image"`
	ArtCrop        *string  `json:"art_crop"`
	LegalCommander bool     `json:"legal_commander"`
	OracleText     *string  `json:"oracle_text"`
}

// Competitors resolves the rivals against the pool, or drops them.
//
// Indexed under both spellings, like `ResolveCards` below and
// `ResolveAlternatives`: a double-faced card resolves from either face and
// comes back under its full `A // B` name, and a model names a competitor by
// the face it knows. **Until 2026-08-23 this indexed the pool's spelling
// alone, and that was inherited rather than chosen** -- so "Ajani, Nacatl
// Pariah" resolved in research and was dropped here as a card the model had
// invented. Measured against the real pool, raised with Aaron, and fixed in
// one deliberate change with the corpus re-recorded to match; commanders are
// exactly the population most likely to be double-faced.
func Competitors(ctx context.Context, conn *pool.Conn, raw []any,
	allowed map[string]bool) ([]Competitor, int, error) {
	items := []map[string]any{}
	names := []string{}
	for _, r := range raw {
		row, ok := r.(map[string]any)
		if !ok {
			continue
		}
		items = append(items, row)
		names = append(names, strings.TrimSpace(plainOr(row["card"])))
	}
	wanted := []string{}
	for _, n := range names {
		if n != "" {
			wanted = append(wanted, n)
		}
	}
	if len(wanted) == 0 {
		// Every well-formed item counts as dropped -- items with no `card` at
		// all included. The recorded count is the item count, not the count of
		// resolvable names.
		return []Competitor{}, len(items), nil
	}

	looked, err := deckread.CardsNamed(ctx, conn, wanted)
	if err != nil {
		return nil, 0, err
	}
	found := map[string]deckread.NamedCard{}
	for _, c := range looked.Cards {
		found[strings.ToLower(c.Name)] = c
		if c.AskedAs != nil && *c.AskedAs != "" {
			found[strings.ToLower(*c.AskedAs)] = c
		}
	}

	out := []Competitor{}
	dropped := 0
	for i, row := range items {
		record, ok := found[strings.ToLower(names[i])]
		if !ok {
			dropped++
			continue
		}
		section := Section(row, allowed)
		out = append(out, Competitor{
			Name: record.Name, Prose: section.Prose, SourceIDs: section.SourceIDs,
			ManaCost: record.ManaCost, TypeLine: record.TypeLine,
			ColorIdentity: record.ColorIdentity, Image: record.Image,
			ArtCrop: record.ArtCrop, LegalCommander: record.LegalCommander,
			OracleText: record.OracleText,
		})
	}
	return out, dropped, nil
}

// ResearchCard is a named card as research serves it, resolved.
//
// A DIFFERENT field order from Competitor for the same facts -- `oracle_text`
// is fifth here and last there. Both orders are recorded, and field order is
// the wire, so the two cannot share a type however similar they look.
type ResearchCard struct {
	Name           string   `json:"name"`
	InPool         bool     `json:"in_pool"`
	ManaCost       *string  `json:"mana_cost"`
	TypeLine       *string  `json:"type_line"`
	OracleText     *string  `json:"oracle_text"`
	ColorIdentity  []string `json:"color_identity"`
	Image          *string  `json:"image"`
	ArtCrop        *string  `json:"art_crop"`
	LegalCommander bool     `json:"legal_commander"`
}

// UnresolvedCard is a card the pool does not have: **two keys and no more**.
//
// Its own type because that is the shape, not because it is convenient. A
// single struct with `omitempty` everywhere would reproduce these two keys only
// by accident -- and would stop the moment a resolved card had a null field.
// `in_pool: false` is what a client renders differently, and `name` is the
// model's own spelling because there is no other spelling to offer.
type UnresolvedCard struct {
	Name   string `json:"name"`
	InPool bool   `json:"in_pool"`
}

// ResolveCards resolves every named card against the pool and LABELS what is
// missing rather than dropping it -- the deliberate opposite of Competitors,
// and the reason ADR 26 exists. Returns the cards in the order named (as `any`,
// since a resolved one and a missing one are different shapes) and how many the
// pool did not have.
func ResolveCards(ctx context.Context, conn *pool.Conn, names []any,
	maxCards int) ([]any, int, error) {
	wanted := []string{}
	seen := map[string]bool{}
	for _, raw := range names {
		name := strings.TrimSpace(plainOr(raw))
		key := strings.ToLower(name)
		if name != "" && !seen[key] {
			seen[key] = true
			wanted = append(wanted, name)
		}
	}
	if len(wanted) == 0 {
		return []any{}, 0, nil
	}
	if len(wanted) > maxCards {
		wanted = wanted[:maxCards]
	}

	looked, err := deckread.CardsNamed(ctx, conn, wanted)
	if err != nil {
		return nil, 0, err
	}
	// Indexed under both spellings, as Competitors above is: a double-faced
	// card resolves from either face and comes back under its full `A // B`
	// name.
	byKey := map[string]deckread.NamedCard{}
	for _, c := range looked.Cards {
		byKey[strings.ToLower(c.Name)] = c
		if c.AskedAs != nil && *c.AskedAs != "" {
			byKey[strings.ToLower(*c.AskedAs)] = c
		}
	}

	out := []any{}
	already := map[string]bool{}
	unresolved := 0
	for _, name := range wanted {
		record, ok := byKey[strings.ToLower(name)]
		if !ok {
			unresolved++
			out = append(out, UnresolvedCard{Name: name, InPool: false})
			continue
		}
		if already[record.Name] {
			continue
		}
		already[record.Name] = true
		out = append(out, ResearchCard{
			Name: record.Name, InPool: true,
			ManaCost: record.ManaCost, TypeLine: record.TypeLine,
			OracleText: record.OracleText, ColorIdentity: record.ColorIdentity,
			Image: record.Image, ArtCrop: record.ArtCrop,
			LegalCommander: record.LegalCommander,
		})
	}
	return out, unresolved, nil
}
