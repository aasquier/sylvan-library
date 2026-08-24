// Package library is where decks come from, and which
// decks a caller may see (ADR 22). A Source is **one person's library**,
// slug-keyed; a Library resolves the owner segment of
// `/api/decks/{owner}/{slug}` to a Source, so a handler asks a source for a
// slug and never learns that ownership exists -- the visibility rule is
// enforced by which source you get, not by branching in a handler.
//
//	caller is the owner        -> their tier, writable
//	caller is somebody else    -> their tier, shared decks only, read-only
//	no such account            -> nothing at all
//
// A private deck is **absent** from the source, so every verb against it is
// ErrNotFound -> 404 before writability is consulted; a shared deck is
// present but read-only (`Writable` is reported here so the wire's
// `writable` field can say what it says today; the refusal itself belongs
// to the write layer). Hence the rule: 403 is only ever an answer about a
// deck the caller can already read.
//
// This file is the read side. The file tier reads `<root>/<slug>/deck.yaml`
// and `<slug>/artifacts/`; the SQL tier reads `user_decks` and
// `user_deck_artifacts` in `app.db`, opened read-only -- the write half
// lives in write.go, on its own handle.
package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/artifacts"
	"github.com/aasquier/sylvan-library/go/internal/deck"
)

// ErrNotFound is the missing-deck refusal, typed so the route layer can
// turn it into a 404 without guessing. The text is what the 404 says:
// `no deck '<x>'`.
type ErrNotFound struct{ Slug string }

func (e ErrNotFound) Error() string { return "no deck '" + e.Slug + "'" }

// ErrArtifactNotFound is a deliverable this deck has not got -- never
// built, built with no `swaps.md` to write, or a name that is not a
// deliverable at all; one answer, on purpose.
type ErrArtifactNotFound struct{ Name string }

func (e ErrArtifactNotFound) Error() string { return "no artifact '" + e.Name + "'" }

// Deliverables is the served set and the path-traversal guard in one.
// `Snapshot` is deliberately outside it.
//
// Both are the renderer's, re-exported here rather than restated: the set a
// reader may ask for and the set a rebuild prunes have to be the same list,
// and the only way to guarantee that is for there to be one.
var Deliverables = artifacts.Deliverables

// Snapshot is the build's own baseline, the renderer's name re-exported.
const Snapshot = artifacts.Snapshot

func isDeliverable(name string) bool { return artifacts.IsDeliverable(name) }

// Artifact is one generated deliverable, as the library holds it.
type Artifact struct {
	Name    string
	Size    int64
	BuiltAt time.Time
}

// Source is the read side: the questions the API asks about a
// person's decks.
type Source interface {
	// Slugs is every deck's slug, without parsing any of them.
	Slugs(ctx context.Context) ([]string, error)
	// Get is one deck, or ErrNotFound.
	Get(ctx context.Context, slug string) (*deck.Deck, error)
	// All is every deck, parsed, in a stable order.
	All(ctx context.Context) ([]*deck.Deck, error)
	// ReadText is the deck's YAML, verbatim.
	ReadText(ctx context.Context, slug string) (string, error)
	// Artifacts is which deliverables this deck has, in Deliverables order.
	Artifacts(ctx context.Context, slug string) ([]Artifact, error)
	// ReadArtifact is one deliverable's text; ErrArtifactNotFound or ErrNotFound.
	ReadArtifact(ctx context.Context, slug, name string) (string, error)
	// ReadBaseline is the last build's snapshot, or "" with false.
	ReadBaseline(ctx context.Context, slug string) (string, bool, error)
	// Writable is whether this caller may edit these decks -- reported, so
	// the UI can hide a control; the refusal itself is the write layer's.
	Writable() bool
	// OwnerID is nil for the file tier, the account for the SQL tier --
	// how the activity log keys a deck.
	OwnerID() *int64
}

// ---- the file tier ---------------------------------------------------------

// FileSource is `FileDeckSource`: decks as `<root>/<slug>/deck.yaml`.
type FileSource struct {
	Root     string
	writable bool
}

// NewFileSource is a file tier over root. `writable` is the caller's
// relationship to the library, decided by Library and never here.
func NewFileSource(root string, writable bool) *FileSource {
	return &FileSource{Root: root, writable: writable}
}

// deckPaths is `config.deck_paths`: every `*/deck.yaml`, sorted, an
// underscore-prefixed directory being scaffolding and `.trash` invisible
// to the glob.
func (f *FileSource) deckPaths() ([]string, error) {
	entries, err := os.ReadDir(f.Root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, err
	}
	out := []string{}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "_") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(f.Root, e.Name(), "deck.yaml")
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (f *FileSource) Slugs(context.Context) ([]string, error) {
	paths, err := f.deckPaths()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		out = append(out, filepath.Base(filepath.Dir(p)))
	}
	return out, nil
}

// path is `<root>/<slug>/deck.yaml`, or ErrNotFound. A slug is one path
// segment by construction -- the door proxies any request whose path is
// not already canonical, so `..` never arrives -- and it is held to that
// here too, so a source used from somewhere other than a route cannot be
// walked out of its root: anything with a separator, or that is only dots,
// is simply not a deck.
func (f *FileSource) path(slug string) (string, error) {
	if slug == "" || strings.Trim(slug, ".") == "" || strings.ContainsAny(slug, `/\`) {
		return "", ErrNotFound{Slug: slug}
	}
	p := filepath.Join(f.Root, slug, "deck.yaml")
	if info, err := os.Stat(p); err != nil || !info.Mode().IsRegular() {
		return "", ErrNotFound{Slug: slug}
	}
	return p, nil
}

func (f *FileSource) Get(ctx context.Context, slug string) (*deck.Deck, error) {
	text, err := f.ReadText(ctx, slug)
	if err != nil {
		return nil, err
	}
	return deck.FromText(text, slug)
}

func (f *FileSource) All(context.Context) ([]*deck.Deck, error) {
	paths, err := f.deckPaths()
	if err != nil {
		return nil, err
	}
	out := make([]*deck.Deck, 0, len(paths))
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		d, err := deck.FromText(string(raw), filepath.Base(filepath.Dir(p)))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		out = append(out, d)
	}
	return out, nil
}

func (f *FileSource) ReadText(_ context.Context, slug string) (string, error) {
	p, err := f.path(slug)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (f *FileSource) artifactDir(slug string) (string, error) {
	p, err := f.path(slug) // ErrNotFound first: "never built" and "no such deck" differ
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(p), "artifacts"), nil
}

func (f *FileSource) Artifacts(_ context.Context, slug string) ([]Artifact, error) {
	dir, err := f.artifactDir(slug)
	if err != nil {
		return nil, err
	}
	out := []Artifact{}
	for _, name := range Deliverables {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		out = append(out, Artifact{Name: name, Size: info.Size(), BuiltAt: info.ModTime().UTC()})
	}
	return out, nil
}

func (f *FileSource) ReadArtifact(_ context.Context, slug, name string) (string, error) {
	// Membership first, and it is the only check that matters: a name that
	// is not one of the five never becomes a path at all.
	if !isDeliverable(name) {
		return "", ErrArtifactNotFound{Name: name}
	}
	dir, err := f.artifactDir(slug)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return "", ErrArtifactNotFound{Name: name}
	}
	return string(raw), nil
}

func (f *FileSource) ReadBaseline(_ context.Context, slug string) (string, bool, error) {
	dir, err := f.artifactDir(slug)
	if err != nil {
		return "", false, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, Snapshot))
	if err != nil {
		return "", false, nil //nolint:nilerr // a missing snapshot is ordinary: a deck built for the first time has none
	}
	return string(raw), true, nil
}

func (f *FileSource) Writable() bool { return f.writable }

func (f *FileSource) OwnerID() *int64 { return nil }

// ---- the SQL tier ----------------------------------------------------------

// SQLSource is `SqlDeckSource`: the decks belonging to one account, as rows
// in `app.db`. `sharedOnly` is applied in the WHERE clause, so a private deck
// cannot be leaked by a code path that forgot to filter -- the row never
// arrives. ADR 22's 404 is that clause.
type SQLSource struct {
	db *sql.DB
	// write is the read-write handle, separate from `db` on purpose: every
	// read goes through a pool of read-only connections that block nobody,
	// and the one write path is serialised behind a single connection. Nil on
	// an instance with no app.db, where a write refuses rather than panics.
	write      *sql.DB
	ownerID    int64
	writable   bool
	sharedOnly bool
}

// NewSQLSource is one owner's SQL tier over an app.db handle. `write` may be
// nil, which makes the tier read-only however `writable` is set -- the honest
// answer on an instance whose database was opened for reading only.
func NewSQLSource(db, write *sql.DB, ownerID int64, writable, sharedOnly bool) *SQLSource {
	return &SQLSource{db: db, write: write, ownerID: ownerID,
		writable: writable, sharedOnly: sharedOnly}
}

func (s *SQLSource) where() (string, []any) {
	sql := "owner_id = ? AND deleted_at IS NULL"
	args := []any{s.ownerID}
	if s.sharedOnly {
		sql += " AND shared = 1"
	}
	return sql, args
}

func (s *SQLSource) Slugs(ctx context.Context) ([]string, error) {
	where, args := s.where()
	rows, err := s.db.QueryContext(ctx, "SELECT slug FROM user_decks WHERE "+where+" ORDER BY slug", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		out = append(out, slug)
	}
	return out, rows.Err()
}

type row struct {
	id     int64
	slug   string
	yaml   string
	shared bool
}

func (s *SQLSource) row(ctx context.Context, slug string) (row, error) {
	where, args := s.where()
	var r row
	var shared int64
	err := s.db.QueryRowContext(ctx, "SELECT id, slug, yaml, shared FROM user_decks WHERE "+where+" AND slug = ?",
		append(args, slug)...).Scan(&r.id, &r.slug, &r.yaml, &shared)
	if errors.Is(err, sql.ErrNoRows) {
		return row{}, ErrNotFound{Slug: slug}
	}
	if err != nil {
		return row{}, err
	}
	r.shared = shared != 0
	return r, nil
}

// parse is `SqlDeckSource._parse`: the row's slug is the identity, and the
// `shared` column is the truth for this tier, written over whatever the
// YAML says.
func parse(r row) (*deck.Deck, error) {
	d, err := deck.FromText(r.yaml, r.slug)
	if err != nil {
		return nil, err
	}
	d.Shared = r.shared
	return d, nil
}

func (s *SQLSource) Get(ctx context.Context, slug string) (*deck.Deck, error) {
	r, err := s.row(ctx, slug)
	if err != nil {
		return nil, err
	}
	return parse(r)
}

func (s *SQLSource) All(ctx context.Context) ([]*deck.Deck, error) {
	where, args := s.where()
	rows, err := s.db.QueryContext(ctx, "SELECT id, slug, yaml, shared FROM user_decks WHERE "+where+" ORDER BY slug", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*deck.Deck{}
	for rows.Next() {
		var r row
		var shared int64
		if err := rows.Scan(&r.id, &r.slug, &r.yaml, &shared); err != nil {
			return nil, err
		}
		r.shared = shared != 0
		d, err := parse(r)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *SQLSource) ReadText(ctx context.Context, slug string) (string, error) {
	r, err := s.row(ctx, slug)
	if err != nil {
		return "", err
	}
	return r.yaml, nil
}

func (s *SQLSource) Artifacts(ctx context.Context, slug string) ([]Artifact, error) {
	r, err := s.row(ctx, slug)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT name, LENGTH(CAST(body AS BLOB)) AS size, built_at FROM user_deck_artifacts WHERE deck_id = ?", r.id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	held := map[string]Artifact{}
	for rows.Next() {
		var a Artifact
		var built string
		if err := rows.Scan(&a.Name, &a.Size, &built); err != nil {
			return nil, err
		}
		a.BuiltAt, err = parseISO(built)
		if err != nil {
			return nil, err
		}
		held[a.Name] = a
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []Artifact{}
	for _, name := range Deliverables { // the reader's order, not the database's
		if a, ok := held[name]; ok {
			out = append(out, a)
		}
	}
	return out, nil
}

func (s *SQLSource) body(ctx context.Context, slug, name string) (string, error) {
	r, err := s.row(ctx, slug)
	if err != nil {
		return "", err
	}
	var text string
	err = s.db.QueryRowContext(ctx, "SELECT body FROM user_deck_artifacts WHERE deck_id = ? AND name = ?", r.id, name).Scan(&text)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrArtifactNotFound{Name: name}
	}
	return text, err
}

func (s *SQLSource) ReadArtifact(ctx context.Context, slug, name string) (string, error) {
	if !isDeliverable(name) {
		return "", ErrArtifactNotFound{Name: name}
	}
	return s.body(ctx, slug, name)
}

func (s *SQLSource) ReadBaseline(ctx context.Context, slug string) (string, bool, error) {
	text, err := s.body(ctx, slug, Snapshot)
	var missing ErrArtifactNotFound
	if errors.As(err, &missing) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return text, true, nil
}

func (s *SQLSource) Writable() bool { return s.writable }

func (s *SQLSource) OwnerID() *int64 { id := s.ownerID; return &id }

// parseISO reads `datetime.isoformat(timespec="seconds")` and the forms
// beside it.
func parseISO(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("not an ISO 8601 timestamp: %q", s)
}

// ---- the shared-only view of the file tier ---------------------------------

// SharedOnly is `_SharedOnly`: a read-only view hiding whatever its inner
// source does not share. Wraps the file tier only; the SQL tier filters in
// its own WHERE clause.
type SharedOnly struct{ inner Source }

// NewSharedOnly wraps a source built read-only.
func NewSharedOnly(inner Source) *SharedOnly { return &SharedOnly{inner: inner} }

func (v *SharedOnly) visible(ctx context.Context, slug string) error {
	d, err := v.inner.Get(ctx, slug)
	if err != nil {
		return err
	}
	if !d.Shared {
		return ErrNotFound{Slug: slug}
	}
	return nil
}

func (v *SharedOnly) Slugs(ctx context.Context) ([]string, error) {
	decks, err := v.All(ctx)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, d := range decks {
		out = append(out, d.Slug)
	}
	return out, nil
}

func (v *SharedOnly) All(ctx context.Context) ([]*deck.Deck, error) {
	decks, err := v.inner.All(ctx)
	if err != nil {
		return nil, err
	}
	out := []*deck.Deck{}
	for _, d := range decks {
		if d.Shared {
			out = append(out, d)
		}
	}
	return out, nil
}

func (v *SharedOnly) Get(ctx context.Context, slug string) (*deck.Deck, error) {
	if err := v.visible(ctx, slug); err != nil {
		return nil, err
	}
	return v.inner.Get(ctx, slug)
}

func (v *SharedOnly) ReadText(ctx context.Context, slug string) (string, error) {
	if err := v.visible(ctx, slug); err != nil {
		return "", err
	}
	return v.inner.ReadText(ctx, slug)
}

func (v *SharedOnly) Artifacts(ctx context.Context, slug string) ([]Artifact, error) {
	if err := v.visible(ctx, slug); err != nil {
		return nil, err
	}
	return v.inner.Artifacts(ctx, slug)
}

func (v *SharedOnly) ReadArtifact(ctx context.Context, slug, name string) (string, error) {
	if err := v.visible(ctx, slug); err != nil {
		return "", err
	}
	return v.inner.ReadArtifact(ctx, slug, name)
}

func (v *SharedOnly) ReadBaseline(ctx context.Context, slug string) (string, bool, error) {
	if err := v.visible(ctx, slug); err != nil {
		return "", false, err
	}
	return v.inner.ReadBaseline(ctx, slug)
}

func (v *SharedOnly) Writable() bool { return false }

func (v *SharedOnly) OwnerID() *int64 { return v.inner.OwnerID() }
