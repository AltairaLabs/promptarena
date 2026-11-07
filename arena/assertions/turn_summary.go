package assertions

import "github.com/AltairaLabs/PromptKit/runtime/v2/types"

// metaKeyAssertions is the message Meta key under which the turn-level
// assertion stage (text and duplex paths) records its outcome.
const metaKeyAssertions = "assertions"

// TurnSummary is the turn-level assertion outcome recorded on one message.
type TurnSummary struct {
	Total  int
	Failed int
	Passed bool
}

// TurnSummaryFromMeta reads the turn-level assertion outcome from a message's
// Meta["assertions"]. It is the one reader of that map, so the run summary,
// the pass/fail predicate and every report agree on what a turn recorded.
//
// It understands the text-path shape ({results, passed, total, failed}), the
// duplex shape ({results, all_passed, total, failed}, with a typed results
// slice), the JSON round-trip of either (float64 numbers), and the legacy
// map-of-assertions shape. ok is false when the message carries no assertions.
func TurnSummaryFromMeta(meta map[string]interface{}) (summary TurnSummary, ok bool) {
	m, ok := meta[metaKeyAssertions].(map[string]interface{})
	if !ok {
		return TurnSummary{}, false
	}

	entries, hasEntries := resultEntries(m["results"])
	if !hasEntries {
		if _, hasTotal := intValue(m["total"]); !hasTotal {
			if _, hasFailed := intValue(m["failed"]); !hasFailed {
				entries, hasEntries = legacyEntries(m)
			}
		}
	}

	if hasEntries {
		summary.Total = len(entries)
	} else {
		summary.Total, _ = intValue(m["total"])
	}

	if failed, hasFailed := intValue(m["failed"]); hasFailed {
		summary.Failed = failed
	} else {
		summary.Failed = countFailedEntries(entries)
	}

	switch {
	case isBool(m["passed"]):
		summary.Passed = m["passed"].(bool)
	case isBool(m["all_passed"]):
		summary.Passed = m["all_passed"].(bool)
	default:
		summary.Passed = summary.Failed == 0
	}
	if !summary.Passed && summary.Failed == 0 {
		summary.Failed = 1
	}
	return summary, true
}

// CountTurnAssertions sums the turn-level assertions recorded across messages.
func CountTurnAssertions(msgs []types.Message) (total, failed int) {
	for i := range msgs {
		if s, ok := TurnSummaryFromMeta(msgs[i].Meta); ok {
			total += s.Total
			failed += s.Failed
		}
	}
	return total, failed
}

// resultEntries normalises the "results" value, which is []interface{} on the
// text path and after a JSON round trip, and []map[string]interface{} on the
// duplex path.
func resultEntries(v interface{}) ([]interface{}, bool) {
	switch r := v.(type) {
	case []interface{}:
		return r, true
	case []map[string]interface{}:
		out := make([]interface{}, len(r))
		for i := range r {
			out[i] = r[i]
		}
		return out, true
	default:
		return nil, false
	}
}

// legacyEntries collects the per-assertion maps of the legacy shape, where the
// assertions map is keyed by assertion name and each value carries "passed".
func legacyEntries(m map[string]interface{}) ([]interface{}, bool) {
	var out []interface{}
	for _, v := range m {
		if entry, ok := v.(map[string]interface{}); ok {
			if _, has := entry["passed"]; has {
				out = append(out, entry)
			}
		}
	}
	return out, len(out) > 0
}

func countFailedEntries(entries []interface{}) int {
	failed := 0
	for _, e := range entries {
		entry, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		if passed, ok := entry["passed"].(bool); ok && !passed {
			failed++
		}
	}
	return failed
}

func intValue(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

func isBool(v interface{}) bool {
	_, ok := v.(bool)
	return ok
}
