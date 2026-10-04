package library

import (
	"io/fs"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/aasquier/sylvan-library/go/internal/deck"
)

// Memo is the file tier's memory of parsed decks, keyed on each file's stamp
// -- mtime in nanoseconds plus size -- so a deck whose file has not moved is
// not parsed again.
//
// It exists because of a measurement (`library_bench_test.go`; Black's
// ledger entry of 2026-09-26): one visit to the deck shelf was ~42 ms of CPU
// and 27 MB of allocation for the deployed library's twenty-five decks, ~90%
// of it inside the YAML parse, for an answer that was identical last time.
// On the two shared cores the site runs on, the only lever that pays is not
// parsing a file that has not changed.
//
// **Whose it is decides whether it is ever consulted.** `GET /api/decks`
// builds a fresh [Library] and [FileSource] per request, so a memo held by
// the source would be correct, tested, and never once asked -- every request
// would open its own. The owner is the long-lived `*api.API`, which hands its
// one Memo down through [Resolver] into every file tier the Library builds,
// the same arrangement as the `app.db` handle it lazily holds.
//
// **The invalidation is the pool's guarantee applied to live user data.**
// Every write to a deck file is `writeAtomically` -- a fresh file renamed
// into place -- which moves the mtime and, nearly always, the size; a stamp
// that has moved is a miss, and the file is read and parsed again. The
// pool's own stated hazard applies unchanged: a file replaced by a different
// one with an identical nanosecond mtime *and* identical size would be
// served from memory. Nothing in this app writes a deck that way, and the
// test that pins the edge says so by name.
//
// **A remembered deck is shared between requests, so it is read and never
// written.** Every caller of [Source.Get] and [Source.All] reads: the route
// payloads and the Claude tools go through `internal/deckread`, whose package
// comment argues it holds no writes; the edit engine works on the file's
// text, never on a parsed deck; and the one route that reshapes a deck before
// reading it (`api.without`) copies first and says so. The SQL tier is not
// remembered and fills its own fresh parse in (`parse` in source.go).
//
// Counted from the start -- hits and misses, like `cache.Store.Counts` and
// the door's `etagCounts` -- so a test can hold the serving path to actually
// reaching the memo. All three are read out of a running instance on `GET
// /api/admin/stats/system`, behind the admin prefix: machine facts for the
// one admin, and nothing a player can see renders them (commandment 10).
//
// A nil Memo is a
// file tier that remembers nothing: every lookup misses without counting and
// every store is dropped, which is what the CLI and the door's slug-only
// readers get.
type Memo struct {
	mu      sync.Mutex
	entries map[string]memoEntry
	hits    atomic.Int64
	misses  atomic.Int64
}

// memoEntry is what one path parsed to, at the stamp it had when read.
type memoEntry struct {
	mtime int64
	size  int64
	deck  *deck.Deck
}

// NewMemo is an empty memory.
func NewMemo() *Memo { return &Memo{entries: map[string]memoEntry{}} }

// Counts is how many lookups answered from memory and how many had to read
// and parse, over this process's life. A nil Memo has never been asked.
func (m *Memo) Counts() (hits, misses int64) {
	if m == nil {
		return 0, 0
	}
	return m.hits.Load(), m.misses.Load()
}

// remembered is the deck path parsed to at exactly info's stamp, or nil. The
// stamp is compared whole: a file touched without a byte changing is a miss,
// because a moved mtime is the one signal a write leaves and nothing here
// second-guesses it.
func (m *Memo) remembered(path string, info fs.FileInfo) *deck.Deck {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	e, ok := m.entries[path]
	m.mu.Unlock()
	if ok && e.mtime == info.ModTime().UnixNano() && e.size == info.Size() {
		m.hits.Add(1)
		return e.deck
	}
	m.misses.Add(1)
	return nil
}

// learned records d as what path parsed to at info's stamp -- the stamp read
// *before* the file was, deliberately. A file rewritten between the stat and
// the read is remembered under a stamp that is no longer on disk, so the next
// visit misses and reads again: an entry that can be stale is unreachable
// rather than wrong.
func (m *Memo) learned(path string, info fs.FileInfo, d *deck.Deck) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.entries[path] = memoEntry{mtime: info.ModTime().UnixNano(), size: info.Size(), deck: d}
	m.mu.Unlock()
}

// keepOnly forgets every remembered path under root that is not in paths.
// The shelf's listing is the library's whole population, so a deck that left
// -- deleted to the crypt, its directory renamed away -- stops costing memory
// at the next visit, and a memo shared by several roots forgets only its own.
func (m *Memo) keepOnly(root string, paths []string) {
	if m == nil {
		return
	}
	keep := make(map[string]bool, len(paths))
	for _, p := range paths {
		keep[p] = true
	}
	prefix := strings.TrimSuffix(root, "/") + "/"
	m.mu.Lock()
	for p := range m.entries {
		if strings.HasPrefix(p, prefix) && !keep[p] {
			delete(m.entries, p)
		}
	}
	m.mu.Unlock()
}
