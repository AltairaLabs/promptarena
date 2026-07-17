package web

import (
	"encoding/json"
	"testing"
)

func TestVoiceStateMsg(t *testing.T) {
	got := voiceStateMsg(voiceStateSpeaking)
	var m map[string]string
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["type"] != "state" || m["state"] != "speaking" {
		t.Fatalf("unexpected frame: %s", got)
	}
}

func TestParseVoiceControlMute(t *testing.T) {
	c, err := parseVoiceControl([]byte(`{"type":"mute","muted":true}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.Type != "mute" || !c.Muted {
		t.Fatalf("unexpected control: %+v", c)
	}
}
