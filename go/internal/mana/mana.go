// Package mana reads a Scryfall cost string into generic
// mana, coloured pips (each the SET of colours that can pay it), Phyrexian
// symbols (kept apart: two life always pays them, so they constrain no mana
// base, yet they count toward mana value and identity) and whether the cost
// holds an X -- and then runs the exact castability solver over that reading.
//
// The parser came first, because the analysis surfaces' curve and pip counts
// read it. The solver followed (`solver.go`), held to a
// recorded 13,944-case enumeration and to that case set's
// pinned answer digest: ENGINEERING section 1 built that oracle to be usable
// "in any language,
// forever", and this is the language it holds today.
//
// The package takes plain records and keeps no database, no network and no
// dependency past the standard library -- the same boundary CLAUDE.md draws
// around the deterministic core, and for the same reason. It is what lets the
// most correctness-critical function in the project be tested ten thousand
// times in a few milliseconds.
package mana

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Colors is WUBRG, in that order.
var Colors = []string{"W", "U", "B", "R", "G"}

// Colorless is {C}: genuinely colourless mana, a pip only a colourless
// source can pay.
const Colorless = "C"

var symbolRe = regexp.MustCompile(`\{([^}]+)\}`)

// Cost is a parsed mana cost.
type Cost struct {
	Generic   int
	Pips      [][]string // one sorted colour set per coloured requirement
	HasX      bool
	Phyrexian [][]string
}

// ManaValue is generic plus one per pip plus one per
// Phyrexian symbol. X counts as 0.
func (c Cost) ManaValue() int { return c.Generic + len(c.Pips) + len(c.Phyrexian) }

// ColorsOf is every colour in any symbol, sorted.
func (c Cost) ColorsOf() []string {
	seen := map[string]bool{}
	for _, pip := range append(append([][]string{}, c.Pips...), c.Phyrexian...) {
		for _, col := range pip {
			if isColor(col) {
				seen[col] = true
			}
		}
	}
	out := []string{}
	for col := range seen {
		out = append(out, col)
	}
	sort.Strings(out)
	return out
}

// String renders the cost, and it is load-bearing rather than cosmetic:
// the recorded case set names a case by rendering its cost, so these bytes
// are compared against the corpus for all 168 costs in the enumeration.
//
// Two details are the recorded rendering's and are reproduced rather than
// tidied. A generic of
// zero is omitted entirely, and a cost that renders to
// nothing at all is "{0}" -- so a bare X is "{X}" and an empty cost is "{0}".
func (c Cost) String() string {
	var b strings.Builder
	if c.HasX {
		b.WriteString("{X}")
	}
	if c.Generic != 0 {
		b.WriteString("{" + strconv.Itoa(c.Generic) + "}")
	}
	for _, pip := range c.Pips {
		b.WriteString("{" + strings.Join(sortedCopy(pip), "/") + "}")
	}
	for _, pip := range c.Phyrexian {
		b.WriteString("{" + strings.Join(sortedCopy(pip), "/") + "/P}")
	}
	if b.Len() == 0 {
		return "{0}"
	}
	return b.String()
}

// sortedCopy sorts without disturbing the caller's slice. The rendering
// sorts each
// pip at render time and a hand-built Cost need not arrive sorted, so the sort
// has to happen here -- but a String() that reorders its receiver's fields
// would be a trap laid for whoever calls it inside a failure message.
func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func isColor(s string) bool {
	for _, c := range Colors {
		if c == s {
			return true
		}
	}
	return false
}

func producible(s string) bool { return isColor(s) || s == Colorless }

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Parse reads a cost string: generic, coloured, {C}, hybrid {G/W},
// monocolour hybrid {2/G} (a pip any source can satisfy plus the generic --
// an approximation that never claims a hand castable when it is not),
// Phyrexian {G/P}, and an unknown symbol as one generic, so castability is
// never overstated.
func Parse(cost string) Cost {
	var out Cost
	if cost == "" {
		return out
	}
	for _, m := range symbolRe.FindAllStringSubmatch(cost, -1) {
		sym := strings.TrimSpace(strings.ToUpper(m[1]))
		switch {
		case sym == "X":
			out.HasX = true
		case isDigits(sym):
			n, _ := strconv.Atoi(sym)
			out.Generic += n
		case sym == Colorless:
			out.Pips = append(out.Pips, []string{Colorless})
		case isColor(sym):
			out.Pips = append(out.Pips, []string{sym})
		case strings.Contains(sym, "/"):
			halves := strings.Split(sym, "/")
			if contains(halves, "P") {
				set := []string{}
				for _, h := range halves {
					if producible(h) {
						set = append(set, h)
					}
				}
				sort.Strings(set)
				out.Phyrexian = append(out.Phyrexian, set)
				continue
			}
			numeric := ""
			for _, h := range halves {
				if isDigits(h) {
					numeric = h
					break
				}
			}
			if numeric != "" {
				n, _ := strconv.Atoi(numeric)
				out.Generic += n
				continue
			}
			set := []string{}
			seen := map[string]bool{}
			for _, h := range halves {
				if producible(h) && !seen[h] {
					seen[h] = true
					set = append(set, h)
				}
			}
			if len(set) > 0 {
				sort.Strings(set)
				out.Pips = append(out.Pips, set)
			}
		default:
			out.Generic++
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
