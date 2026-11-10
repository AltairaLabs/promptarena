// Package otobackend plays LocalSink audio on the host device through
// github.com/ebitengine/oto. Import it for its side effect in a binary that
// should play audio:
//
//	import _ "github.com/AltairaLabs/promptarena/v2/arena/audio/otobackend"
//
// It is a separate package because oto depends on purego, which makes a
// CGO_ENABLED=0 Linux binary dynamically linked against glibc. Binaries that
// only import arena/engine (headless workers) stay static. This file needs a
// real audio device to exercise, so it is excluded from coverage.
package otobackend

import (
	"io"

	"github.com/ebitengine/oto/v3"

	"github.com/AltairaLabs/promptarena/v2/arena/audio"
)

//nolint:gochecknoinits // registering on import is this package's whole purpose.
func init() {
	audio.RegisterPlaybackBackend(Open)
}

// Open opens the host audio device for a stereo s16le stream at rate.
func Open(rate, channels int) (audio.PlaybackContext, error) {
	options := &oto.NewContextOptions{
		SampleRate:   rate,
		ChannelCount: channels,
		Format:       oto.FormatSignedInt16LE,
	}
	ctx, ready, err := oto.NewContext(options)
	if err != nil {
		return nil, err
	}
	<-ready
	return contextAdapter{ctx: ctx}, nil
}

type contextAdapter struct {
	ctx *oto.Context
}

// NewPlayer creates an oto.Player that reads from reader.
func (a contextAdapter) NewPlayer(reader io.Reader) audio.PlaybackPlayer {
	return playerAdapter{p: a.ctx.NewPlayer(reader)}
}

type playerAdapter struct {
	p *oto.Player
}

// Play starts oto playback on the underlying player.
func (a playerAdapter) Play() { a.p.Play() }
