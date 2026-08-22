// Package mana is `mtglab/mana.py`, the parser half: a Scryfall cost string
// read into generic mana, coloured pips (each the SET of colours that can
// pay it), Phyrexian symbols (kept apart: two life always pays them, so they
// constrain no mana base, yet they count toward mana value and identity) and
// whether the cost holds an X. The castability solver -- Kuhn's matching over
// pips and units -- arrives with Tier 1 in Phase 5 and is held to the 13,944
// case oracle then; the parser arrives now because `analyze.py`'s curve and
// pip counts read it.
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

// Cost is `mana.ManaCost`.
type Cost struct {
	Generic   int
	Pips      [][]string // one sorted colour set per coloured requirement
	HasX      bool
	Phyrexian [][]string
}

// ManaValue is `ManaCost.mana_value`: generic plus one per pip plus one per
// Phyrexian symbol. X counts as 0.
func (c Cost) ManaValue() int { return c.Generic + len(c.Pips) + len(c.Phyrexian) }

// ColorsOf is `ManaCost.colors`: every colour in any symbol, sorted.
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

// Parse is `mana.parse_mana_cost`: generic, coloured, {C}, hybrid {G/W},
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
