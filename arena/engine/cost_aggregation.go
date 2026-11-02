package engine

import "github.com/AltairaLabs/PromptKit/runtime/v2/types"

// selfPlayCostMetaKey is the Meta key under which duplex's
// streamSelfPlayUserAudio records the persona LLM call's CostInfo.
// Stored as map[string]any rather than CostInfo because the message
// passes through JSON persistence between writer and reader.
const selfPlayCostMetaKey = "self_play_cost"

// ttsCostMetaKey is the Meta key under which TTS synthesis cost is recorded.
// Set by stampTTSCostInMeta (duplex persona path) and by TTSStage.processElement
// (pipeline path) after a successful Synthesize call.
const ttsCostMetaKey = "tts_cost"

// Keys of a cost map stored in Meta.
const (
	metaInputTokens   = "input_tokens"
	metaOutputTokens  = "output_tokens"
	metaCachedTokens  = "cached_tokens" //nolint:gosec // a metadata key, not a credential
	metaInputCostUSD  = "input_cost_usd"
	metaOutputCostUSD = "output_cost_usd"
	metaCachedCostUSD = "cached_cost_usd"
	metaTotalCostUSD  = "total_cost_usd"
)

// ancillaryCostMetaKeys lists every Message.Meta key that holds an
// ancillary cost contribution to fold into the total. Ordered
// alphabetically.
var ancillaryCostMetaKeys = []string{
	selfPlayCostMetaKey,
	ttsCostMetaKey,
}

// addAncillaryCostFromMeta folds every recognized ancillary cost stored
// in a message's Meta into the running total. The persona-side LLM call
// cost (self_play_cost) is the original case — see #1105. Future
// migrations (TTS, STT, embedding) extend the same pattern by adding to
// ancillaryCostMetaKeys.
//
// Numeric values may arrive as float64 (post-JSON round-trip) or as
// int / int64 (in-memory). Both forms are accepted; unknown types
// contribute zero.
func addAncillaryCostFromMeta(total *types.CostInfo, meta map[string]interface{}) {
	if total == nil || meta == nil {
		return
	}
	for _, key := range ancillaryCostMetaKeys {
		addCostFromMetaKey(total, meta, key)
	}
}

func addCostFromMetaKey(total *types.CostInfo, meta map[string]interface{}, key string) {
	sc, ok := meta[key].(map[string]interface{})
	if !ok {
		return
	}
	c := costFromMeta(sc)
	addCost(total, &c)
}

// costFromMeta reads a cost map stored in Meta.
func costFromMeta(m map[string]interface{}) types.CostInfo {
	return types.CostInfo{
		InputTokens:   metaInt(m[metaInputTokens]),
		OutputTokens:  metaInt(m[metaOutputTokens]),
		CachedTokens:  metaInt(m[metaCachedTokens]),
		InputCostUSD:  metaFloat(m[metaInputCostUSD]),
		OutputCostUSD: metaFloat(m[metaOutputCostUSD]),
		CachedCostUSD: metaFloat(m[metaCachedCostUSD]),
		TotalCost:     metaFloat(m[metaTotalCostUSD]),
	}
}

// addSelfPlayCostFromMeta is a back-compat alias for addAncillaryCostFromMeta.
//
// Deprecated: use addAncillaryCostFromMeta directly; this wrapper exists so
// existing call sites continue to compile.
func addSelfPlayCostFromMeta(total *types.CostInfo, meta map[string]interface{}) {
	addAncillaryCostFromMeta(total, meta)
}

func metaInt(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func metaFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

// rewoundCostMetaKey is the conversation metadata key holding what the
// exchanges a self-play rewind removed cost, so the run's total still counts
// them. Stored in the same map shape as self_play_cost.
const rewoundCostMetaKey = "rewound_cost"

// addCost adds c to total.
func addCost(total, c *types.CostInfo) {
	total.InputTokens += c.InputTokens
	total.OutputTokens += c.OutputTokens
	total.CachedTokens += c.CachedTokens
	total.InputCostUSD += c.InputCostUSD
	total.OutputCostUSD += c.OutputCostUSD
	total.CachedCostUSD += c.CachedCostUSD
	total.TotalCost += c.TotalCost
}

// msgCost adds what msg cost, including ancillary costs in its Meta, to total.
func msgCost(total *types.CostInfo, msg *types.Message) {
	if msg.CostInfo != nil {
		addCost(total, msg.CostInfo)
	}
	addAncillaryCostFromMeta(total, msg.Meta)
}

// rewoundCapability labels rewound cost in the report's cost breakdown.
const rewoundCapability = "rewound"

// addCostToMeta returns existing (a cost map, or nil) with c added to it.
func addCostToMeta(existing interface{}, c *types.CostInfo) map[string]interface{} {
	var sum types.CostInfo
	if m, ok := existing.(map[string]interface{}); ok {
		sum = costFromMeta(m)
	}
	addCost(&sum, c)
	return map[string]interface{}{
		metaInputTokens:   sum.InputTokens,
		metaOutputTokens:  sum.OutputTokens,
		metaCachedTokens:  sum.CachedTokens,
		metaInputCostUSD:  sum.InputCostUSD,
		metaOutputCostUSD: sum.OutputCostUSD,
		metaCachedCostUSD: sum.CachedCostUSD,
		metaTotalCostUSD:  sum.TotalCost,
		"capability":      rewoundCapability,
	}
}
