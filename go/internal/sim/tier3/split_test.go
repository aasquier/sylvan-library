package tier3_test

import "github.com/aasquier/sylvan-library/go/internal/textutil"

// splitLikeTheCorpus is the recorded line splitting -- the boundary set the
// corpus's `is_game_result` list was built over. Named rather than inlined
// so the
// predicate test is asking about the predicate rather than about the split.
func splitLikeTheCorpus(s string) []string { return textutil.SplitLines(s) }
