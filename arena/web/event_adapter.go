// Package web provides the HTTP server and SSE event streaming for the Arena web UI.
package web

import (
	"encoding/base64"
	"encoding/json"
	"hash/fnv"
	"sync"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/events"
	"github.com/AltairaLabs/PromptKit/runtime/types"
	arenaaudio "github.com/AltairaLabs/promptarena/arena/audio"
)

// SSEEvent is the JSON structure sent to SSE clients.
type SSEEvent struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	// Sequence is the bus's monotonic publish counter. It matters because the
	// bus dispatches through a fixed-size worker pool, so listeners do NOT see
	// events in publish order — verified live: concatenating reasoning
	// fragments in arrival order produced "C = = 3" where the model wrote
	// "C = 3", one fragment having overtaken another. Anything a consumer
	// reassembles from multiple frames must sort on this first.
	Sequence       int64       `json:"sequence,omitempty"`
	ExecutionID    string      `json:"executionId,omitempty"`
	ConversationID string      `json:"conversationId,omitempty"`
	Data           interface{} `json:"data,omitempty"`
}

// clientBufferSize is the channel buffer for each SSE client.
const clientBufferSize = 256

// clientState tracks what a single SSE client has successfully been sent, so
// BroadcastFullMessages can skip messages that client already holds unchanged.
//
// sentFull maps conversation ID -> per-index hash of the last message.full
// payload delivered to this client. An entry is recorded only when the
// non-blocking send succeeds, so a frame dropped for a full buffer stays
// "unsent" and is retried on the next save — this is what makes dropped
// messages self-heal.
type clientState struct {
	sentFull map[string][]uint64
}

// unsentHash is the zero entry for an index this client has never received.
// Real payload hashes are offset away from it (see hashMessage), so a genuine
// hash can never collide with "never sent".
const unsentHash uint64 = 0

// EventAdapter subscribes to an events.Bus and fans out
// JSON-serialized events to registered SSE client channels.
type EventAdapter struct {
	mu      sync.RWMutex
	clients map[chan []byte]*clientState

	audioMu      sync.RWMutex
	audioClients map[chan []byte]struct{}
}

// NewEventAdapter creates a new EventAdapter.
func NewEventAdapter() *EventAdapter {
	return &EventAdapter{
		clients:      make(map[chan []byte]*clientState),
		audioClients: make(map[chan []byte]struct{}),
	}
}

// Subscribe subscribes the adapter to an event bus.
func (a *EventAdapter) Subscribe(bus events.Bus) {
	if bus == nil {
		return
	}
	bus.SubscribeAll(a.HandleEvent)
}

// Register adds a new SSE client and returns its event channel.
func (a *EventAdapter) Register() chan []byte {
	ch := make(chan []byte, clientBufferSize)
	a.mu.Lock()
	// Fresh client: empty sent-set, so the next BroadcastFullMessages sends it
	// the complete history while established clients get only what changed.
	a.clients[ch] = &clientState{sentFull: make(map[string][]uint64)}
	a.mu.Unlock()
	return ch
}

// Unregister removes an SSE client. The caller is responsible for draining
// or discarding the channel after unregistering.
func (a *EventAdapter) Unregister(ch chan []byte) {
	a.mu.Lock()
	delete(a.clients, ch)
	a.mu.Unlock()
}

// RegisterAudio adds a client channel that receives audio SSE messages.
// Audio clients are distinct from regular event clients — only clients that
// explicitly opted in (via ?audio=1) receive audio frames. The same channel
// may also be registered via Register; the browser EventSource demuxes
// regular vs audio events by their `event:` line.
func (a *EventAdapter) RegisterAudio(ch chan []byte) {
	a.audioMu.Lock()
	a.audioClients[ch] = struct{}{}
	a.audioMu.Unlock()
}

// UnregisterAudio removes an audio client channel.
func (a *EventAdapter) UnregisterAudio(ch chan []byte) {
	a.audioMu.Lock()
	delete(a.audioClients, ch)
	a.audioMu.Unlock()
}

// AttachAudioRouter subscribes the adapter to a per-run AudioRouter.
// Frames are encoded as SSE "audio" events for clients that opted in
// via ?audio=1. The subscription lives until the router closes.
func (a *EventAdapter) AttachAudioRouter(runID string, router *arenaaudio.AudioRouter, rate int) {
	if a == nil || router == nil {
		return
	}
	consumer := router.Subscribe("sse-"+runID, audioConsumerBuffer)
	go func() {
		seq := int64(0)
		for frame := range consumer {
			seq++
			msg := encodeAudioSSE(runID, frame, rate, seq)
			a.broadcastAudio(msg)
		}
	}()
}

func (a *EventAdapter) broadcastAudio(msg []byte) {
	a.audioMu.RLock()
	for ch := range a.audioClients {
		select {
		case ch <- msg:
		default:
			// drop — client stalled
		}
	}
	a.audioMu.RUnlock()
}

// audioConsumerBuffer is the per-run AudioRouter subscription buffer for the
// SSE relay. Sized to absorb short bursts without dropping at the router edge.
const audioConsumerBuffer = 50

// bytesPerSample is the byte width of an int16 PCM sample (s16le).
const bytesPerSample = 2

// highByteShift is the bit shift used to extract the high byte of a uint16
// when encoding little-endian s16le.
const highByteShift = 8

// audioSSEPayload is the JSON envelope for an audio frame sent over SSE.
type audioSSEPayload struct {
	Type      string `json:"type"`
	RunID     string `json:"run_id"`
	Direction string `json:"direction"`
	Seq       int64  `json:"seq"`
	Rate      int    `json:"rate"`
	Samples   string `json:"samples"`
}

// encodeAudioSSE produces the `event: audio\ndata: {...}\n\n` byte slice for
// a single frame. JSON envelope keeps the wire format homogeneous with other
// SSE events.
func encodeAudioSSE(runID string, frame arenaaudio.Frame, rate int, seq int64) []byte {
	payload := audioSSEPayload{
		Type:      "audio",
		RunID:     runID,
		Direction: string(frame.Direction),
		Seq:       seq,
		Rate:      rate,
		Samples:   samplesToBase64(frame.Samples),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	out := make([]byte, 0, len("event: audio\ndata: ")+len(data)+len("\n\n"))
	out = append(out, "event: audio\ndata: "...)
	out = append(out, data...)
	out = append(out, '\n', '\n')
	return out
}

// samplesToBase64 encodes int16 PCM samples as little-endian s16le bytes,
// then base64. This matches the wire format expected by the browser's
// AudioWorklet/decoder. The int16→uint16 cast is bit-pattern preserving
// (two's complement) and intentional for s16le encoding.
func samplesToBase64(samples []int16) string {
	b := make([]byte, bytesPerSample*len(samples))
	for i, s := range samples {
		//nolint:gosec // s16le wire format requires bit-pattern reinterpretation of int16 as uint16.
		u := uint16(s)
		b[bytesPerSample*i] = byte(u)
		b[bytesPerSample*i+1] = byte(u >> highByteShift)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// HandleEvent converts a runtime event to JSON and broadcasts to all clients.
func (a *EventAdapter) HandleEvent(event *events.Event) {
	sse := a.mapEvent(event)
	if sse == nil {
		return
	}

	data, err := json.Marshal(sse)
	if err != nil {
		return
	}

	a.broadcast(data)
}

// broadcast fans a pre-marshaled SSE frame out to every registered client,
// dropping it for any client whose buffer is full rather than blocking.
func (a *EventAdapter) broadcast(data []byte) {
	a.mu.RLock()
	for ch := range a.clients {
		select {
		case ch <- data:
		default:
			// Client buffer full — drop event to avoid blocking
		}
	}
	a.mu.RUnlock()
}

// BroadcastFullMessages emits a "message.full" SSE event per message in msgs,
// carrying the complete persisted types.Message (role, content, parts,
// tool_calls, tool_result, timestamp, latency_ms, cost_info, finish_reason,
// meta, validations) rather than the thin projection used by the live
// runtime-event stream. This lets the Inspector show metrics/meta/cost/raw
// JSON for messages as they're persisted.
//
// Delivery is per-client delta: each client is sent only the messages whose
// payload differs from what that client last received. This keeps a long
// conversation from re-sending its whole history on every save (which cost
// O(N) SSE traffic per save, O(N^2) over a run) while preserving the two
// catch-up properties the full re-send used to provide for free:
//
//   - A newly registered or reconnecting client starts with an empty sent-set,
//     so it still receives the complete history on the next save.
//   - A frame dropped because a client's buffer was full is not recorded as
//     sent, so it is retried on the next save rather than lost.
//
// The client reducer keys on index and upserts, so a partial send is safe.
func (a *EventAdapter) BroadcastFullMessages(convID string, msgs []types.Message) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.clients) == 0 {
		return
	}

	cache := &frameCache{
		frames:    make([][]byte, len(msgs)),
		hashes:    make([]uint64, len(msgs)),
		marshaled: make([]bool, len(msgs)),
	}

	for ch, st := range a.clients {
		st.sentFull[convID] = sendDelta(ch, st.sentFull[convID], convID, msgs, cache)
	}
}

// frameCache marshals each message.full frame lazily and at most once, then
// shares it across every client that turns out to need that index.
type frameCache struct {
	frames    [][]byte
	hashes    []uint64
	marshaled []bool
}

// frame returns the SSE frame and payload hash for one message, marshaling on
// first use. A nil frame means the message could not be marshaled.
func (c *frameCache) frame(convID string, i int, msg *types.Message) ([]byte, uint64) {
	if !c.marshaled[i] {
		c.marshaled[i] = true
		c.frames[i], c.hashes[i] = marshalFullMessage(convID, i, msg)
	}
	return c.frames[i], c.hashes[i]
}

// sendDelta delivers to one client the messages whose payload differs from what
// that client last received, and returns its updated sent-set.
func sendDelta(
	ch chan []byte, sent []uint64, convID string, msgs []types.Message, cache *frameCache,
) []uint64 {
	sent = growSentSet(sent, len(msgs))
	for i := range msgs {
		frame, hash := cache.frame(convID, i, &msgs[i])
		// Marshal failed for this message, or the client already has this
		// exact payload — nothing to send.
		if frame == nil || sent[i] == hash {
			continue
		}
		select {
		case ch <- frame:
			sent[i] = hash
		default:
			// Buffer full — leave unsent so the next save retries it.
		}
	}
	return sent
}

// growSentSet extends a client's sent-set to cover n messages, and never
// shrinks it. A save can legitimately carry FEWER messages than the last one:
// the engine re-saves a conversation progressively, from a single system
// message up to the full history, on every turn. Truncating would discard the
// hashes above that low-water mark and re-send the whole history each turn —
// which is the cost this delta exists to avoid.
//
// Keeping stale entries is safe: an index past n is never read, and if it comes
// back the hash comparison decides. Should a conversation genuinely reset, an
// index whose payload is unchanged needs no resend anyway, since the client
// already renders it.
func growSentSet(sent []uint64, n int) []uint64 {
	if len(sent) >= n {
		return sent
	}
	grown := make([]uint64, n)
	copy(grown, sent)
	return grown
}

// marshalFullMessage encodes one message.full SSE frame and returns it with a
// hash of the message payload used for per-client change detection. It returns
// (nil, 0) if the message cannot be marshaled.
func marshalFullMessage(convID string, index int, msg *types.Message) ([]byte, uint64) {
	body, err := json.Marshal(displayMessage(msg))
	if err != nil {
		return nil, 0
	}
	data, err := json.Marshal(&SSEEvent{
		Type:           "message.full",
		ConversationID: convID,
		Data: map[string]interface{}{
			jsonKeyIndex:   index,
			jsonKeyMessage: json.RawMessage(body),
		},
	})
	if err != nil {
		return nil, 0
	}
	return data, hashMessage(body)
}

// hashMessage returns a non-zero FNV-1a hash of a marshaled message, so that
// no real payload is ever indistinguishable from the unsentHash sentinel.
func hashMessage(body []byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write(body)
	if sum := h.Sum64(); sum != unsentHash {
		return sum
	}
	return 1
}

// BroadcastContentDelta emits a "content.delta" SSE frame carrying one chunk of
// assistant text as it is generated.
//
// This exists because there is no content delta on the event bus. The runtime
// publishes reasoning.delta but nothing equivalent for the answer itself —
// assistant text lives only on the MessageStreamChunk channel the interactive
// handler consumes, so streaming it has to be an explicit broadcast rather than
// a mapped event.
//
// Only the delta and its message index travel. MessageStreamChunk also carries
// the whole accumulated Messages slice per chunk, which would mean re-sending
// the conversation on every token.
//
// Like the bus route this is a preview and may be dropped for a slow client:
// message.created and message.full remain authoritative for the finished turn,
// and a consumer must replace the streamed text rather than append to it.
func (a *EventAdapter) BroadcastContentDelta(convID string, index int, delta string) {
	if delta == "" {
		return
	}
	data, err := json.Marshal(&SSEEvent{
		Type:           "content.delta",
		Timestamp:      time.Now(),
		ConversationID: convID,
		Data: map[string]interface{}{
			jsonKeyIndex: index,
			"delta":      delta,
		},
	})
	if err != nil {
		return
	}
	a.broadcast(data)
}

// mapEvent converts a runtime event to an SSEEvent.
func (a *EventAdapter) mapEvent(event *events.Event) *SSEEvent {
	sse := &SSEEvent{
		Type:           string(event.Type),
		Timestamp:      event.Timestamp,
		Sequence:       event.Sequence,
		ExecutionID:    event.ExecutionID,
		ConversationID: event.ConversationID,
	}

	switch data := event.Data.(type) {
	case *events.ProviderCallStartedData:
		sse.Data = map[string]interface{}{
			"provider": data.Provider,
			"model":    data.Model,
		}
	case *events.ProviderCallCompletedData:
		sse.Data = map[string]interface{}{
			"provider": data.Provider,
			"model":    data.Model,
			"duration": data.Duration.Seconds(),
			"cost":     data.Cost,
		}
	case *events.ProviderCallFailedData:
		sse.Data = map[string]interface{}{
			"provider": data.Provider,
			"model":    data.Model,
			"error":    errorString(data.Error),
		}
	case events.MessageCreatedData:
		sse.Data = withReasoning(map[string]interface{}{
			"role":       data.Role,
			"content":    data.Content,
			jsonKeyIndex: data.Index,
			"toolCalls":  data.ToolCalls,
			"toolResult": data.ToolResult,
		}, data.Reasoning)
	case *events.MessageCreatedData:
		// runtime/events/emitter.go emits a pointer; without this case the
		// payload was silently dropped (nil data) and the frontend reducer
		// never aggregated messages into liveRun.messages.
		if data != nil {
			sse.Data = withReasoning(map[string]interface{}{
				"role":       data.Role,
				"content":    data.Content,
				jsonKeyIndex: data.Index,
				"toolCalls":  data.ToolCalls,
				"toolResult": data.ToolResult,
			}, data.Reasoning)
		}
	case *events.ReasoningDeltaData:
		if data != nil {
			sse.Data = reasoningDeltaPayload(data)
		}
	case events.ReasoningDeltaData:
		sse.Data = reasoningDeltaPayload(&data)
	case events.MessageUpdatedData:
		sse.Data = map[string]interface{}{
			jsonKeyIndex:   data.Index,
			"latencyMs":    data.LatencyMs,
			"inputTokens":  data.InputTokens,
			"outputTokens": data.OutputTokens,
			"totalCost":    data.TotalCost,
		}
	case *events.MessageUpdatedData:
		if data != nil {
			sse.Data = map[string]interface{}{
				jsonKeyIndex:   data.Index,
				"latencyMs":    data.LatencyMs,
				"inputTokens":  data.InputTokens,
				"outputTokens": data.OutputTokens,
				"totalCost":    data.TotalCost,
			}
		}
	case events.ConversationStartedData:
		sse.Data = map[string]interface{}{
			"systemPrompt": data.SystemPrompt,
		}
	case events.MiddlewareCompletedData:
		sse.Data = map[string]interface{}{
			"name":     data.Name,
			"duration": data.Duration.Seconds(),
		}
	case events.ToolCallEventData:
		sse.Data = map[string]interface{}{
			"toolName": data.ToolName,
			"callId":   data.CallID,
			"status":   data.Status,
		}
	case *events.ToolCallEventData:
		sse.Data = map[string]interface{}{
			"toolName": data.ToolName,
			"callId":   data.CallID,
			"status":   data.Status,
		}
	case events.ValidationEventData:
		sse.Data = validationPayload(&data)
	case *events.ValidationEventData:
		sse.Data = validationPayload(data)
	case *events.TemplateStartedData:
		sse.Data = map[string]interface{}{
			"taskType":      data.TaskType,
			"variableCount": data.VariableCount,
			"modelOverride": data.ModelOverride,
		}
	case *events.TemplateRenderedData:
		sse.Data = map[string]interface{}{
			"taskType":      data.TaskType,
			"promptHash":    data.PromptHash,
			"variablesUsed": data.VariablesUsed,
			"renderPasses":  data.RenderPasses,
		}
	case *events.TemplateFailedData:
		sse.Data = map[string]interface{}{
			"taskType":   data.TaskType,
			"error":      data.Error,
			"unresolved": data.UnresolvedPlaceholders,
		}
	case events.CustomEventData:
		sse.Data = data.Data
	case *events.CustomEventData:
		sse.Data = data.Data
	default:
		// For unhandled event data types, still send the event type/timestamp
		sse.Data = nil
	}

	return sse
}

// errorString safely converts an error to a string.
func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// validationPayload renders a guardrail firing for the SSE channel.
// Two switch cases (value + pointer) used to inline the same map literal,
// which trips goconst on the "enforced" key.
func validationPayload(data *events.ValidationEventData) map[string]interface{} {
	return map[string]interface{}{
		"validatorName": data.ValidatorName,
		"validatorType": data.ValidatorType,
		"error":         errorString(data.Error),
		"enforced":      data.Enforced,
		"score":         data.Score,
	}
}

// withReasoning adds a message's reasoning trace to an SSE payload, leaving the
// key absent entirely when the turn produced none. Absent rather than null
// matters: an empty trace is ordinary (a one-step answer has nothing to
// summarize), and the client renders a disclosure for any reasoning it is
// handed.
//
// Only the displayable text and the redacted flag cross the wire. Opaque
// entries (provider signatures, encrypted blocks) are deliberately dropped:
// they exist for intra-turn round-trip, are never displayed, and can be large.
//
// ReasoningTrace is carried in-process on MessageCreatedData, whose Reasoning
// field is `json:"-"` upstream, so it has to be mapped explicitly here rather
// than riding along with the struct.
func withReasoning(payload map[string]interface{}, rt *types.ReasoningTrace) map[string]interface{} {
	if rt == nil || (rt.Text == "" && !rt.Redacted) {
		return payload
	}
	payload["reasoning"] = map[string]interface{}{
		"text":     rt.Text,
		"redacted": rt.Redacted,
	}
	return payload
}

// displayMessage returns msg with any opaque reasoning entries removed, so the
// two routes that feed the same browser message agree on what crosses the wire.
//
// message.created maps reasoning field-by-field and carries only text plus the
// redacted flag. message.full marshals the stored types.Message wholesale, and
// ReasoningTrace.Opaque serializes with it — provider-native signatures and
// encrypted blocks that exist for intra-turn round-trip, are never displayed,
// and in practice run larger than the text they accompany (536-1288 bytes per
// message against Claude). Without this the display path would ship them to
// every SSE client.
//
// The copy is shallow and only the Reasoning pointer is replaced, so the stored
// message the caller owns is left untouched.
func displayMessage(msg *types.Message) *types.Message {
	if msg == nil || msg.Reasoning == nil || len(msg.Reasoning.Opaque) == 0 {
		return msg
	}
	out := *msg
	out.Reasoning = &types.ReasoningTrace{
		Text:     msg.Reasoning.Text,
		Redacted: msg.Reasoning.Redacted,
	}
	return &out
}

// reasoningDeltaPayload shapes one reasoning fragment for the browser.
//
// Round and providerCallId are carried alongside the text because they are the
// same join key the round's tool and provider events use. Without them a
// streaming consumer watching a tool loop cannot tell which model turn it is
// watching think, and fragments from two rounds are indistinguishable.
//
// This route is the lossy one: the bus drops events under burst, so a consumer
// must treat these as a preview. The authoritative trace arrives whole on
// message.created once the turn completes.
func reasoningDeltaPayload(d *events.ReasoningDeltaData) map[string]interface{} {
	return map[string]interface{}{
		"text":           d.Text,
		"round":          d.Round,
		"providerCallId": d.ProviderCallID,
	}
}
