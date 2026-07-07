package audio

import "testing"

func TestMockClips_EmbeddedAndWellFormed(t *testing.T) {
	for name, clip := range map[string][]byte{
		"user":      MockUserTurnPCM(),
		"assistant": MockAssistantTurnPCM(),
	} {
		if len(clip) == 0 {
			t.Errorf("%s clip is empty", name)
		}
		if len(clip)%2 != 0 {
			t.Errorf("%s clip has odd byte count %d; not valid s16le", name, len(clip))
		}
	}
	if MockClipSampleRate != Rate24k {
		t.Errorf("MockClipSampleRate = %d, want %d", MockClipSampleRate, Rate24k)
	}
}
