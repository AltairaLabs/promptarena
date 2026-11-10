package tui

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/tui/logging"

	"github.com/AltairaLabs/PromptKit/runtime/v2/events"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

func TestEventAdapterHandlesRunLifecycle(t *testing.T) {
	t.Parallel()

	model := NewModel("cfg", 1)
	adapter := NewEventAdapterWithModel(model)

	start := &events.Event{
		Type:        events.EventType("arena.run.started"),
		ExecutionID: "run-1",
		Timestamp:   time.Now(),
		Data: events.CustomEventData{
			Data: map[string]interface{}{
				"scenario": "sc-1",
				"provider": "prov-1",
				"region":   "us-east",
			},
		},
	}
	complete := &events.Event{
		Type:        events.EventType("arena.run.completed"),
		ExecutionID: "run-1",
		Timestamp:   time.Now(),
		Data: events.CustomEventData{
			Data: map[string]interface{}{
				"duration": time.Second,
				"cost":     0.1,
			},
		},
	}

	adapter.HandleEvent(start)
	adapter.HandleEvent(complete)

	if len(model.activeRuns) == 0 {
		t.Fatalf("expected active run to be created from events")
	}
	if model.completedCount == 0 {
		t.Fatalf("expected run completion to increment completed count")
	}
}

// TestEventAdapterPopulatesRunFieldsFromPointerData guards the pointer-vs-value
// bug: emitter.EmitCustom always emits *events.CustomEventData, so the adapter's
// read helpers must accept the pointer shape. If they only match the value type,
// scenario/provider/region/duration/cost silently arrive as zero values.
func TestEventAdapterPopulatesRunFieldsFromPointerData(t *testing.T) {
	t.Parallel()

	model := NewModel("cfg", 1)
	adapter := NewEventAdapterWithModel(model)

	start := &events.Event{
		Type:        events.EventType("arena.run.started"),
		ExecutionID: "run-1",
		Timestamp:   time.Now(),
		Data: &events.CustomEventData{
			Data: map[string]interface{}{
				"scenario": "sc-1",
				"provider": "prov-1",
				"region":   "us-east",
			},
		},
	}
	complete := &events.Event{
		Type:        events.EventType("arena.run.completed"),
		ExecutionID: "run-1",
		Timestamp:   time.Now(),
		Data: &events.CustomEventData{
			Data: map[string]interface{}{
				"duration": 3 * time.Second,
				"cost":     0.42,
			},
		},
	}

	adapter.HandleEvent(start)
	require.Len(t, model.activeRuns, 1)
	assert.Equal(t, "sc-1", model.activeRuns[0].Scenario)
	assert.Equal(t, "prov-1", model.activeRuns[0].Provider)
	assert.Equal(t, "us-east", model.activeRuns[0].Region)

	adapter.HandleEvent(complete)
	require.Len(t, model.activeRuns, 1)
	assert.Equal(t, 3*time.Second, model.activeRuns[0].Duration)
	assert.InEpsilon(t, 0.42, model.activeRuns[0].Cost, 1e-9)
}

func TestEventAdapterLogsProviderEvents(t *testing.T) {
	t.Parallel()

	model := NewModel("cfg", 1)
	adapter := NewEventAdapterWithModel(model)

	adapter.HandleEvent(&events.Event{
		Type:      events.EventProviderCallStarted,
		Timestamp: time.Now(),
		Data: &events.ProviderCallStartedData{
			Provider: "prov",
			Model:    "model",
		},
	})

	if len(model.logs) == 0 {
		t.Fatalf("expected logs to receive provider event")
	}
}

// TestEventAdapter_AllEventTypes pins the log line each provider and legacy
// middleware event becomes, and that pipeline lifecycle events are not
// logged. Tool, validation, context, state and stream events are pinned
// through the real emitter in TestEventAdapter_EmitterEventsReachTheTUI.
func TestEventAdapter_AllEventTypes(t *testing.T) {
	t.Parallel()

	adapter := NewEventAdapter(nil)
	boom := fmt.Errorf("boom")

	testCases := []struct {
		name      string
		eventType events.EventType
		data      events.EventData
		level     string // "" means the event maps to no TUI message
		message   string
	}{
		{
			name:      "pipeline started is not logged",
			eventType: events.EventPipelineStarted,
			data:      &events.PipelineStartedData{MiddlewareCount: 5},
		},
		{
			name:      "pipeline completed is not logged",
			eventType: events.EventPipelineCompleted,
			data:      &events.PipelineCompletedData{Duration: time.Second},
		},
		{
			name:      "pipeline failed is not logged",
			eventType: events.EventPipelineFailed,
			data:      &events.PipelineFailedData{Error: boom},
		},
		{
			name:      "middleware started",
			eventType: events.EventMiddlewareStarted,
			data:      &events.MiddlewareStartedData{Name: "auth"},
			level:     "INFO",
			message:   "Middleware started: auth",
		},
		{
			name:      "middleware completed",
			eventType: events.EventMiddlewareCompleted,
			data:      &events.MiddlewareCompletedData{Name: "auth", Duration: 250 * time.Millisecond},
			level:     "INFO",
			message:   "Middleware completed: auth (0.25s)",
		},
		{
			name:      "middleware failed",
			eventType: events.EventMiddlewareFailed,
			data:      &events.MiddlewareFailedData{Name: "auth", Error: boom},
			level:     "ERROR",
			message:   "Middleware failed: auth (boom)",
		},
		{
			name:      "provider call started",
			eventType: events.EventProviderCallStarted,
			data:      &events.ProviderCallStartedData{Provider: "openai", Model: "gpt-4o"},
			level:     "INFO",
			message:   "Provider call started: openai gpt-4o",
		},
		{
			name:      "provider call completed",
			eventType: events.EventProviderCallCompleted,
			data: &events.ProviderCallCompletedData{
				Provider: "openai", Model: "gpt-4o", Duration: 1500 * time.Millisecond, Cost: 0.0123,
			},
			level:   "INFO",
			message: "Provider call completed: openai gpt-4o (1.50s, cost $0.0123)",
		},
		{
			name:      "provider call failed",
			eventType: events.EventProviderCallFailed,
			data:      &events.ProviderCallFailedData{Provider: "openai", Model: "gpt-4o", Error: boom},
			level:     "ERROR",
			message:   "Provider call failed: openai gpt-4o (boom)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			msg := adapter.mapEvent(&events.Event{Type: tc.eventType, Timestamp: time.Now(), Data: tc.data})
			if tc.level == "" {
				assert.Nil(t, msg, "expected no TUI message, got %#v", msg)
				return
			}
			lm, ok := msg.(logging.Msg)
			require.True(t, ok, "expected logging.Msg, got %T", msg)
			assert.Equal(t, tc.level, lm.Level)
			assert.Equal(t, tc.message, lm.Message)
		})
	}
}

func TestEventAdapter_Subscribe(t *testing.T) {
	t.Parallel()

	bus := events.NewEventBus()
	model := NewModel("cfg", 1)
	adapter := NewEventAdapterWithModel(model)

	// Subscribe adapter to bus
	adapter.Subscribe(bus)

	// Emit an event
	bus.Publish(&events.Event{
		Type:      events.EventProviderCallStarted,
		Timestamp: time.Now(),
		Data:      &events.ProviderCallStartedData{Provider: "test-provider", Model: "test-model"},
	})

	// Give the event a moment to process
	time.Sleep(10 * time.Millisecond)

	// Verify the event was received by checking logs
	model.mu.Lock()
	logCount := len(model.logs)
	model.mu.Unlock()
	if logCount == 0 {
		t.Error("expected adapter to receive event after subscription")
	}
}

func TestEventAdapter_SubscribeNilBus(t *testing.T) {
	t.Parallel()

	adapter := NewEventAdapter(nil)
	// Should not panic
	adapter.Subscribe(nil)
}

func TestEventAdapter_HelperFunctions(t *testing.T) {
	t.Parallel()

	t.Run("readError with error value", func(t *testing.T) {
		testErr := fmt.Errorf("validation failed")
		data := events.CustomEventData{
			Data: map[string]interface{}{
				"error": testErr,
			},
		}
		err := readError(data, "error")
		if err == nil {
			t.Error("expected non-nil error")
		}
	})

	t.Run("readError with string value", func(t *testing.T) {
		data := events.CustomEventData{
			Data: map[string]interface{}{
				"error": "something went wrong",
			},
		}
		err := readError(data, "error")
		if err == nil {
			t.Error("expected non-nil error")
		}
		if err.Error() != "something went wrong" {
			t.Errorf("expected error message 'something went wrong', got '%s'", err.Error())
		}
	})

	t.Run("readError with nil data", func(t *testing.T) {
		data := events.CustomEventData{
			Data: map[string]interface{}{},
		}
		err := readError(data, "error")
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("readInt", func(t *testing.T) {
		data := events.CustomEventData{
			Data: map[string]interface{}{
				"turn_index": 5,
			},
		}
		val := readInt(data, "turn_index")
		if val != 5 {
			t.Errorf("expected 5, got %d", val)
		}
	})

	t.Run("readInt missing key", func(t *testing.T) {
		data := events.CustomEventData{
			Data: map[string]interface{}{},
		}
		val := readInt(data, "missing")
		if val != 0 {
			t.Errorf("expected 0 for missing key, got %d", val)
		}
	})

	t.Run("ternary true", func(t *testing.T) {
		result := ternary(true, "yes", "no")
		if result != "yes" {
			t.Errorf("expected 'yes', got '%s'", result)
		}
	})

	t.Run("ternary false", func(t *testing.T) {
		result := ternary(false, "yes", "no")
		if result != "no" {
			t.Errorf("expected 'no', got '%s'", result)
		}
	})
}

func TestEventAdapter_ArenaCustomEvents(t *testing.T) {
	t.Parallel()

	model := NewModel("cfg", 1)
	adapter := NewEventAdapterWithModel(model)

	t.Run("arena run failed", func(t *testing.T) {
		evt := &events.Event{
			Type:        events.EventType("arena.run.failed"),
			ExecutionID: "run-fail",
			Timestamp:   time.Now(),
			Data: events.CustomEventData{
				Data: map[string]interface{}{
					"error": "test failure",
				},
			},
		}
		adapter.HandleEvent(evt)
		if model.failedCount == 0 {
			t.Error("expected failed count to increment")
		}
	})

	t.Run("arena turn events", func(t *testing.T) {
		turnModel := NewModel("cfg", 1)
		turnAdapter := NewEventAdapterWithModel(turnModel)
		turnAdapter.HandleEvent(&events.Event{
			Type:        events.EventType("arena.run.started"),
			ExecutionID: "run-1",
			Timestamp:   time.Now(),
			Data:        events.CustomEventData{Data: map[string]interface{}{"scenario": "test-scenario"}},
		})

		turnAdapter.HandleEvent(&events.Event{
			Type:        events.EventType("arena.turn.started"),
			ExecutionID: "run-1",
			Timestamp:   time.Now(),
			Data: events.CustomEventData{
				Data: map[string]interface{}{
					"turn_index": 1,
					"role":       "user",
					"scenario":   "test-scenario",
				},
			},
		})
		require.Len(t, turnModel.activeRuns, 1)
		assert.Equal(t, "user", turnModel.activeRuns[0].CurrentTurnRole)
		assert.Equal(t, 1, turnModel.activeRuns[0].CurrentTurnIndex)
		assert.Equal(t, "Turn 2 (user) started: run-1", turnModel.logs[len(turnModel.logs)-1].Message)

		turnAdapter.HandleEvent(&events.Event{
			Type:        events.EventType("arena.turn.failed"),
			ExecutionID: "run-1",
			Timestamp:   time.Now(),
			Data: &events.CustomEventData{
				Data: map[string]interface{}{
					"turn_index": 1,
					"role":       "assistant",
					"error":      "provider timeout",
				},
			},
		})
		assert.Equal(t, "assistant", turnModel.activeRuns[0].CurrentTurnRole)
		assert.Equal(t, "provider timeout", turnModel.activeRuns[0].Error)
		last := turnModel.logs[len(turnModel.logs)-1]
		assert.Equal(t, logLevelError, last.Level)
		assert.Equal(t, "Turn 2 (assistant) failed: run-1 - provider timeout", last.Message)
	})

	t.Run("unknown event type ignored", func(t *testing.T) {
		quietModel := NewModel("cfg", 1)
		NewEventAdapterWithModel(quietModel).HandleEvent(&events.Event{
			Type:      events.EventType("unknown.event"),
			Timestamp: time.Now(),
			Data:      events.CustomEventData{Data: map[string]interface{}{"error": "ignored"}},
		})
		assert.Empty(t, quietModel.logs)
		assert.Empty(t, quietModel.activeRuns)
		assert.Zero(t, quietModel.completedCount)
	})
}

func TestEventAdapter_SendMethods(t *testing.T) {
	t.Parallel()

	t.Run("send with nil send func", func(t *testing.T) {
		adapter := NewEventAdapter(nil)
		// Should not panic with nil send func
		adapter.deliver(logging.Msg{Message: "test"})
	})

	t.Run("send with model", func(t *testing.T) {
		model := NewModel("cfg", 1)
		adapter := NewEventAdapterWithModel(model)
		adapter.deliver(logging.Msg{Level: "INFO", Message: "test message"})
		if len(model.logs) == 0 {
			t.Error("expected log message to be added to model")
		}
	})

}

func TestEventAdapter_EventErrorExtraction(t *testing.T) {
	t.Parallel()

	testErr := fmt.Errorf("test error")

	testCases := []struct {
		name     string
		data     events.EventData
		expected error
	}{
		{
			name:     "ProviderCallFailedData",
			data:     &events.ProviderCallFailedData{Error: testErr},
			expected: testErr,
		},
		{
			name:     "MiddlewareFailedData",
			data:     events.MiddlewareFailedData{Error: testErr},
			expected: testErr,
		},
		{
			name:     "ToolCallFailedData",
			data:     events.ToolCallFailedData{Error: testErr},
			expected: testErr,
		},
		{
			name:     "ValidationFailedData",
			data:     events.ValidationFailedData{Error: testErr},
			expected: testErr,
		},
		{
			name:     "other data type",
			data:     events.PipelineStartedData{},
			expected: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			evt := &events.Event{
				Type: events.EventPipelineFailed,
				Data: tc.data,
			}
			err := eventError(evt)
			if err != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, err)
			}
		})
	}
}

func TestEventAdapter_NameExtraction(t *testing.T) {
	t.Parallel()

	t.Run("providerName and providerModel", func(t *testing.T) {
		testCases := []struct {
			data          events.EventData
			expectedName  string
			expectedModel string
		}{
			{
				data:          &events.ProviderCallStartedData{Provider: "openai", Model: "gpt-4"},
				expectedName:  "openai",
				expectedModel: "gpt-4",
			},
			{
				data:          &events.ProviderCallCompletedData{Provider: "anthropic", Model: "claude-3"},
				expectedName:  "anthropic",
				expectedModel: "claude-3",
			},
			{
				data:          &events.ProviderCallFailedData{Provider: "mock", Model: "test"},
				expectedName:  "mock",
				expectedModel: "test",
			},
			{
				data:          events.PipelineStartedData{},
				expectedName:  "",
				expectedModel: "",
			},
		}

		for _, tc := range testCases {
			evt := &events.Event{Data: tc.data}
			name := providerName(evt)
			model := providerModel(evt)
			if name != tc.expectedName {
				t.Errorf("expected provider name '%s', got '%s'", tc.expectedName, name)
			}
			if model != tc.expectedModel {
				t.Errorf("expected provider model '%s', got '%s'", tc.expectedModel, model)
			}
		}
	})

	t.Run("middlewareName", func(t *testing.T) {
		testCases := []struct {
			data         events.EventData
			expectedName string
		}{
			{data: events.MiddlewareStartedData{Name: "auth"}, expectedName: "auth"},
			{data: events.MiddlewareCompletedData{Name: "logging"}, expectedName: "logging"},
			{data: events.MiddlewareFailedData{Name: "validation"}, expectedName: "validation"},
			{data: events.PipelineStartedData{}, expectedName: ""},
		}

		for _, tc := range testCases {
			evt := &events.Event{Data: tc.data}
			name := middlewareName(evt)
			if name != tc.expectedName {
				t.Errorf("expected middleware name '%s', got '%s'", tc.expectedName, name)
			}
		}
	})

	t.Run("toolName", func(t *testing.T) {
		testCases := []struct {
			data         events.EventData
			expectedName string
		}{
			{data: events.ToolCallStartedData{ToolName: "calculator"}, expectedName: "calculator"},
			{data: events.ToolCallCompletedData{ToolName: "search"}, expectedName: "search"},
			{data: events.ToolCallFailedData{ToolName: "database"}, expectedName: "database"},
			{data: events.PipelineStartedData{}, expectedName: ""},
		}

		for _, tc := range testCases {
			evt := &events.Event{Data: tc.data}
			name := toolName(evt)
			if name != tc.expectedName {
				t.Errorf("expected tool name '%s', got '%s'", tc.expectedName, name)
			}
		}
	})

	t.Run("validationName", func(t *testing.T) {
		testCases := []struct {
			data         events.EventData
			expectedName string
		}{
			{data: events.ValidationStartedData{ValidatorName: "schema"}, expectedName: "schema"},
			{data: events.ValidationPassedData{ValidatorName: "format"}, expectedName: "format"},
			{data: events.ValidationFailedData{ValidatorName: "content"}, expectedName: "content"},
			{data: events.PipelineStartedData{}, expectedName: ""},
		}

		for _, tc := range testCases {
			evt := &events.Event{Data: tc.data}
			name := validationName(evt)
			if name != tc.expectedName {
				t.Errorf("expected validation name '%s', got '%s'", tc.expectedName, name)
			}
		}
	})
}

// Tests for handleMessageCreated
func TestEventAdapter_HandleMessageCreated(t *testing.T) {
	t.Parallel()

	adapter := NewEventAdapter(nil)

	// Regression test for the silent failure where handleMessageCreated
	// type-asserted to value `events.MessageCreatedData`, but the
	// production emitter (events/emitter.go: MessageCreated) emits
	// `*events.MessageCreatedData` (pointer). The assertion failed,
	// returned nil, and the TUI conversation panel never updated mid-run.
	// This test uses the pointer form that production actually sends.
	t.Run("pointer data (production shape)", func(t *testing.T) {
		evt := &events.Event{
			Type:           events.EventMessageCreated,
			ConversationID: "conv-prod",
			Timestamp:      time.Now(),
			Data: &events.MessageCreatedData{
				Role:    "user",
				Content: "What's my schedule today?",
				Index:   2,
			},
		}
		msg := adapter.handleMessageCreated(evt)
		require.NotNil(t, msg, "pointer-shaped Data must produce a MessageCreatedMsg")
		got, ok := msg.(MessageCreatedMsg)
		require.True(t, ok)
		assert.Equal(t, "user", got.Role)
		assert.Equal(t, "What's my schedule today?", got.Content)
		assert.Equal(t, 2, got.Index)
	})

	t.Run("basic message", func(t *testing.T) {
		evt := &events.Event{
			Type:           events.EventMessageCreated,
			ConversationID: "conv-1",
			Timestamp:      time.Now(),
			Data: events.MessageCreatedData{
				Role:    "assistant",
				Content: "Hello, how can I help?",
				Index:   0,
			},
		}

		msg := adapter.handleMessageCreated(evt)
		require.NotNil(t, msg)

		createdMsg, ok := msg.(MessageCreatedMsg)
		require.True(t, ok)
		assert.Equal(t, "conv-1", createdMsg.ConversationID)
		assert.Equal(t, "assistant", createdMsg.Role)
		assert.Equal(t, "Hello, how can I help?", createdMsg.Content)
		assert.Equal(t, 0, createdMsg.Index)
	})

	t.Run("with reasoning", func(t *testing.T) {
		evt := &events.Event{
			Type:           events.EventMessageCreated,
			ConversationID: "conv-1",
			Timestamp:      time.Now(),
			Data: events.MessageCreatedData{
				Role:      "assistant",
				Content:   "ANSWER: 16",
				Index:     0,
				Reasoning: &types.ReasoningTrace{Text: "chain of thought"},
			},
		}

		msg := adapter.handleMessageCreated(evt)
		require.NotNil(t, msg)
		createdMsg, ok := msg.(MessageCreatedMsg)
		require.True(t, ok)
		require.NotNil(t, createdMsg.Reasoning, "reasoning must survive the event->msg seam")
		assert.Equal(t, "chain of thought", createdMsg.Reasoning.Text)
	})

	t.Run("with tool calls", func(t *testing.T) {
		evt := &events.Event{
			Type:           events.EventMessageCreated,
			ConversationID: "conv-1",
			Timestamp:      time.Now(),
			Data: events.MessageCreatedData{
				Role:    "assistant",
				Content: "",
				Index:   1,
				ToolCalls: []events.MessageToolCall{
					{ID: "call-1", Name: "get_weather", Args: `{"city": "NYC"}`},
					{ID: "call-2", Name: "get_time", Args: `{"zone": "EST"}`},
				},
			},
		}

		msg := adapter.handleMessageCreated(evt)
		require.NotNil(t, msg)

		createdMsg, ok := msg.(MessageCreatedMsg)
		require.True(t, ok)
		require.Len(t, createdMsg.ToolCalls, 2)
		assert.Equal(t, "get_weather", createdMsg.ToolCalls[0].Name)
		assert.Equal(t, "get_time", createdMsg.ToolCalls[1].Name)
	})

	t.Run("with tool result", func(t *testing.T) {
		evt := &events.Event{
			Type:           events.EventMessageCreated,
			ConversationID: "conv-1",
			Timestamp:      time.Now(),
			Data: events.MessageCreatedData{
				Role:    "tool",
				Content: "",
				Index:   2,
				ToolResult: &events.MessageToolResult{
					ID:        "call-1",
					Name:      "get_weather",
					Parts:     []types.ContentPart{types.NewTextPart(`{"temp": 72, "conditions": "sunny"}`)},
					LatencyMs: 150,
				},
			},
		}

		msg := adapter.handleMessageCreated(evt)
		require.NotNil(t, msg)

		createdMsg, ok := msg.(MessageCreatedMsg)
		require.True(t, ok)
		require.NotNil(t, createdMsg.ToolResult)
		assert.Equal(t, "get_weather", createdMsg.ToolResult.Name)
		assert.Equal(t, int64(150), createdMsg.ToolResult.LatencyMs)
	})

	t.Run("with tool result error", func(t *testing.T) {
		evt := &events.Event{
			Type:           events.EventMessageCreated,
			ConversationID: "conv-1",
			Timestamp:      time.Now(),
			Data: events.MessageCreatedData{
				Role:    "tool",
				Content: "",
				Index:   2,
				ToolResult: &events.MessageToolResult{
					ID:    "call-1",
					Name:  "get_weather",
					Error: "API rate limit exceeded",
				},
			},
		}

		msg := adapter.handleMessageCreated(evt)
		require.NotNil(t, msg)

		createdMsg, ok := msg.(MessageCreatedMsg)
		require.True(t, ok)
		require.NotNil(t, createdMsg.ToolResult)
		assert.Equal(t, "API rate limit exceeded", createdMsg.ToolResult.Error)
	})

	t.Run("wrong data type returns nil", func(t *testing.T) {
		evt := &events.Event{
			Type:           events.EventMessageCreated,
			ConversationID: "conv-1",
			Timestamp:      time.Now(),
			Data:           events.PipelineStartedData{}, // Wrong type
		}

		msg := adapter.handleMessageCreated(evt)
		assert.Nil(t, msg)
	})
}

// Tests for handleMessageUpdated
func TestEventAdapter_HandleMessageUpdated(t *testing.T) {
	t.Parallel()

	adapter := NewEventAdapter(nil)

	t.Run("basic update", func(t *testing.T) {
		evt := &events.Event{
			Type:           events.EventMessageUpdated,
			ConversationID: "conv-1",
			Timestamp:      time.Now(),
			Data: events.MessageUpdatedData{
				Index:        0,
				LatencyMs:    500,
				InputTokens:  100,
				OutputTokens: 50,
				TotalCost:    0.015,
			},
		}

		msg := adapter.handleMessageUpdated(evt)
		require.NotNil(t, msg)

		updatedMsg, ok := msg.(MessageUpdatedMsg)
		require.True(t, ok)
		assert.Equal(t, "conv-1", updatedMsg.ConversationID)
		assert.Equal(t, 0, updatedMsg.Index)
		assert.Equal(t, int64(500), updatedMsg.LatencyMs)
		assert.Equal(t, 100, updatedMsg.InputTokens)
		assert.Equal(t, 50, updatedMsg.OutputTokens)
		assert.Equal(t, 0.015, updatedMsg.TotalCost)
	})

	t.Run("wrong data type returns nil", func(t *testing.T) {
		evt := &events.Event{
			Type:           events.EventMessageUpdated,
			ConversationID: "conv-1",
			Timestamp:      time.Now(),
			Data:           events.PipelineStartedData{}, // Wrong type
		}

		msg := adapter.handleMessageUpdated(evt)
		assert.Nil(t, msg)
	})
}

// Tests for handleConversationStarted
func TestEventAdapter_HandleConversationStarted(t *testing.T) {
	t.Parallel()

	adapter := NewEventAdapter(nil)

	t.Run("basic conversation started", func(t *testing.T) {
		evt := &events.Event{
			Type:           events.EventConversationStarted,
			ConversationID: "conv-1",
			Timestamp:      time.Now(),
			Data: events.ConversationStartedData{
				SystemPrompt: "You are a helpful AI assistant.",
			},
		}

		msg := adapter.handleConversationStarted(evt)
		require.NotNil(t, msg)

		startedMsg, ok := msg.(ConversationStartedMsg)
		require.True(t, ok)
		assert.Equal(t, "conv-1", startedMsg.ConversationID)
		assert.Equal(t, "You are a helpful AI assistant.", startedMsg.SystemPrompt)
	})

	t.Run("empty system prompt", func(t *testing.T) {
		evt := &events.Event{
			Type:           events.EventConversationStarted,
			ConversationID: "conv-2",
			Timestamp:      time.Now(),
			Data: events.ConversationStartedData{
				SystemPrompt: "",
			},
		}

		msg := adapter.handleConversationStarted(evt)
		require.NotNil(t, msg)

		startedMsg, ok := msg.(ConversationStartedMsg)
		require.True(t, ok)
		assert.Equal(t, "", startedMsg.SystemPrompt)
	})

	t.Run("wrong data type returns nil", func(t *testing.T) {
		evt := &events.Event{
			Type:           events.EventConversationStarted,
			ConversationID: "conv-1",
			Timestamp:      time.Now(),
			Data:           events.PipelineStartedData{}, // Wrong type
		}

		msg := adapter.handleConversationStarted(evt)
		assert.Nil(t, msg)
	})
}

// Tests for mapEvent with new event types
func TestEventAdapter_MapEvent_NewEventTypes(t *testing.T) {
	t.Parallel()

	adapter := NewEventAdapter(nil)

	t.Run("message created event", func(t *testing.T) {
		evt := &events.Event{
			Type:           events.EventMessageCreated,
			ConversationID: "conv-1",
			Timestamp:      time.Now(),
			Data: events.MessageCreatedData{
				Role:    "assistant",
				Content: "Hello",
				Index:   0,
			},
		}

		msg := adapter.mapEvent(evt)
		require.NotNil(t, msg)
		_, ok := msg.(MessageCreatedMsg)
		assert.True(t, ok)
	})

	t.Run("message updated event", func(t *testing.T) {
		evt := &events.Event{
			Type:           events.EventMessageUpdated,
			ConversationID: "conv-1",
			Timestamp:      time.Now(),
			Data: events.MessageUpdatedData{
				Index:        0,
				LatencyMs:    100,
				InputTokens:  50,
				OutputTokens: 25,
				TotalCost:    0.01,
			},
		}

		msg := adapter.mapEvent(evt)
		require.NotNil(t, msg)
		_, ok := msg.(MessageUpdatedMsg)
		assert.True(t, ok)
	})

	t.Run("conversation started event", func(t *testing.T) {
		evt := &events.Event{
			Type:           events.EventConversationStarted,
			ConversationID: "conv-1",
			Timestamp:      time.Now(),
			Data: events.ConversationStartedData{
				SystemPrompt: "You are helpful.",
			},
		}

		msg := adapter.mapEvent(evt)
		require.NotNil(t, msg)
		_, ok := msg.(ConversationStartedMsg)
		assert.True(t, ok)
	})
}

// TestEventAdapter_ReasoningDelta verifies the events-bus reasoning signal maps
// to a non-content ReasoningDeltaMsg for live display.
func TestEventAdapter_ReasoningDelta(t *testing.T) {
	t.Parallel()
	adapter := NewEventAdapter(nil)
	msg := adapter.mapEvent(&events.Event{
		Type:      events.EventReasoningDelta,
		Timestamp: time.Now(),
		Data:      &events.ReasoningDeltaData{Text: "thinking out loud"},
	})
	rd, ok := msg.(ReasoningDeltaMsg)
	if !ok {
		t.Fatalf("expected ReasoningDeltaMsg, got %T", msg)
	}
	if rd.Text != "thinking out loud" {
		t.Fatalf("Text = %q, want %q", rd.Text, "thinking out loud")
	}
}

// TestEventAdapter_MessageCreated_TextInParts pins the other half of the
// payload contract.
//
// A message's text is in Content for an assistant turn and in Parts for a user
// turn — the runtime's own live testing found user turns arriving with Content
// empty. Reading .Content directly therefore renders every Parts-carrying
// message as an empty bubble, which is what the conversation panel was doing.
// MessageCreatedData.GetContent is the canonical rule for which field wins.
func TestEventAdapter_MessageCreated_TextInParts(t *testing.T) {
	t.Parallel()

	text := "where is my order"
	adapter := NewEventAdapter(nil)
	msg := adapter.handleMessageCreated(&events.Event{
		Type:           events.EventMessageCreated,
		ConversationID: "conv-parts",
		Timestamp:      time.Now(),
		Data: &events.MessageCreatedData{
			Role:  "user",
			Parts: []types.ContentPart{{Type: "text", Text: &text}},
		},
	})

	require.NotNil(t, msg)
	got, ok := msg.(MessageCreatedMsg)
	require.True(t, ok)
	assert.Equal(t, text, got.Content,
		"the user's text was dropped: the panel renders an empty bubble for any "+
			"message that carries its text in Parts")
}

func TestEventAdapter_RunStartedCarriesDataRow(t *testing.T) {
	adapter := NewEventAdapterWithModel(NewModel("test.yaml", 1))
	msg := adapter.mapEvent(&events.Event{
		Type:        events.EventType("arena.run.started"),
		ExecutionID: "run-1",
		Data: &events.CustomEventData{Data: map[string]interface{}{
			"scenario": "capitals",
			"data_row": "fr",
		}},
	})
	started, ok := msg.(RunStartedMsg)
	if !ok {
		t.Fatalf("expected RunStartedMsg, got %T", msg)
	}
	if started.Scenario != "capitals" || started.DataRow != "fr" {
		t.Errorf("got scenario %q, row %q", started.Scenario, started.DataRow)
	}
}

func TestEventAdapter_TrialsSkipped(t *testing.T) {
	model := NewModel("cfg", 10)
	adapter := NewEventAdapterWithModel(model)
	adapter.HandleEvent(&events.Event{
		Type:      events.EventType("arena.trials.skipped"),
		Timestamp: time.Now(),
		Data:      events.CustomEventData{Data: map[string]interface{}{"count": 4}},
	})
	model.mu.Lock()
	defer model.mu.Unlock()
	if model.totalRuns != 6 {
		t.Errorf("totalRuns = %d, want 6 after 4 trials were skipped", model.totalRuns)
	}
}
