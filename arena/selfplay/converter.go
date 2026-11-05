package selfplay

import (
	"encoding/base64"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"strings"
	"unicode"
)

// converters are the built-in converters, by the name a transform's converter
// field gives. Each rewrites the whole message; seed drives the random ones.
var converters = map[string]func(msg string, seed uint64) string{
	"base64":       func(m string, _ uint64) string { return base64.StdEncoding.EncodeToString([]byte(m)) },
	"rot13":        func(m string, _ uint64) string { return strings.Map(rot13, m) },
	"leetspeak":    func(m string, _ uint64) string { return leetReplacer.Replace(m) },
	"reverse":      func(m string, _ uint64) string { return reverseRunes(m) },
	"homoglyph":    func(m string, _ uint64) string { return strings.Map(homoglyph, m) },
	"unicode_tags": func(m string, _ uint64) string { return strings.Map(unicodeTag, m) },
	"zero_width":   func(m string, _ uint64) string { return zeroWidth(m) },
	"best_of_n":    bestOfN,
}

// convert runs the named converter over msg.
func convert(name, msg string, seed uint64) (string, error) {
	c, ok := converters[name]
	if !ok {
		return "", fmt.Errorf("unknown converter %q", name)
	}
	return c(msg, seed), nil
}

// converterSeed seeds the random converters from the message and where it is
// in the run, so a run is repeatable, each trial gets its own variant, and
// dataset rows that send different messages get independent ones.
func converterSeed(req *TransformRequest, index int) uint64 {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%s\x00%s\x00%d\x00%d\x00%d\x00%d\x00%s",
		req.Scenario, req.Persona, req.Exchange, req.Trial, req.Seed, index, req.Message)
	if req.ToolCallID != "" {
		// Each call to a tool draws its own variant, even of the same result.
		_, _ = fmt.Fprintf(h, "\x00%s", req.ToolCallID)
	}
	return h.Sum64()
}

func rot13(r rune) rune {
	switch {
	case r >= 'a' && r <= 'z':
		return 'a' + (r-'a'+13)%26 //nolint:mnd // the rotation
	case r >= 'A' && r <= 'Z':
		return 'A' + (r-'A'+13)%26 //nolint:mnd // the rotation
	}
	return r
}

var leetReplacer = strings.NewReplacer(
	"a", "4", "A", "4", "e", "3", "E", "3", "i", "1", "I", "1",
	"o", "0", "O", "0", "s", "5", "S", "5", "t", "7", "T", "7",
)

func reverseRunes(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

// homoglyphs maps Latin letters to Cyrillic ones that look the same.
var homoglyphs = map[rune]rune{
	'a': '\u0430', 'c': '\u0441', 'e': '\u0435', 'i': '\u0456', 'j': '\u0458', 'o': '\u043e',
	'p': '\u0440', 's': '\u0455', 'x': '\u0445', 'y': '\u0443', 'A': '\u0410', 'B': '\u0412',
	'C': '\u0421', 'E': '\u0415', 'H': '\u041d', 'I': '\u0406', 'J': '\u0408', 'K': '\u041a',
	'M': '\u041c', 'O': '\u041e', 'P': '\u0420', 'S': '\u0405', 'T': '\u0422', 'X': '\u0425',
	'Y': '\u04ae',
}

func homoglyph(r rune) rune {
	if g, ok := homoglyphs[r]; ok {
		return g
	}
	return r
}

// unicodeTag moves printable ASCII into the Unicode tag block (U+E0020 to
// U+E007E), which renders as nothing but which a model may still read.
func unicodeTag(r rune) rune {
	if r >= ' ' && r <= '~' {
		return 0xE0000 + r //nolint:mnd // start of the tag block
	}
	return r
}

// zeroWidth puts a zero-width space between adjacent letters, so words split
// into odd tokens while digits, punctuation and combining marks stay intact.
func zeroWidth(s string) string {
	var b strings.Builder
	prev := ' '
	for _, r := range s {
		if unicode.IsLetter(prev) && unicode.IsLetter(r) {
			b.WriteRune('\u200b')
		}
		b.WriteRune(r)
		prev = r
	}
	return b.String()
}

// Best-of-N augmentation probabilities, from Hughes et al., "Best-of-N
// Jailbreaking" (arXiv 2412.03556).
const (
	bonScrambleP = 0.6
	bonCapsP     = 0.6
	bonNoiseP    = 0.06
)

// bestOfN applies Best-of-N's text augmentations: scramble the middle letters
// of longer words, flip the case of letters at random, and nudge printable
// ASCII characters by one code point.
func bestOfN(msg string, seed uint64) string {
	rng := rand.New(rand.NewPCG(seed, seed>>1|1)) //nolint:gosec // augmentation, not security
	out := []rune(msg)
	scrambleWords(out, rng)
	for i, r := range out {
		if unicode.IsLetter(r) && rng.Float64() < bonCapsP {
			r = flipCase(r)
		}
		// '!' and '~' are excluded so noise never makes a space or leaves ASCII.
		if r > '!' && r < '~' && rng.Float64() < bonNoiseP {
			r += rng.Int32N(2)*2 - 1 //nolint:mnd // one code point up or down
		}
		out[i] = r
	}
	return string(out)
}

// scrambleWords shuffles the middle letters of each run of letters in text,
// with Best-of-N's probability.
func scrambleWords(text []rune, rng *rand.Rand) {
	for start := 0; start < len(text); {
		if !unicode.IsLetter(text[start]) {
			start++
			continue
		}
		end := start
		for end < len(text) && unicode.IsLetter(text[end]) {
			end++
		}
		// A word keeps its first and last letter; one of 3 letters or fewer
		// has no middle worth scrambling.
		if end-start > 3 && rng.Float64() < bonScrambleP {
			mid := text[start+1 : end-1]
			rng.Shuffle(len(mid), func(a, b int) { mid[a], mid[b] = mid[b], mid[a] })
		}
		start = end
	}
}

func flipCase(r rune) rune {
	if unicode.IsUpper(r) {
		return unicode.ToLower(r)
	}
	return unicode.ToUpper(r)
}
