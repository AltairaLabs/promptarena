package audio

import _ "embed"

// Built-in role-labeled mock audio. These short clips ("This is a mock user
// turn." / "This is a mock assistant turn.", in two distinct voices) are the
// default audio for mock duplex runs — so a keyless/CI demo produces clear,
// self-documenting sound you can follow by ear, instead of canned clips that
// don't match the conversation. Raw s16le / 24 kHz / mono, matching the format
// the audio path expects. Regenerate with scripts/gen-mock-clips.sh.
//
//go:embed assets/mock-user-turn.pcm
var mockUserTurnPCM []byte

//go:embed assets/mock-assistant-turn.pcm
var mockAssistantTurnPCM []byte

// MockUserTurnPCM returns the built-in "mock user turn" clip as raw s16le PCM
// at MockClipSampleRate. The returned slice must not be mutated.
func MockUserTurnPCM() []byte { return mockUserTurnPCM }

// MockAssistantTurnPCM returns the built-in "mock assistant turn" clip as raw
// s16le PCM at MockClipSampleRate. The returned slice must not be mutated.
func MockAssistantTurnPCM() []byte { return mockAssistantTurnPCM }

// MockClipSampleRate is the sample rate of the built-in mock clips.
const MockClipSampleRate = Rate24k
