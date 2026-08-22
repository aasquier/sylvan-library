package auth

import (
	"fmt"
)

// failf is `fmt.Errorf` for a refusal whose sentence a route answers
// **verbatim**, and it exists because those two things are in tension.
//
// Python raises `LastAdmin("refusing to revoke admin: ...")` and the handler
// answers `detail=str(exc)` -- so the sentence the caller reads is the
// exception's whole text, with no class name in front of it. Go's idiom for
// "which kind of failure is this" is `fmt.Errorf("%w: ...", ErrLastAdmin)`,
// and that puts the sentinel's own words at the front of `Error()`: the
// browser would render `last admin: refusing to revoke admin: ...`. Every
// sentence in this package is on that path, so the tension is not an edge
// case.
//
// The call keeps the `%w:` spelling deliberately -- it reads like
// `fmt.Errorf`, it greps like `fmt.Errorf`, and the sentinel stays in the
// argument position a reader expects. What differs is that the sentinel's text
// is dropped from the message while `errors.Is` still finds it, through
// `Unwrap` below.
//
// It panics on a call shape it cannot honour, which is a programming error
// caught the first time the line runs rather than a wrong sentence shipped to
// a user.
func failf(format string, args ...any) error {
	const marker = "%w: "
	if len(format) < len(marker) || format[:len(marker)] != marker {
		panic("auth: failf wants a format beginning `%w: `, got " + format)
	}
	if len(args) == 0 {
		panic("auth: failf wants a sentinel error as its first argument")
	}
	kind, ok := args[0].(error)
	if !ok {
		panic(fmt.Sprintf("auth: failf's first argument is %T, not an error", args[0]))
	}
	return &refusal{kind: kind, message: fmt.Sprintf(format[len(marker):], args[1:]...)}
}

// refusal is a sentence plus the sentinel it is an instance of. `Error` is the
// sentence alone, which is what a route puts on the wire; `Unwrap` is what
// makes `errors.Is(err, ErrLastAdmin)` true.
type refusal struct {
	kind    error
	message string
}

func (r *refusal) Error() string { return r.message }
func (r *refusal) Unwrap() error { return r.kind }
