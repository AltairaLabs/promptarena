package web

import "encoding/json"

const (
	voiceStateLive      = "live"
	voiceStateListening = "listening"
	voiceStateSpeaking  = "speaking"
)

// voiceControl is a client→server text control frame.
type voiceControl struct {
	Type  string `json:"type"`
	Muted bool   `json:"muted"`
}

func voiceStateMsg(state string) []byte {
	b, _ := json.Marshal(map[string]string{"type": "state", "state": state})
	return b
}

func voiceFlushMsg() []byte {
	b, _ := json.Marshal(map[string]string{"type": "flush"})
	return b
}

func voiceErrorMsg(msg string) []byte {
	b, _ := json.Marshal(map[string]string{"type": "error", "message": msg})
	return b
}

func parseVoiceControl(data []byte) (voiceControl, error) {
	var c voiceControl
	err := json.Unmarshal(data, &c)
	return c, err
}
