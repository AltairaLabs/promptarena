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
	b, _ := json.Marshal(map[string]string{jsonKeyType: jsonKeyState, jsonKeyState: state})
	return b
}

func voiceFlushMsg() []byte {
	b, _ := json.Marshal(map[string]string{jsonKeyType: "flush"})
	return b
}

func voiceErrorMsg(msg string) []byte {
	b, _ := json.Marshal(map[string]string{jsonKeyType: jsonKeyError, jsonKeyMessage: msg})
	return b
}

func parseVoiceControl(data []byte) (voiceControl, error) {
	var c voiceControl
	err := json.Unmarshal(data, &c)
	return c, err
}
