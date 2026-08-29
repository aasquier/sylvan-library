package claude

// The deck description as a surface of its own, rather than a step of an
// import.
//
// `DescribeDeck` was built for the intake sheet (ADR 41), which runs it
// unattended: nobody reads the paragraph before it reaches the deck file,
// because the deck it describes is three seconds old and has nothing to
// overwrite. `internal/api`'s intake even checks that -- a deck that already
// says what it is doing keeps what it says, and the whole step is skipped.
//
// The deck page's description editor is the same mode asked by somebody who is
// looking at the box, and that difference is the whole of this file. There the
// answer is *written*; here it is *offered*, into an editor whose save button
// belongs to the person reading it. Nothing in this package could write it
// anyway -- `may_write` is empty on every mode and `boundary_test.go` bans this
// tree from the write engine transitively -- and on the new surface nothing
// outside the package writes it either: the paragraph reaches the deck file
// through `PATCH .../decks/{owner}/{slug}`, the same door the person's own
// typing goes through, because by then it *is* their typing.
//
// # Nothing here is marked as Claude's, and that is a decision
//
// `why_by: claude` (ADR 41) exists because a rationale is a claim about
// somebody's thinking, and a drafted one is a claim nobody made yet; the mark
// is dropped the instant a person edits the sentence. Two things say this
// field is not that object:
//
//   - **It is the wrong kind of thing.** `strategy` is the label on the box --
//     what the deck is trying to do -- not an account of why a particular card
//     holds a particular slot. `intake.go` already makes that argument where
//     `Description` is declared, and this surface does not change it.
//   - **It has already passed the moment the mark exists for.** ADR 41 drops
//     `why_by` the first time a person edits the sentence. Here the sentence
//     lands in the owner's own textarea, where they read it, may rewrite half
//     of it, and then press save. Writing "Claude wrote this" on the far side
//     of that would be recording something that is not true.
//
// So the honesty this surface owes is paid where it is actually owed: in the
// panel, while the draft is still a draft, labelled as Claude's and not the
// gate's (ADR 14 boundary 3) -- and `Never` below says so in the payload, so a
// second client cannot render it as anything else.

// DescriptionNever is the promise this payload carries about itself, the way
// `NeverSentence` does for the interview.
//
// Said in the payload rather than only in the component for the same reason as
// the interview's: a client that renders this without the sentence is
// rendering a paragraph with no owner, and the owner is the point.
const DescriptionNever = "This is a draft in your own box. Nothing is saved " +
	"until you save it, and every word of it is yours to change."

// DescriptionReport is one response shape for every outcome, including not
// asking at all.
//
// `answered_by` is ADR 14's third boundary made a field -- the gate's output
// and Claude's never share a surface without a label -- and it names the
// system, never the checkpoint that answered (commandment 10).
//
// `asked: false` with a `reason` is a real answer and not a failure: a stance
// of `off` makes no call, costs nothing, and says so. A client that renders
// that as an error is telling somebody their instance is broken when their
// dial is merely down.
type DescriptionReport struct {
	AnsweredBy string        `json:"answered_by"`
	Mode       string        `json:"mode"`
	Slug       string        `json:"slug"`
	Asked      bool          `json:"asked"`
	Reason     string        `json:"reason"`
	Stance     StanceReadout `json:"stance"`
	Strategy   string        `json:"strategy"`
	Themes     []string      `json:"themes"`
	Fact       string        `json:"fact"`
	Never      string        `json:"never"`
}

// DescriptionFor renders what DescribeDeck answered as the shape a client
// reads.
//
// `Themes` goes out as `[]` and never `null`: the client indexes it, and an
// absent list and an empty one are the same fact here -- the deck got no index
// terms -- so they are one shape on the wire rather than two.
func DescriptionFor(slug string, got Description, outcome IntakeOutcome) DescriptionReport {
	return DescriptionReport{
		AnsweredBy: "claude",
		Mode:       ModeDeckDescription,
		Slug:       slug,
		Asked:      outcome.Asked,
		Reason:     outcome.Reason,
		Stance:     Describe(outcome.Stance),
		Strategy:   got.Strategy,
		Themes:     nonNil(got.Themes),
		Fact:       got.Fact,
		Never:      DescriptionNever,
	}
}
