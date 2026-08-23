package tier3_test

import "github.com/aasquier/sylvan-library/go/internal/pytext"

// splitLikePython is `str.splitlines()`, which is what the corpus's
// `is_game_result` list was built over. Named rather than inlined so the
// predicate test is asking about the predicate rather than about the split.
func splitLikePython(text string) []string { return pytext.SplitLines(text) }
