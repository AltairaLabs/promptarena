#!/usr/bin/env bash
# Regenerate the built-in role-labeled mock audio clips embedded by arena/audio.
# Produces raw s16le / 24 kHz / mono PCM — the format the arena audio path
# expects. Requires macOS `say` and `ffmpeg`.
set -euo pipefail

here="$(cd "$(dirname "$0")/.." && pwd)"
out="$here/arena/audio/assets"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# voice, text, output-basename
gen() {
  local voice="$1" text="$2" name="$3"
  say -v "$voice" "$text" -o "$tmp/$name.aiff"
  ffmpeg -y -loglevel error -i "$tmp/$name.aiff" -ar 24000 -ac 1 -f s16le "$out/$name.pcm"
  echo "wrote $out/$name.pcm ($(wc -c <"$out/$name.pcm") bytes)"
}

mkdir -p "$out"
gen Samantha "This is a mock user turn."      mock-user-turn
gen Daniel   "This is a mock assistant turn." mock-assistant-turn
