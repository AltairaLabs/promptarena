package stages

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/v2/events"
	"github.com/AltairaLabs/PromptKit/runtime/v2/hooks"
	"github.com/AltairaLabs/PromptKit/runtime/v2/hooks/guardrails"
	"github.com/AltairaLabs/PromptKit/runtime/v2/pipeline/stage"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// runStage feeds elems through s and returns everything it emitted.
func runDuplexStage(t *testing.T, s stage.Stage, elems ...stage.StreamElement) ([]stage.StreamElement, error) {
	t.Helper()
	in := make(chan stage.StreamElement, len(elems))
	for _, e := range elems {
		in <- e
	}
	close(in)
	out := make(chan stage.StreamElement, 4*len(elems)+1)
	err := s.Process(context.Background(), in, out)
	var got []stage.StreamElement
	for e := range out {
		got = append(got, e)
	}
	return got, err
}

// assistantTurn is the element DuplexProviderStage emits at turn completion:
// the turn's message on an EndOfStream element.
func assistantTurn(text string) stage.StreamElement {
	msg := &types.Message{
		Role:    "assistant",
		Content: text,
		Parts: []types.ContentPart{
			types.NewTextPart(text),
			{Type: types.ContentTypeAudio, Media: &types.MediaContent{MIMEType: "audio/pcm"}},
		},
		ToolCalls: []types.MessageToolCall{{ID: "c1", Name: "lookup"}},
	}
	return stage.StreamElement{Message: msg, EndOfStream: true}
}

func piiHooks(t *testing.T) *hooks.Registry {
	t.Helper()
	compiled, err := guardrails.CompileValidators([]prompt.ValidatorConfig{
		{Type: "banned_words", Params: map[string]interface{}{"words": []interface{}{"secret"}}},
	})
	if err != nil {
		t.Fatalf("compile validators: %v", err)
	}
	opts := make([]hooks.Option, 0, len(compiled))
	for _, h := range compiled {
		opts = append(opts, hooks.WithProviderHook(h))
	}
	return hooks.NewRegistry(opts...)
}

func TestDuplexGuardrailStage_EnforcesOnAssistantTurn(t *testing.T) {
	bus := events.NewEventBus()
	defer bus.Close()
	var mu sync.Mutex
	var failed []*events.Event
	bus.Subscribe(events.EventValidationFailed, func(e *events.Event) {
		mu.Lock()
		failed = append(failed, e)
		mu.Unlock()
	})
	emitter := events.NewEmitter(bus, "run", "run", "conv")

	user := stage.NewMessageElement(&types.Message{Role: "user", Content: "tell me"})
	clean := assistantTurn("hello there")
	dirty := assistantTurn("the secret is 42")

	s := NewDuplexGuardrailStage(piiHooks(t), emitter, "mock-duplex", "m")
	got, err := runDuplexStage(t, s, user, clean, dirty)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want every element forwarded, got %d", len(got))
	}

	if c := got[1].Message; len(c.Validations) != 0 || c.Content != "hello there" {
		t.Fatalf("clean turn must pass untouched, got %+v", c)
	}

	d := got[2].Message
	if strings.Contains(d.Content, "secret") {
		t.Fatalf("enforced content still holds the banned word: %q", d.Content)
	}
	if !got[2].EndOfStream || got[2].Error != nil {
		t.Fatal("enforcement must keep the turn's control flags and add no error")
	}
	if len(d.Validations) != 1 || d.Validations[0].ValidatorType != "banned_words" || d.Validations[0].Passed {
		t.Fatalf("want one failed banned_words validation, got %+v", d.Validations)
	}
	if d.Validations[0].Details["direction"] != hooks.DirectionOutput {
		t.Fatalf("validation must be tagged output, got %v", d.Validations[0].Details)
	}
	if d.FinishReason != types.FinishReasonSafety {
		t.Fatalf("want finish reason safety, got %q", d.FinishReason)
	}
	var texts int
	for _, p := range d.Parts {
		if p.Type == types.ContentTypeText {
			texts++
			if *p.Text != d.Content {
				t.Fatalf("text part must match enforced content, got %q", *p.Text)
			}
		}
	}
	if texts != 1 || len(d.Parts) != 2 {
		t.Fatalf("want the text part replaced and the audio kept, got %+v", d.Parts)
	}
	if len(d.ToolCalls) != 1 {
		t.Fatal("tool calls the session already ran must be kept")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(failed)
		mu.Unlock()
		if n == 1 || time.Now().After(deadline) {
			if n != 1 {
				t.Fatalf("want one validation.failed event, got %d", n)
			}
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// denyHook denies without enforcing, like a hand-written host hook.
type denyHook struct{ metadata map[string]any }

func (denyHook) Name() string { return "deny" }
func (denyHook) BeforeCall(context.Context, *hooks.ProviderRequest) hooks.Decision {
	return hooks.Allow
}
func (h denyHook) AfterCall(context.Context, *hooks.ProviderRequest, *hooks.ProviderResponse) hooks.Decision {
	return hooks.DenyWithMetadata("nope", h.metadata)
}

func TestDuplexGuardrailStage_DenyFailsTheTurn(t *testing.T) {
	score := 0.25
	reg := hooks.NewRegistry(hooks.WithProviderHook(denyHook{metadata: map[string]any{
		"validator_type": "custom", "score": &score,
	}}))
	s := NewDuplexGuardrailStage(reg, nil, "p", "m")
	got, err := runDuplexStage(t, s, assistantTurn("anything"))
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	var denied *hooks.HookDeniedError
	if !errors.As(got[0].Error, &denied) || denied.Reason != "nope" {
		t.Fatalf("want a HookDeniedError on the element, got %v", got[0].Error)
	}
	v := got[0].Message.Validations
	if len(v) != 1 || v[0].Details["score"] != 0.25 {
		t.Fatalf("want the firing recorded with a plain float score, got %+v", v)
	}
	if got[0].Message.Content != "anything" {
		t.Fatal("a non-enforcing denial must not rewrite the content")
	}
}

func TestDuplexGuardrailStage_UntypedDenialRecordsNoValidation(t *testing.T) {
	reg := hooks.NewRegistry(hooks.WithProviderHook(denyHook{}))
	got, _ := runDuplexStage(t, NewDuplexGuardrailStage(reg, nil, "p", "m"), assistantTurn("x"))
	if len(got[0].Message.Validations) != 0 {
		t.Fatal("a decision with no validator_type has no validation to record")
	}
	if got[0].Error == nil {
		t.Fatal("the denial must still fail the turn")
	}
}

func TestDecisionScoreAndDetails(t *testing.T) {
	if decisionScore(map[string]any{"score": 0.5}) != 0.5 {
		t.Fatal("plain float score")
	}
	var nilScore *float64
	if decisionScore(map[string]any{"score": nilScore}) != 0 {
		t.Fatal("nil pointer score reads as 0")
	}
	if decisionScore(nil) != 0 {
		t.Fatal("missing score reads as 0")
	}
	d := firingDetails(hooks.Decision{Metadata: map[string]any{
		hooks.MetadataKeyValidatorDeclaration: "decl", "k": "v",
	}})
	if _, ok := d[hooks.MetadataKeyValidatorDeclaration]; ok || d["k"] != "v" {
		t.Fatalf("details must drop the declaration and keep the rest, got %v", d)
	}
}

func TestApplyEnforcedTurn_MergesMetaAndValidations(t *testing.T) {
	ts := time.Now()
	existing := types.ValidationResult{ValidatorType: "a", Timestamp: ts}
	msg := &types.Message{Content: "old", Validations: []types.ValidationResult{existing}}
	enforced := &types.Message{Content: "new", Validations: []types.ValidationResult{
		existing, {ValidatorType: "b", Timestamp: ts},
	}}
	applyEnforcedTurn(msg, enforced, hooks.Decision{Metadata: map[string]any{
		"reason": "x", hooks.MetadataKeyValidatorDeclaration: "decl",
	}})
	if msg.Content != "new" || len(msg.Validations) != 2 {
		t.Fatalf("want content replaced and one new validation, got %+v", msg)
	}
	if msg.Meta["reason"] != "x" || msg.Meta[hooks.MetadataKeyValidatorDeclaration] != nil {
		t.Fatalf("meta must take the decision metadata minus the declaration, got %v", msg.Meta)
	}
	if len(msg.Parts) != 0 {
		t.Fatal("a message with no parts gains none")
	}
}

// recordingStore captures what RecordingStage appends.
type recordingStore struct {
	events.EventStore
	mu  sync.Mutex
	got []*events.Event
}

func (r *recordingStore) Append(_ context.Context, e *events.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, e)
	return nil
}

func chain(t *testing.T, stages []stage.Stage, elems ...stage.StreamElement) []stage.StreamElement {
	t.Helper()
	for _, s := range stages {
		var err error
		elems, err = runDuplexStage(t, s, elems...)
		if err != nil {
			t.Fatalf("%s: %v", s.Name(), err)
		}
	}
	return elems
}

func TestDuplexOutputRecordingStages_RecordsTurnMessageOnce(t *testing.T) {
	store := &recordingStore{}
	cfg := stage.DefaultRecordingStageConfig()
	cfg.Position = stage.RecordingPositionOutput
	cfg.SessionID = "s"

	text := "partial"
	turn := assistantTurn("the reply")
	turn.Text = &text
	turn.Audio = &stage.AudioData{Samples: []byte{1, 2, 3, 4}, SampleRate: 16000, Channels: 1}
	user := stage.NewMessageElement(&types.Message{Role: "user", Content: "hi"})
	errElem := stage.StreamElement{EndOfStream: true, Error: errors.New("boom"), Message: &types.Message{Role: "assistant"}}
	bare := stage.StreamElement{EndOfTurn: true}

	out := chain(t, DuplexOutputRecordingStages(store, cfg), user, turn, errElem, bare)

	if len(out) != 4 {
		t.Fatalf("downstream must see exactly the original stream, got %d elements", len(out))
	}
	for _, e := range out {
		if e.Source == recordingCopySource {
			t.Fatal("a recording copy leaked downstream")
		}
	}
	if out[1].Message == nil || !out[1].EndOfStream {
		t.Fatal("the turn element must reach downstream intact")
	}

	var created []string
	audio := 0
	for _, e := range store.got {
		switch e.Type {
		case events.EventMessageCreated:
			created = append(created, e.Data.(*events.MessageCreatedData).Role)
		case events.EventAudioOutput:
			audio++
		}
	}
	// user, the turn (via its copy), and the errored element's message, which
	// RecordingStage records itself because an error is not skipped as control.
	if strings.Join(created, ",") != "user,assistant,assistant" {
		t.Fatalf("want each message recorded exactly once, got %v", created)
	}
	if audio != 1 {
		t.Fatalf("want the turn's last audio chunk recorded, got %d", audio)
	}
}

func TestRecordableCopies_ContentElementHasNone(t *testing.T) {
	e := stage.NewMessageElement(&types.Message{Role: "assistant"})
	if c := recordableCopies(&e); c != nil {
		t.Fatalf("a content element is recorded as it is, got %d copies", len(c))
	}
}
