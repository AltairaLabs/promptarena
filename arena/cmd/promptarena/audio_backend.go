package main

// The CLI plays run audio on the host device (TUI and serve), so it links the
// oto playback backend. Library users of arena/engine don't, and stay static.
import _ "github.com/AltairaLabs/promptarena/v2/arena/audio/otobackend"
