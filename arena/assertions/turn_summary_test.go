package assertions

import (
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

func TestTurnSummaryFromMeta(t *testing.T) {
	tests := []struct {
		name string
		meta map[string]interface{}
		want TurnSummary
		ok   bool
	}{
		{name: "nil meta", meta: nil, ok: false},
		{name: "no assertions key", meta: map[string]interface{}{"x": 1}, ok: false},
		{name: "assertions not a map", meta: map[string]interface{}{"assertions": "nope"}, ok: false},
		{
			name: "text path: results, passed, total, failed",
			meta: map[string]interface{}{"assertions": map[string]interface{}{
				"results": []interface{}{
					map[string]interface{}{"passed": true},
					map[string]interface{}{"passed": false},
				},
				"passed": false, "total": 2, "failed": 1,
			}},
			want: TurnSummary{Total: 2, Failed: 1, Passed: false},
			ok:   true,
		},
		{
			name: "duplex path: typed results slice and all_passed",
			meta: map[string]interface{}{"assertions": map[string]interface{}{
				"results": []map[string]interface{}{
					{"passed": true}, {"passed": false}, {"passed": true},
				},
				"all_passed": false, "total": 3, "failed": 1,
			}},
			want: TurnSummary{Total: 3, Failed: 1, Passed: false},
			ok:   true,
		},
		{
			name: "duplex path all passed",
			meta: map[string]interface{}{"assertions": map[string]interface{}{
				"results":    []map[string]interface{}{{"passed": true}},
				"all_passed": true,
			}},
			want: TurnSummary{Total: 1, Failed: 0, Passed: true},
			ok:   true,
		},
		{
			name: "json round trip: float total and failed, no results",
			meta: map[string]interface{}{"assertions": map[string]interface{}{
				"total": float64(4), "failed": float64(2),
			}},
			want: TurnSummary{Total: 4, Failed: 2, Passed: false},
			ok:   true,
		},
		{
			name: "results without failed key counts failing entries",
			meta: map[string]interface{}{"assertions": map[string]interface{}{
				"results": []interface{}{
					map[string]interface{}{"passed": false},
					"not a map",
					map[string]interface{}{"passed": true},
				},
			}},
			want: TurnSummary{Total: 3, Failed: 1, Passed: false},
			ok:   true,
		},
		{
			name: "passed false with no failure count still counts one failure",
			meta: map[string]interface{}{"assertions": map[string]interface{}{
				"passed": false,
			}},
			want: TurnSummary{Total: 0, Failed: 1, Passed: false},
			ok:   true,
		},
		{
			name: "legacy format: map of per-assertion maps",
			meta: map[string]interface{}{"assertions": map[string]interface{}{
				"content_includes": map[string]interface{}{"passed": true},
				"tools_called":     map[string]interface{}{"passed": false},
				"note":             "ignored",
			}},
			want: TurnSummary{Total: 2, Failed: 1, Passed: false},
			ok:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := TurnSummaryFromMeta(tt.meta)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCountTurnAssertions(t *testing.T) {
	msgs := []types.Message{
		{Role: "user"},
		{Role: "assistant", Meta: map[string]interface{}{"assertions": map[string]interface{}{
			"results": []interface{}{map[string]interface{}{"passed": true}, map[string]interface{}{"passed": true}},
			"passed":  true, "total": 2, "failed": 0,
		}}},
		{Role: "assistant", Meta: map[string]interface{}{"assertions": map[string]interface{}{
			"results": []interface{}{map[string]interface{}{"passed": false}},
			"passed":  false, "total": 1, "failed": 1,
		}}},
	}
	total, failed := CountTurnAssertions(msgs)
	if total != 3 || failed != 1 {
		t.Fatalf("got total=%d failed=%d, want 3 and 1", total, failed)
	}
}
