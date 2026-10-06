package operator

import (
	"strconv"
	"strings"
)

const (
	suffixAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	suffixLength   = 2
	labelSeparator = "\x00"
)

// neutralWords are the words injected identifiers are made of; they read like
// the scratch names of ordinary code.
var neutralWords = []string{
	"anchor", "backlog", "buffer", "carry", "cursor", "margin", "offset", "pending",
	"pivot", "probe", "scratch", "slot", "spare", "stash", "staging", "window",
}

// NameStyle writes a word and a suffix in the naming convention of a language.
type NameStyle func(word, suffix string) string

// NeutralName draws an identifier for an injected sentinel from the seed and the
// label of what it names. The identifier does not appear in taken, the text of
// the file, and carries no fixed marker: another seed or another label gives
// another identifier.
func NeutralName(seed Seed, label, taken string, style NameStyle) string {
	for attempt := 0; ; attempt++ {
		rng := streamFor(seed, label+labelSeparator+strconv.Itoa(attempt))
		var suffix strings.Builder
		word := neutralWords[rng.intn(len(neutralWords))]
		for range suffixLength {
			suffix.WriteByte(suffixAlphabet[rng.intn(len(suffixAlphabet))])
		}
		if name := style(word, suffix.String()); !strings.Contains(taken, name) {
			return name
		}
	}
}
