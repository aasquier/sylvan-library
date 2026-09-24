package tier3

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/textutil"
)

// The card-coverage pre-flight — does Forge
// implement every card in this deck?
//
// **This is the non-negotiable piece.** Forge implements ~99.8% of cards legal
// in Commander, which is high enough to be dangerous: a deck can lose a card
// and still produce a winner, a win rate and a turn count that all look fine.
// Every number after a silent drop is wrong, and nothing downstream can tell.
// So the question "what did Forge actually put in the deck" gets answered
// *before* a game runs, from data rather than from a log line, and [RunGames]
// will not start a simulation without it.
//
// Ground truth is `res/cardsfolder/cardsfolder.zip` in the distribution: one
// small text file per implemented card, each carrying one or more `Name:`
// lines. That is the same data the engine loads at startup, so agreeing with
// it is agreeing with Forge itself. 33,587 files yield 34,532 names in about
// two seconds, cached per (path, mtime, size) thereafter.
//
// Two things the index taught us, both of which the exporter depends on:
//
//   - Card names in the index are always **face names**. A modal DFC
//     contributes two entries ("Barkchannel Pathway" and "Tidechannel
//     Pathway"), never the combined "Barkchannel Pathway // Tidechannel
//     Pathway" that Scryfall reports. Split cards are the same.
//   - So a Scryfall-shaped `A // B` name has to be resolved to a face before
//     it can be looked up, and [Resolve] is the one place that happens. The
//     exporter writes exactly what the pre-flight verified, which is the
//     property that makes a clean report mean something.

// cardsfolder is where the card scripts live inside a distribution.
var cardsfolder = filepath.Join("res", "cardsfolder", "cardsfolder.zip")

// ErrForgeNotInstalled is `coverage.ForgeNotInstalled`: the distribution is
// missing or incomplete.
//
// A plain error rather than a fatal, for the reason `PoolRequired` is one:
// this is reachable from a job goroutine as well as from the CLI. Every
// caller in the API turns it into a 503.
var ErrForgeNotInstalled = errors.New("forge is not installed")

// NotInstalled wraps a reason as [ErrForgeNotInstalled]. Its `Error()` is the
// bare reason, never the sentinel's own words as a prefix — the lesson
// `claude.unavailable` learned when a 503's `detail` shipped with a prefix
// nobody wrote into a page that renders it verbatim.
func NotInstalled(format string, args ...any) error {
	return &notInstalled{msg: fmt.Sprintf(format, args...)}
}

type notInstalled struct{ msg string }

func (e *notInstalled) Error() string { return e.msg }
func (e *notInstalled) Is(target error) bool {
	return target == ErrForgeNotInstalled
}

// ErrWorkerNotReady is the *transient* half of [ErrForgeNotInstalled]: the
// arena is configured and would work, and right now the machine that plays the
// games is not answering. A cold image that has not finished booting, a start
// that outran its budget, a shim that has not opened its port yet.
//
// **It exists so that two very different sentences can be said.** Every one of
// these used to be a `NotInstalled`, which is a claim that this instance cannot
// play games at all — true for a checkout with no Forge in it, and false for
// the deployed arena a moment after a new image lands. The distinction the room
// needs is exactly *come back in a minute* against *this will never work here*,
// and nothing could tell them apart. See `forgeTrouble` in `internal/api`.
//
// **It answers to [ErrForgeNotInstalled] as well, on purpose.** Every caller
// that maps that sentinel onto a 503 keeps working untouched — this narrows an
// existing class rather than adding a branch to every switch that reads it, and
// a transient fault and a missing installation really do deserve the same
// status code. Only the words differ.
var ErrWorkerNotReady = errors.New("the forge worker is not answering")

// NotReady wraps a reason as [ErrWorkerNotReady]. The reason is for the log;
// the room is handed a sentence of its own.
func NotReady(format string, args ...any) error {
	return &notReady{msg: fmt.Sprintf(format, args...)}
}

type notReady struct{ msg string }

func (e *notReady) Error() string { return e.msg }
func (e *notReady) Is(target error) bool {
	return target == ErrWorkerNotReady || target == ErrForgeNotInstalled
}

// ErrCoverageFailed is `run.CoverageFailed`: a deck contains cards Forge does
// not implement, so nothing may run.
var ErrCoverageFailed = errors.New("coverage failed")

type coverageFailed struct{ msg string }

func (e *coverageFailed) Error() string        { return e.msg }
func (e *coverageFailed) Is(target error) bool { return target == ErrCoverageFailed }

// ErrResultsUntrustworthy is the refusal for a run where Forge itself reported
// a dropped card or an unloadable deck.
//
// The second of the two coverage checks, and the one that fires if a name ever
// slips past the index. Raised *after* a run, discarding results that
// otherwise look perfectly normal.
var ErrResultsUntrustworthy = errors.New("results untrustworthy")

type untrustworthy struct{ msg string }

func (e *untrustworthy) Error() string        { return e.msg }
func (e *untrustworthy) Is(target error) bool { return target == ErrResultsUntrustworthy }

// CardsfolderPath is `coverage.cardsfolder_path`.
func (s Settings) CardsfolderPath() (string, error) {
	path := filepath.Join(s.Home, cardsfolder)
	if _, err := os.Stat(path); err != nil {
		return "", NotInstalled("no Forge card data at %s -- set "+
			"MTGLAB_FORGE_HOME to an unpacked Forge distribution", path)
	}
	return path, nil
}

// indexKey is `(path, mtime, size)`: upgrading Forge in place invalidates the
// index instead of serving a stale answer — the failure mode that would matter
// most, since a Forge upgrade is exactly when coverage changes.
type indexKey struct {
	path  string
	mtime int64
	size  int64
}

// CardIndex is what one machine's distributions implement, remembered so the
// two-second scan of thirty-odd thousand card scripts is paid once rather than
// once per pre-flight.
//
// **A value rather than a package-level map, and the reason is a test.** It was
// a global behind a mutex with a `ClearIndex` for the suite to call, and the hit
// counters are the only evidence a cache is a cache — so every test that asked
// what the cache had done needed the whole process to itself, and six of them
// were serial for that and nothing else. An index is a fact about the machine
// whose card scripts it read; carrying one on [Settings] says so, and a test
// builds its own with [NewCardIndex].
//
// **A nil index remembers nothing**, which is exactly right for the [Settings]
// literal that asks once and is thrown away. [Defaults] makes one, so the
// settings the composition root loads carry an index and every copy of them
// downstream shares it — a [Settings] is copied by value all over this package
// and the pointer is what keeps one machine's answer one answer.
type CardIndex struct {
	mu    sync.Mutex
	names map[indexKey]map[string]bool
	// Hits and misses, because the measuring shelf's whole argument is that
	// a cache can be correct, tested and never once hit. There is no
	// central cache register yet -- a known, deliberate gap -- so the
	// counters live here and a test reads them.
	hits, misses int
}

// NewCardIndex is an empty index, ready to remember one machine's card scripts.
func NewCardIndex() *CardIndex {
	return &CardIndex{names: map[indexKey]map[string]bool{}}
}

// Stats reports the index's hit and miss counts. A nil index has never done
// either, which is the truth about one that remembers nothing.
func (x *CardIndex) Stats() (hits, misses int) {
	if x == nil {
		return 0, 0
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.hits, x.misses
}

// lookup answers from memory, counting the miss on its way past. The counting
// belongs here rather than at the call site so that the two numbers can never
// disagree about what a single read was.
func (x *CardIndex) lookup(key indexKey) (map[string]bool, bool) {
	if x == nil {
		return nil, false
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if names, ok := x.names[key]; ok {
		x.hits++
		return names, true
	}
	x.misses++
	return nil, false
}

// remember files a freshly read index under its key.
func (x *CardIndex) remember(key indexKey, names map[string]bool) {
	if x == nil {
		return
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.names == nil {
		x.names = map[indexKey]map[string]bool{}
	}
	x.names[key] = names
}

// ImplementedNames is every card name Forge implements, read from its own card
// scripts — from [Settings.Index] when it has been read before on this machine.
func (s Settings) ImplementedNames() (map[string]bool, error) {
	path, err := s.CardsfolderPath()
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, NotInstalled("no Forge card data at %s -- set "+
			"MTGLAB_FORGE_HOME to an unpacked Forge distribution", path)
	}
	key := indexKey{path: path, mtime: info.ModTime().Unix(), size: info.Size()}

	if cached, ok := s.Index.lookup(key); ok {
		return cached, nil
	}
	names, err := readNames(path)
	if err != nil {
		return nil, err
	}
	s.Index.remember(key, names)
	return names, nil
}

func readNames(path string) (map[string]bool, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return nil, NotInstalled("Forge card data at %s is unreadable: %v", path, err)
	}
	defer func() { _ = archive.Close() }()

	names := map[string]bool{}
	for _, info := range archive.File {
		if info.FileInfo().IsDir() || !strings.HasSuffix(info.Name, ".txt") {
			continue
		}
		if err := func() error {
			f, err := info.Open()
			if err != nil {
				// A card script that will not open costs that card, not the
				// whole pre-flight — the `errors="replace"` argument, one
				// level up.
				return nil
			}
			defer f.Close()
			body, err := io.ReadAll(f)
			if err != nil {
				return nil
			}
			// `replace` rather than strict decoding: a decoding error in one
			// card script should cost that card, not the whole pre-flight.
			// Go's []byte -> string is already lossless-to-invalid, and every
			// `Name:` line the index needs is ASCII or valid UTF-8.
			for _, line := range textutil.SplitLines(string(body)) {
				if strings.HasPrefix(line, "Name:") {
					names[textutil.Strip(line[5:])] = true
				}
			}
			return nil
		}(); err != nil {
			return nil, err
		}
	}
	return names, nil
}

// Resolve is the name Forge knows this card by, or "" if it implements no
// face.
//
// Scryfall's combined `A // B` name never appears in Forge's index, so the
// faces are tried in order. Front first: a modal DFC or a transforming
// permanent is the front face as far as a decklist is concerned, and for a
// split card either half names the same physical card.
//
// Returning the *resolved* name rather than a bool is what lets the exporter
// and this check agree by construction.
func Resolve(name string, index map[string]bool) string {
	if index[name] {
		return name
	}
	if strings.Contains(name, " // ") {
		for _, face := range strings.Split(name, " // ") {
			face = textutil.Strip(face)
			if index[face] {
				return face
			}
		}
	}
	return ""
}

// CoverageReport is what Forge would and would not put in the deck.
//
// `Missing` is the answer to the question this file exists for. It being empty
// is the only condition under which a simulation result means anything.
type CoverageReport struct {
	Slug    string
	Checked int
	// Resolved maps a deck.yaml name to the name written into the `.dck`.
	// Equal for all but double-faced and split cards.
	Resolved map[string]string
	Missing  []string
}

// OK reports whether every card in the deck is implemented.
func (r *CoverageReport) OK() bool { return len(r.Missing) == 0 }

// Renamed is the cards whose `.dck` line differs from the deck.yaml name.
func (r *CoverageReport) Renamed() [][2]string {
	keys := make([]string, 0, len(r.Resolved))
	for k := range r.Resolved {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out [][2]string
	for _, k := range keys {
		if v := r.Resolved[k]; k != v {
			out = append(out, [2]string{k, v})
		}
	}
	return out
}

// Summary is the report as the CLI and the 422 render it.
func (r *CoverageReport) Summary() string {
	if r.OK() {
		line := fmt.Sprintf("%s: all %d cards implemented by Forge", r.Slug, r.Checked)
		if n := len(r.Renamed()); n > 0 {
			line += fmt.Sprintf(" (%d resolved to a face name)", n)
		}
		return line
	}
	return fmt.Sprintf("%s: Forge does not implement %d of %d cards:\n  %s",
		r.Slug, len(r.Missing), r.Checked, strings.Join(r.Missing, "\n  "))
}

// Check pre-flights one deck. Commander and companion are checked too.
//
// Distinct names, not the 99 by quantity: thirty-six Forests missing would be
// one problem, not thirty-six, and a report should read like the fix.
func Check(d *deck.Deck, index map[string]bool) CoverageReport {
	wanted := append([]string{}, d.Commander...)
	if d.Companion != nil {
		wanted = append(wanted, *d.Companion)
	}
	for _, c := range d.Cards {
		wanted = append(wanted, c.Name)
	}

	report := CoverageReport{Slug: d.Slug, Resolved: map[string]string{}}
	seen := map[string]bool{}
	for _, name := range wanted {
		if seen[name] {
			continue
		}
		seen[name] = true
		report.Checked++
		if forgeName := Resolve(name, index); forgeName == "" {
			report.Missing = append(report.Missing, name)
		} else {
			report.Resolved[name] = forgeName
		}
	}
	return report
}

// RaiseUnlessCovered is one message format for a failed pre-flight, wherever
// it was computed.
//
// Split out of [CheckCoverage] when the worker landed (ADR 35): the shim
// computes reports on the worker machine and the client re-raises them on the
// request thread, and two hand-written copies of this message would drift the
// day one of them was edited.
func RaiseUnlessCovered(reports []CoverageReport) error {
	var broken []string
	for i := range reports {
		if !reports[i].OK() {
			broken = append(broken, reports[i].Summary())
		}
	}
	if len(broken) == 0 {
		return nil
	}
	return &coverageFailed{msg: "Forge does not implement every card, so no " +
		"result would mean anything:\n" + strings.Join(broken, "\n")}
}

// CheckCoverage pre-flights every deck. Returns [ErrCoverageFailed] if any
// card is missing.
//
// Separated from [RunGames] so a caller can pre-flight without a JVM — it
// reads a zip and needs no Java at all, which makes it the cheap check to run
// first and the one an API can run on a request thread.
func (s Settings) CheckCoverage(decks []*deck.Deck) ([]CoverageReport, error) {
	index, err := s.ImplementedNames()
	if err != nil {
		return nil, err
	}
	reports := make([]CoverageReport, 0, len(decks))
	for _, d := range decks {
		reports = append(reports, Check(d, index))
	}
	if err := RaiseUnlessCovered(reports); err != nil {
		return nil, err
	}
	return reports, nil
}
