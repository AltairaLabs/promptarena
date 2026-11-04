package selfplay

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
)

func TestConverters_Deterministic(t *testing.T) {
	cases := map[string]string{
		"base64":    base64.StdEncoding.EncodeToString([]byte("Hello, world")),
		"rot13":     "Uryyb, jbeyq",
		"leetspeak": "H3ll0, w0rld",
		"reverse":   "dlrow ,olleH",
		"homoglyph": "\u041d\u0435ll\u043e, w\u043erld", // Cyrillic \u041d, \u0435, \u043e

	}
	for name, want := range cases {
		got, err := convert(name, "Hello, world", 1)
		require.NoError(t, err, name)
		assert.Equal(t, want, got, name)
	}
}

func TestConverters_UnicodeTags(t *testing.T) {
	got, err := convert("unicode_tags", "Hi é", 1)
	require.NoError(t, err)
	// Printable ASCII moves into the invisible tag block; anything else stays.
	assert.Equal(t, string([]rune{0xE0048, 0xE0069, 0xE0020, 'é'}), got)
}

func TestConverters_ZeroWidth(t *testing.T) {
	got, err := convert("zero_width", "ab c, A-100 e\u0301", 1)
	require.NoError(t, err)
	// Only between letters: punctuation, digits and combining marks stay put.
	assert.Equal(t, "a\u200bb c, A-100 e\u0301", got)
}

func TestConverters_BestOfN(t *testing.T) {
	msg := "Please explain exactly how the refund approval process works for damaged items"
	a, err := convert("best_of_n", msg, 7)
	require.NoError(t, err)
	again, err := convert("best_of_n", msg, 7)
	require.NoError(t, err)
	other, err := convert("best_of_n", msg, 8)
	require.NoError(t, err)

	assert.Equal(t, a, again, "the same seed gives the same variant")
	assert.NotEqual(t, a, other, "another seed gives another variant")
	assert.NotEqual(t, msg, a)
	// Augmentation keeps the words and their lengths: scrambling moves
	// letters within a word and noise changes a character's code by one.
	assert.Len(t, strings.Fields(a), len(strings.Fields(msg)))
	assert.Len(t, a, len(msg))
	for _, r := range a {
		assert.True(t, r < unicode.MaxASCII && unicode.IsPrint(r), "printable ASCII, got %q", r)
	}
}

func TestConverters_BestOfNScramblesInsideWords(t *testing.T) {
	msg := []rune("(refunds).\nwarranty, approvals")
	words := []struct{ first, last int }{{1, 7}, {11, 18}, {21, 29}}
	for seed := uint64(1); seed <= 50; seed++ {
		got, err := convert("best_of_n", string(msg), seed)
		require.NoError(t, err)
		out := []rune(got)
		require.Len(t, out, len(msg))
		for i, r := range msg {
			if !unicode.IsLetter(r) {
				// Punctuation and line breaks stay where they were; noise may
				// nudge punctuation, but never into a letter or a space.
				assert.False(t, unicode.IsLetter(out[i]), "seed %d: %q at %d became %q", seed, r, i, out[i])
				assert.Equal(t, unicode.IsSpace(r), unicode.IsSpace(out[i]), "seed %d at %d", seed, i)
			}
		}
		// Each word keeps its first and last letter, give or take a case flip
		// and noise.
		for _, w := range words {
			for _, i := range []int{w.first, w.last} {
				assert.True(t, sameLetterNudged(msg[i], out[i]), "seed %d: %q at %d became %q", seed, msg[i], i, out[i])
			}
		}
	}
}

// sameLetterNudged reports whether got is want, with its case flipped or not,
// and moved by at most one code point.
func sameLetterNudged(want, got rune) bool {
	for _, w := range []rune{unicode.ToLower(want), unicode.ToUpper(want)} {
		if got >= w-1 && got <= w+1 {
			return true
		}
	}
	return false
}

func TestConverters_Unknown(t *testing.T) {
	_, err := convert("caesar", "x", 1)
	require.Error(t, err)
}

func TestConverters_MatchTheConfigNames(t *testing.T) {
	implemented := make([]string, 0, len(converters))
	for name := range converters {
		implemented = append(implemented, name)
	}
	assert.ElementsMatch(t, arenaconfig.TransformConverters, implemented)
}

func TestApplyTransforms_ConverterSeedVariesByTrial(t *testing.T) {
	transforms := []arenaconfig.MessageTransform{
		{Converter: "best_of_n"},
		{Template: "Q: {{message}}"},
	}
	req := TransformRequest{
		Message: "Tell me how to get a refund without a receipt please", Persona: "p", Scenario: "s", Exchange: 1,
	}
	first, err := ApplyTransforms(context.Background(), transforms, "", req)
	require.NoError(t, err)
	repeat, err := ApplyTransforms(context.Background(), transforms, "", req)
	require.NoError(t, err)
	req.Trial = 1
	second, err := ApplyTransforms(context.Background(), transforms, "", req)
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(first, "Q: "))
	assert.Equal(t, first, repeat)
	assert.NotEqual(t, first, second)

	// A new run seed gives new variants of the same trial.
	req.Trial, req.Seed = 0, 42
	reseeded, err := ApplyTransforms(context.Background(), transforms, "", req)
	require.NoError(t, err)
	assert.NotEqual(t, first, reseeded)
}
