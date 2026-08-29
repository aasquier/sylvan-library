package claude

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/anthropics/anthropic-sdk-go"
)

// The mode definitions, embedded.
//
// A mode is a system prompt, a tool set and a response schema, and all three
// live as **recorded data** rather than as string literals -- the same
// split `tools` makes, for the same reason at greater scale. A prompt is prose
// whose bytes reach a model; `theme-proposal`'s alone runs to thousands of
// words. Hand-copying seven of them into source code is the drift the
// embed exists to prevent, and unlike a persona's voice a mistyped
// instruction here changes what the model is *allowed to do* rather than how
// it sounds.
//
// **The response schemas are the sharpest case for keeping them data.** ADR 25's
// slot argument has no `defence`, `verdict` or `summary` property and forbids
// extra ones, so a balanced answer -- the attractive one, and the one that is
// a rationale generator wearing a hat -- has nowhere to go. ADR 34's scan has
// no field for a card name. Those **absences are the features**, and an
// absence is precisely what a hand-copy drops with nothing looking wrong.
//
// All seven definitions load. The definition is data; the code that
// assembles a brief and reads an answer back is the mode's own.
//
// **Seven, and this comment said six until 2026-08-23.** The file's first
// builder worked from a hand list and silently lost the scan mode, whose
// definition is spelled differently from its siblings; #257 rebuilt it to
// discover modes by type, and the prose describing it kept the old number. A
// count in a comment is a claim
// to re-check against the data, which is why `ModeNames` is derived and no
// number is written down anywhere a test cannot read it.

//go:embed data/modes.json
var modesJSON []byte

type modeFile struct {
	Modes []struct {
		Name           string            `json:"name"`
		Purpose        string            `json:"purpose"`
		Instructions   string            `json:"instructions"`
		ToolNames      []string          `json:"tool_names"`
		ServerTools    []serverToolSpec  `json:"server_tools"`
		MayWrite       []string          `json:"may_write"`
		MaxTokens      int64             `json:"max_tokens"`
		Effort         string            `json:"effort"`
		ResponseSchema map[string]any    `json:"response_schema"`
		ScopeNotes     map[string]string `json:"scope_notes"`
	} `json:"modes"`
}

// serverToolSpec is an Anthropic-hosted tool as the API spells it. It crosses
// as the wire's own object rather than as a Go type, and is mapped onto the
// SDK's typed union here -- so a type nobody taught this function about is a
// loud failure at startup rather than a mode that quietly lost its search.
type serverToolSpec struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	MaxUses int64  `json:"max_uses"`
}

func (s serverToolSpec) param() (anthropic.ToolUnionParam, error) {
	switch s.Type {
	case "web_search_20260209":
		search := &anthropic.WebSearchTool20260209Param{}
		if s.MaxUses > 0 {
			search.MaxUses = anthropic.Int(s.MaxUses)
		}
		return anthropic.ToolUnionParam{OfWebSearchTool20260209: search}, nil
	default:
		return anthropic.ToolUnionParam{}, fmt.Errorf(
			"no way to build the hosted tool %q -- it is declared in "+
				"data/modes.json and this switch has never heard of it, so the "+
				"mode would go out without it", s.Type)
	}
}

// modes is every defined mode, by name.
//
// A package-level var rather than an `init()`, deliberately: Go runs a
// package's `init` functions in **file-name order**, which has emptied a
// registry in this tree before by firing a handler file ahead of the file that
// filled it. Var initialisation is ordered by dependency instead, so this is
// built before anything that reads it regardless of what the files are called.
// Reaching `tools` from here is safe for a different reason -- it is another
// package, and Go finishes a dependency's initialisation before its importer's.
var modes = loadModes()

func loadModes() map[string]Mode {
	var file modeFile
	if err := json.Unmarshal(modesJSON, &file); err != nil {
		panic(fmt.Sprintf("claude: data/modes.json will not parse: %v", err))
	}
	out := make(map[string]Mode, len(file.Modes))
	for _, row := range file.Modes {
		servers := make([]anthropic.ToolUnionParam, 0, len(row.ServerTools))
		names := make([]string, 0, len(row.ServerTools))
		for _, spec := range row.ServerTools {
			built, err := spec.param()
			if err != nil {
				panic(fmt.Sprintf("claude: mode %q: %v", row.Name, err))
			}
			servers = append(servers, built)
			names = append(names, spec.Name)
		}
		out[row.Name] = MustMode(Mode{
			Name:            row.Name,
			Purpose:         row.Purpose,
			Instructions:    row.Instructions,
			ToolNames:       row.ToolNames,
			ServerTools:     servers,
			ServerToolNames: names,
			MayWrite:        row.MayWrite,
			MaxTokens:       row.MaxTokens,
			Effort:          anthropic.OutputConfigEffort(row.Effort),
			ResponseSchema:  row.ResponseSchema,
			ScopeNotes:      row.ScopeNotes,
		})
	}
	return out
}

// ModeNames is every defined mode, sorted.
func ModeNames() []string {
	names := make([]string, 0, len(modes))
	for name := range modes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetMode is one mode by name, or an error naming what there is.
//
// An unknown name is a programming error rather than a user's, so this is
// mostly read through the typed constants below -- but it exists because the
// registry is data, and data can be asked about.
func GetMode(name string) (Mode, error) {
	m, ok := modes[name]
	if !ok {
		return Mode{}, fmt.Errorf("no such mode %q -- there are %v", name, ModeNames())
	}
	return m, nil
}

// The modes by name. Constants rather than string literals at the call sites,
// so a renamed mode is a compile error in one place instead of a nil lookup in
// six.
const (
	ModeRationaleInterview = "rationale-interview"
	ModeSlotArgument       = "slot-argument"
	ModeCommanderDossier   = "commander-dossier"
	ModeResearch           = "research"
	ModeThemeConversation  = "theme-conversation"
	ModeThemeProposal      = "theme-proposal"
	ModeScan               = "scan"
	// The two the intake added (ADR 41). Both are read-only like every other
	// mode -- `may_write` stays empty and `NewMode` still refuses anything
	// else. What ADR 41 changed is that a caller OUTSIDE this tree may write
	// what `rationale-draft` answered, which is why the write door it uses is
	// on `boundary_test.go`'s banned surface rather than in a mode.
	ModeRationaleDraft = "rationale-draft"
	ModeIntakeFiling   = "intake-filing"
)
