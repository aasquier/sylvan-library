// Package gate is `decks/validate.py`, `decks/companion.py` and
// `decks/partners.py`: the checks a deck must pass before anything is
// emitted, over a parsed deck and the pool's records -- pure functions,
// no database, the same shape the Python gate keeps so that the same tests
// hold it (docs/go-migration/PLAN.md, Phase 3: validate agrees with Python
// case-for-case). Partners arrived first, because `/api/cards/search` with
// `commanders_only` asks `can_be_commander` of every hit.
package gate

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// The pairing abilities, as `decks/partners.py` names them.
const (
	Partner           = "partner"
	PartnerWith       = "partner-with"
	Labeled           = "labeled-partner"
	BackgroundChooser = "background-chooser"
	DoctorsCompanion  = "doctors-companion"
)

// A card's own type line marks these; they carry no pairing ability of
// their own and can only ever be the *second* commander.
const (
	BackgroundType = "Background"
	DoctorType     = "Time Lord Doctor"
)

// Pairing is the pairing ability printed on a card.
type Pairing struct {
	Kind        string
	Label       string // Labeled: the text after "Partner—"
	PartnerName string // PartnerWith: the specific card named
}

// Describe is `Pairing.describe`.
func (p Pairing) Describe() string {
	switch p.Kind {
	case Labeled:
		return "Partner—" + p.Label
	case PartnerWith:
		return "Partner with " + p.PartnerName
	case Partner:
		return "Partner"
	case BackgroundChooser:
		return "Choose a Background"
	case DoctorsCompanion:
		return "Doctor's companion"
	}
	return p.Kind
}

var (
	partnerWithRe = regexp.MustCompile(`^Partner with ([^(]+)`)
	labeledRe     = regexp.MustCompile(`^Partner\s*—\s*([^(]+)`)
	plainRe       = regexp.MustCompile(`^Partner(\s*\(|$)`)
)

// Front is the type line's front face: a double-faced card's combined line
// carries both halves around a `//`, and the commander's own types are the
// ones on the side you cast.
func Front(typeLine string) string {
	front, _, _ := strings.Cut(typeLine, " // ")
	return front
}

// IsBackground is `partners.is_background`.
func IsBackground(rec *pool.CardRecord) bool {
	return strings.Contains(Front(rec.TypeLine), BackgroundType)
}

// IsDoctor is `partners.is_doctor`.
func IsDoctor(rec *pool.CardRecord) bool {
	return strings.Contains(Front(rec.TypeLine), DoctorType)
}

// PairingOf is `partners.pairing`: the pairing ability printed on a card, if
// any. Order matters: "Partner with" and "Partner—" both start with
// "Partner", so the specific forms are tested first.
func PairingOf(rec *pool.CardRecord) *Pairing {
	for _, line := range strings.Split(rec.OracleText, "\n") {
		line = strings.TrimSpace(line)
		if m := partnerWithRe.FindStringSubmatch(line); m != nil {
			return &Pairing{Kind: PartnerWith, PartnerName: strings.TrimSpace(m[1])}
		}
		if m := labeledRe.FindStringSubmatch(line); m != nil {
			return &Pairing{Kind: Labeled, Label: strings.TrimSpace(m[1])}
		}
		if plainRe.MatchString(line) {
			return &Pairing{Kind: Partner}
		}
		if strings.HasPrefix(line, "Choose a Background") {
			return &Pairing{Kind: BackgroundChooser}
		}
		if strings.HasPrefix(line, "Doctor's companion") {
			return &Pairing{Kind: DoctorsCompanion}
		}
	}
	return nil
}

// CanBeCommander is `partners.can_be_commander`: legal in the command zone?
// `paired` is what makes a Background legal -- and it is the *only* thing
// pairing changes. A pairing ability never waives the legendary
// requirement: the official ruling on the Battlebond partners is "A
// nonlegendary creature can't be your commander, even if it has a 'partner
// with' ability."
func CanBeCommander(rec *pool.CardRecord, paired bool) bool {
	front := Front(rec.TypeLine)
	if strings.Contains(front, "Legendary") && strings.Contains(front, "Creature") {
		return true
	}
	if strings.Contains(strings.ToLower(rec.OracleText), "can be your commander") {
		return true
	}
	return paired && IsBackground(rec)
}

// NonlegendaryPartner is `partners.nonlegendary_partner`: a `Partner with`
// card that is not legendary, so it can never be a commander.
func NonlegendaryPartner(rec *pool.CardRecord) bool {
	if strings.Contains(Front(rec.TypeLine), "Legendary") {
		return false
	}
	p := PairingOf(rec)
	return p != nil && p.Kind == PartnerWith
}

// match is `partners._match`: is this ordered pair legal? Callers try both.
func match(pa *Pairing, b *pool.CardRecord, pb *Pairing) bool {
	if pa == nil {
		return false
	}
	switch pa.Kind {
	case Partner:
		return pb != nil && pb.Kind == Partner
	case Labeled:
		return pb != nil && pb.Kind == Labeled && strings.EqualFold(pb.Label, pa.Label)
	case PartnerWith:
		return strings.EqualFold(pa.PartnerName, b.Name)
	case BackgroundChooser:
		return IsBackground(b)
	case DoctorsCompanion:
		return IsDoctor(b)
	}
	return false
}

// CheckPair is `partners.check_pair`: "" if the two cards may be commanders
// together, else why not -- precisely, because "invalid pair" is useless at
// the table.
func CheckPair(a, b *pool.CardRecord) string {
	pa, pb := PairingOf(a), PairingOf(b)
	if match(pa, b, pb) || match(pb, a, pa) {
		return ""
	}
	if pa == nil && pb == nil {
		return "neither card has a pairing ability, so they cannot be two commanders together"
	}
	if pa != nil && pb != nil && pa.Kind == Labeled && pb.Kind == Labeled {
		return fmt.Sprintf("%s has Partner—%s but %s has Partner—%s; both must have the same one",
			a.Name, pa.Label, b.Name, pb.Label)
	}
	for _, side := range []struct {
		one   *pool.CardRecord
		p     *Pairing
		other *pool.CardRecord
	}{{a, pa, b}, {b, pb, a}} {
		if side.p == nil {
			continue
		}
		switch side.p.Kind {
		case PartnerWith:
			return fmt.Sprintf("%s has Partner with %s, so it can only pair with that card, not %s",
				side.one.Name, side.p.PartnerName, side.other.Name)
		case BackgroundChooser:
			return fmt.Sprintf("%s chooses a Background, but %s is not a Background",
				side.one.Name, side.other.Name)
		case DoctorsCompanion:
			return fmt.Sprintf("%s is a Doctor's companion, but %s is not a Doctor",
				side.one.Name, side.other.Name)
		}
	}
	lone := a
	if pa == nil {
		lone = b
	}
	return fmt.Sprintf("only %s has a pairing ability; both commanders need one that matches", lone.Name)
}
