// Package cards is the reader for the photographed card (ADR 34's
// deterministic half) -- a set code and a collector number off the
// bottom-left corner is a *lookup* and resolves; a title is a *similarity*
// and only ever offers five names, because the measured scores of right and
// wrong answers overlap and no threshold can separate them. ADR 34 holds
// the argument.
package cards

import (
	"context"
	"regexp"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// Candidates is how many names a title sighting offers; the knee is three.
const Candidates = 5

// MaxSightings bounds one request's work: a fanned spread is ten or so.
const MaxSightings = 40

// MaxTitle is the longest title considered; past it is a misread card.
const MaxTitle = 200

// isCard: cards a Commander deck could contain, banned ones included, and
// none of the emblems, schemes, planes and vanguards that cannot be in one.
const isCard = "json_extract_string(legalities, 'commander') IN ('legal', 'banned')"

var (
	faceNumber   = regexp.MustCompile(`^\s*([^/\s]+)`)
	setCodeRe    = regexp.MustCompile(`^[A-Za-z0-9]{2,6}$`)
	faceNumberIn = regexp.MustCompile(`(\d{1,4}[a-z]?)`)
	yearRe       = regexp.MustCompile(`^(19|20)\d\d$`)
	tokenSplit   = regexp.MustCompile(`[^0-9A-Za-z]+`)
)

// Sighting is what one capture thought it saw. Every field is optional and
// independently unreliable; "" is the absent value.
type Sighting struct {
	SetCode         string
	CollectorNumber string
	Title           string
	// The bottom-left block exactly as the reader saw it. Preferred over the
	// two fields above when present -- see FromCorner.
	Corner string
}

// Candidate is a name the title might be, and how alike the strings are.
// Carried so a client can order and shade a list, never so anything can
// threshold on it.
type Candidate struct {
	Name  string
	Score float64
}

// Reading is what the pool made of one sighting. `Resolved` is only ever a
// printing lookup's answer; `Via` is printing, title, or nothing.
type Reading struct {
	Resolved   string
	Via        string
	Candidates []Candidate
}

// SetCodes is every set code the pool knows, upper-cased. Read once per
// Read and handed down, never memoised on the package: the pool can be
// pointed elsewhere under a process.
func SetCodes(ctx context.Context, c *pool.Conn) (map[string]bool, error) {
	rows, err := c.DB().QueryContext(ctx,
		"SELECT DISTINCT upper(set_code) FROM printings WHERE set_code IS NOT NULL")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		out[code] = true
	}
	return out, rows.Err()
}

// FromCorner reads a set code and a collector number out of the raw corner
// block -- server-side because the answer needs the pool's 986 real set
// codes: the longest real prefix of a line's *first* token is the set code
// (the artist after it never gets a vote), and a token starting with a digit
// is the collector-number line, never a set code. A four-digit year is the
// copyright line bleeding into the crop.
func FromCorner(text string, codes map[string]bool) Sighting {
	if text == "" {
		return Sighting{Corner: text}
	}
	var number, code string
	lines := strings.Split(text, "\n")
	if len(lines) > 8 {
		lines = lines[:8]
	}
	for _, line := range lines {
		tokens := []string{}
		for _, t := range tokenSplit.Split(line, -1) {
			if t != "" {
				tokens = append(tokens, t)
			}
		}
		if len(tokens) == 0 {
			continue
		}
		if number == "" {
			for _, token := range tokens {
				if m := faceNumberIn.FindStringSubmatch(token); m != nil && !yearRe.MatchString(m[1]) {
					number = m[1]
					break
				}
			}
		}
		if code == "" {
			first := strings.ToUpper(tokens[0])
			if first[0] < '0' || first[0] > '9' {
				for size := 6; size > 1; size-- {
					if len(first) >= size && codes[first[:size]] {
						code = first[:size]
						break
					}
				}
			}
		}
	}
	return Sighting{SetCode: code, CollectorNumber: number, Corner: text}
}

// FaceNumber is `284/281` -> `284`: the pool stores the numerator alone.
func FaceNumber(text string) string {
	if text == "" {
		return ""
	}
	m := faceNumber.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	return m[1]
}

// ByPrinting is the corner tier: one row out of `printings`, or nothing.
// Zero-padding is tolerated on both sides; a pair naming two different cards
// resolves to neither, because this tier's whole claim is that it has no
// judgement in it.
func ByPrinting(ctx context.Context, c *pool.Conn, setCode, collectorNumber string) (string, error) {
	number := FaceNumber(collectorNumber)
	if setCode == "" || number == "" || !setCodeRe.MatchString(setCode) {
		return "", nil
	}
	rows, err := c.DB().QueryContext(ctx,
		`SELECT DISTINCT name FROM printings
		 WHERE lower(set_code) = lower(?)
		   AND (collector_number = ? OR ltrim(collector_number, '0') = ltrim(?, '0'))
		 LIMIT 2`, setCode, number, number)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return "", err
		}
		names = append(names, n)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(names) == 1 {
		return names[0], nil
	}
	return "", nil
}

// ByTitle is the title tier: a ranked shortlist, and never an answer.
// Scored against the whole name *and* the front face, the better of the two,
// because the camera sees one side of a double-faced card.
func ByTitle(ctx context.Context, c *pool.Conn, title string, limit int) ([]Candidate, error) {
	out := []Candidate{}
	text := strings.TrimSpace(title)
	if text == "" {
		return out, nil
	}
	if runes := []rune(text); len(runes) > MaxTitle {
		text = string(runes[:MaxTitle])
	}
	if limit < 1 {
		limit = 1
	}
	rows, err := c.DB().QueryContext(ctx,
		`SELECT name, greatest(
		           jaro_winkler_similarity(lower(name), lower(?)),
		           jaro_winkler_similarity(lower(split_part(name, ' // ', 1)), lower(?))) AS score
		 FROM oracle_cards WHERE `+isCard+`
		 ORDER BY score DESC, edhrec_rank NULLS LAST
		 LIMIT ?`, text, text, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var cand Candidate
		if err := rows.Scan(&cand.Name, &cand.Score); err != nil {
			return nil, err
		}
		out = append(out, cand)
	}
	return out, rows.Err()
}

// Read reads a batch of captures, in the order they were taken: one Reading
// per Sighting, always, so a client can hold the two lists side by side.
func Read(ctx context.Context, c *pool.Conn, sightings []Sighting) ([]Reading, error) {
	batch := sightings
	if len(batch) > MaxSightings {
		batch = batch[:MaxSightings]
	}
	// One scan for the whole batch, and only when a corner needs reading.
	var codes map[string]bool
	for _, s := range batch {
		if s.Corner != "" && s.SetCode == "" {
			var err error
			if codes, err = SetCodes(ctx, c); err != nil {
				return nil, err
			}
			break
		}
	}
	out := make([]Reading, 0, len(batch))
	for _, raw := range batch {
		sighting := raw
		if raw.Corner != "" && raw.SetCode == "" {
			sighting = FromCorner(raw.Corner, codes)
		}
		name, err := ByPrinting(ctx, c, sighting.SetCode, sighting.CollectorNumber)
		if err != nil {
			return nil, err
		}
		if name != "" {
			out = append(out, Reading{Resolved: name, Via: "printing", Candidates: []Candidate{}})
			continue
		}
		candidates, err := ByTitle(ctx, c, raw.Title, Candidates)
		if err != nil {
			return nil, err
		}
		via := "nothing"
		if len(candidates) > 0 {
			via = "title"
		}
		out = append(out, Reading{Via: via, Candidates: candidates})
	}
	return out, nil
}
