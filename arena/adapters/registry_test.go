package adapters

import (
	"fmt"
	"testing"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// mockAdapter is a test adapter for testing the registry.
type mockAdapter struct {
	canHandleFunc func(source string, typeHint string) bool
	enumerateFunc func(source string) ([]RecordingReference, error)
	loadFunc      func(ref RecordingReference) ([]types.Message, *RecordingMetadata, error)
}

func (m *mockAdapter) CanHandle(source string, typeHint string) bool {
	if m.canHandleFunc != nil {
		return m.canHandleFunc(source, typeHint)
	}
	return false
}

func (m *mockAdapter) Enumerate(source string) ([]RecordingReference, error) {
	if m.enumerateFunc != nil {
		return m.enumerateFunc(source)
	}
	return []RecordingReference{{ID: source, Source: source}}, nil
}

func (m *mockAdapter) Load(ref RecordingReference) ([]types.Message, *RecordingMetadata, error) {
	if m.loadFunc != nil {
		return m.loadFunc(ref)
	}
	return nil, nil, nil
}

func TestNewRegistry(t *testing.T) {
	registry := NewRegistry()
	if registry == nil {
		t.Fatal("NewRegistry() returned nil")
	}

	// Should have built-in adapters registered
	if len(registry.adapters) == 0 {
		t.Error("NewRegistry() should register built-in adapters")
	}
}

func TestRegistry_Register(t *testing.T) {
	registry := &Registry{
		adapters: make([]RecordingAdapter, 0),
	}

	adapter := &mockAdapter{}
	registry.Register(adapter)

	if len(registry.adapters) != 1 {
		t.Errorf("Register() did not add adapter, got %d adapters", len(registry.adapters))
	}
}

func TestRegistry_FindAdapter(t *testing.T) {
	registry := &Registry{
		adapters: make([]RecordingAdapter, 0),
	}

	// Register a mock adapter that handles .test files
	mockAdptr := &mockAdapter{
		canHandleFunc: func(path string, typeHint string) bool {
			return hasExtension(path, ".test")
		},
	}
	registry.Register(mockAdptr)

	tests := []struct {
		name     string
		path     string
		typeHint string
		want     bool
	}{
		{
			name:     "finds adapter for .test extension",
			path:     "file.test",
			typeHint: "",
			want:     true,
		},
		{
			name:     "does not find adapter for .other extension",
			path:     "file.other",
			typeHint: "",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := registry.FindAdapter(tt.path, tt.typeHint)
			got := adapter != nil
			if got != tt.want {
				t.Errorf("FindAdapter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRegistry_Load(t *testing.T) {
	registry := &Registry{
		adapters: make([]RecordingAdapter, 0),
	}

	// Register a mock adapter
	expectedMessages := []types.Message{
		{Role: "user", Content: "test"},
	}
	expectedMetadata := &RecordingMetadata{
		SessionID: "test-session",
	}

	mockAdptr := &mockAdapter{
		canHandleFunc: func(source string, typeHint string) bool {
			return hasExtension(source, ".test")
		},
		loadFunc: func(ref RecordingReference) ([]types.Message, *RecordingMetadata, error) {
			return expectedMessages, expectedMetadata, nil
		},
	}
	registry.Register(mockAdptr)

	ref := RecordingReference{ID: "file.test", Source: "file.test"}
	messages, metadata, err := registry.Load(ref)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(messages) != len(expectedMessages) {
		t.Errorf("Load() got %d messages, want %d", len(messages), len(expectedMessages))
	}

	if metadata.SessionID != expectedMetadata.SessionID {
		t.Errorf("Load() got session ID %s, want %s", metadata.SessionID, expectedMetadata.SessionID)
	}
}

func TestRegistry_Load_NoAdapter(t *testing.T) {
	registry := &Registry{
		adapters: make([]RecordingAdapter, 0),
	}

	ref := RecordingReference{ID: "file.unknown", Source: "file.unknown"}
	_, _, err := registry.Load(ref)
	if err == nil {
		t.Error("Load() should return error when no adapter found")
	}
}

func TestRegistry_Enumerate(t *testing.T) {
	registry := &Registry{
		adapters: make([]RecordingAdapter, 0),
	}

	// Register a mock adapter with enumerate function
	mockAdptr := &mockAdapter{
		canHandleFunc: func(source string, typeHint string) bool {
			return hasExtension(source, ".test")
		},
		enumerateFunc: func(source string) ([]RecordingReference, error) {
			return []RecordingReference{
				{ID: "file1.test", Source: source, TypeHint: "test"},
				{ID: "file2.test", Source: source, TypeHint: "test"},
			}, nil
		},
	}
	registry.Register(mockAdptr)

	refs, err := registry.Enumerate("*.test", "")
	if err != nil {
		t.Fatalf("Enumerate() error = %v", err)
	}

	if len(refs) != 2 {
		t.Errorf("Enumerate() got %d refs, want 2", len(refs))
	}
}

func TestRegistry_Enumerate_NoAdapter(t *testing.T) {
	registry := &Registry{
		adapters: make([]RecordingAdapter, 0),
	}

	_, err := registry.Enumerate("file.unknown", "")
	if err == nil {
		t.Error("Enumerate() should return error when no adapter found")
	}
}

func TestHasExtension(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		extensions []string
		want       bool
	}{
		{
			name:       "matches single extension",
			path:       "file.json",
			extensions: []string{".json"},
			want:       true,
		},
		{
			name:       "matches one of multiple extensions",
			path:       "file.yaml",
			extensions: []string{".json", ".yaml", ".yml"},
			want:       true,
		},
		{
			name:       "case insensitive match",
			path:       "file.JSON",
			extensions: []string{".json"},
			want:       true,
		},
		{
			name:       "does not match different extension",
			path:       "file.txt",
			extensions: []string{".json"},
			want:       false,
		},
		{
			name:       "matches .recording.json compound extension",
			path:       "file.recording.json",
			extensions: []string{".recording.json"},
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasExtension(tt.path, tt.extensions...)
			if got != tt.want {
				t.Errorf("hasExtension() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchesTypeHint(t *testing.T) {
	tests := []struct {
		name     string
		typeHint string
		types    []string
		want     bool
	}{
		{
			name:     "matches exact type",
			typeHint: "session",
			types:    []string{"session"},
			want:     true,
		},
		{
			name:     "matches one of multiple types",
			typeHint: "transcript",
			types:    []string{"session", "transcript", "arena"},
			want:     true,
		},
		{
			name:     "case insensitive match",
			typeHint: "SESSION",
			types:    []string{"session"},
			want:     true,
		},
		{
			name:     "trims whitespace",
			typeHint: " session ",
			types:    []string{"session"},
			want:     true,
		},
		{
			name:     "does not match different type",
			typeHint: "other",
			types:    []string{"session"},
			want:     false,
		},
		{
			name:     "empty type hint returns false",
			typeHint: "",
			types:    []string{"session"},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesTypeHint(tt.typeHint, tt.types...)
			if got != tt.want {
				t.Errorf("matchesTypeHint() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRecordingMetadata(t *testing.T) {
	// Test RecordingMetadata structure
	metadata := &RecordingMetadata{
		JudgeTargets: map[string]ProviderSpec{
			"default": {
				Type:  "openai",
				Model: "gpt-4",
				ID:    "gpt-4-judge",
			},
		},
		ProviderInfo: map[string]interface{}{
			"provider_id": "test-provider",
			"model":       "test-model",
		},
		Tags:       []string{"tag1", "tag2"},
		Timestamps: []time.Time{time.Now(), time.Now().Add(time.Minute)},
		SessionID:  "test-session",
		Duration:   time.Minute,
		Extras: map[string]interface{}{
			"custom_field": "value",
		},
	}

	if metadata.SessionID != "test-session" {
		t.Errorf("SessionID = %s, want test-session", metadata.SessionID)
	}

	if len(metadata.JudgeTargets) != 1 {
		t.Errorf("JudgeTargets length = %d, want 1", len(metadata.JudgeTargets))
	}

	if metadata.JudgeTargets["default"].Type != "openai" {
		t.Errorf("JudgeTargets[default].Type = %s, want openai", metadata.JudgeTargets["default"].Type)
	}

	if len(metadata.Tags) != 2 {
		t.Errorf("Tags length = %d, want 2", len(metadata.Tags))
	}

	if metadata.Duration != time.Minute {
		t.Errorf("Duration = %v, want 1m", metadata.Duration)
	}
}

func TestProviderSpec(t *testing.T) {
	spec := ProviderSpec{
		Type:  "openai",
		Model: "gpt-4",
		ID:    "gpt-4-judge",
	}

	if spec.Type != "openai" {
		t.Errorf("Type = %s, want openai", spec.Type)
	}
	if spec.Model != "gpt-4" {
		t.Errorf("Model = %s, want gpt-4", spec.Model)
	}
	if spec.ID != "gpt-4-judge" {
		t.Errorf("ID = %s, want gpt-4-judge", spec.ID)
	}
}

// The default registry must route what people actually have on disk: run
// output from `promptarena run` (plain .json), PromptKit session recordings
// (.recording.json or the event store's .jsonl) and transcripts.
func TestNewRegistry_RoutesRealFileShapes(t *testing.T) {
	r := NewRegistry()
	tests := []struct {
		source string
		want   RecordingAdapter
	}{
		{"out/2026-08-31T19-48-12Z-0001_gemini_default_tool-usage_3a7b0b83_000b.json", &ArenaOutputAdapter{}},
		{"out/*.json", &ArenaOutputAdapter{}},
		{"recordings/customer-support.arena.json", &ArenaOutputAdapter{}},
		{"recordings/geography.recording.json", &SessionRecordingAdapter{}},
		{"out/recordings/run-123.jsonl", &SessionRecordingAdapter{}},
		{"conv.transcript.yaml", &TranscriptAdapter{}},
	}
	for _, tt := range tests {
		got := r.FindAdapter(tt.source, "")
		if got == nil {
			t.Errorf("FindAdapter(%q) = nil", tt.source)
			continue
		}
		if fmt.Sprintf("%T", got) != fmt.Sprintf("%T", tt.want) {
			t.Errorf("FindAdapter(%q) = %T, want %T", tt.source, got, tt.want)
		}
	}
}

// An eval that names its recording format (type: mock, say, for a custom
// adapter) must reach that adapter even when the file's extension would
// otherwise route to a built-in. Built-ins only auto-detect when no hint is
// given.
func TestNewRegistry_ExplicitHintBeatsExtension(t *testing.T) {
	r := NewRegistry()
	custom := &mockAdapter{
		canHandleFunc: func(_ string, hint string) bool { return hint == "mock" },
	}
	r.Register(custom)

	if got := r.FindAdapter("test.json", "mock"); got != custom {
		t.Errorf("FindAdapter(test.json, mock) = %T, want the custom adapter", got)
	}
	if got := r.FindAdapter("test.json", "session"); fmt.Sprintf("%T", got) != "*adapters.SessionRecordingAdapter" {
		t.Errorf("FindAdapter(test.json, session) = %T, want SessionRecordingAdapter", got)
	}
	if got := r.FindAdapter("test.json", ""); fmt.Sprintf("%T", got) != "*adapters.ArenaOutputAdapter" {
		t.Errorf("FindAdapter(test.json, \"\") = %T, want ArenaOutputAdapter by extension", got)
	}
}
