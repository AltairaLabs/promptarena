package generate

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
)

const fingerprintLength = 16

// Fingerprint produces a stable hash for a session based on how it failed and
// what the user said. Sessions with the same fingerprint are duplicates for
// dedup purposes.
//
// Only evals with a failed verdict contribute. A passed verdict says nothing
// about a failure, and a bare measurement's "badness" is the user's bound,
// not a property of the session.
func Fingerprint(session *SessionDetail) string {
	h := sha256.New()

	var keys []string
	for i := range session.Evals {
		if session.Evals[i].Failed() {
			keys = append(keys, failureKey(&session.Evals[i]))
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		_, _ = fmt.Fprintln(h, k)
	}

	for i := range session.Messages {
		if session.Messages[i].Role == roleUser {
			_, _ = fmt.Fprintln(h, session.Messages[i].GetContent())
		}
	}

	return fmt.Sprintf("%x", h.Sum(nil))[:fingerprintLength]
}

// DeduplicateSessions keeps the first session per unique fingerprint.
func DeduplicateSessions(sessions []*SessionDetail) []*SessionDetail {
	seen := make(map[string]bool)
	var result []*SessionDetail
	for _, s := range sessions {
		fp := Fingerprint(s)
		if seen[fp] {
			continue
		}
		seen[fp] = true
		result = append(result, s)
	}
	return result
}

// failureKey is a stable representation of one failed eval: type, turn, and
// the sorted keys of its details (keys, not values, so a message that differs
// only in the offending text still counts as the same failure).
func failureKey(r *EvalResult) string {
	parts := []string{r.Type}
	if r.Turn != nil {
		parts = append(parts, fmt.Sprintf("turn:%d", *r.Turn))
	}
	detailKeys := make([]string, 0, len(r.Details))
	for k := range r.Details {
		detailKeys = append(detailKeys, k)
	}
	sort.Strings(detailKeys)
	parts = append(parts, detailKeys...)
	return strings.Join(parts, ":")
}
