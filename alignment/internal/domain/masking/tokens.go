// Package masking holds the rules that keep an alignment input blind: the
// criteria are shown under opaque tokens in a permuted order, no payload names
// a criterion by its identifier, and the test is never shown under a name that
// reveals which criterion it was written for.
package masking

import (
	"cmp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/RachidChabane/musthold/alignment/internal/domain/criterion"
	"github.com/RachidChabane/musthold/format"
)

// Shape of the opaque tokens.
const (
	// TokenPrefix starts every criterion token.
	TokenPrefix = "K-"
	// tokenHexDigits is the number of hash digits a token carries.
	tokenHexDigits = 4
	// tokenRedrawLimit bounds the redraws of a colliding token.
	tokenRedrawLimit = 4096
	// NoneAnswer is the answer that names no criterion.
	NoneAnswer = "NONE"
	// Redacted replaces a criterion reference found in a text.
	Redacted = "the requirement"

	fieldSeparator = "\x1f"
	orderField     = "order"
	statementJoin  = "\n"
)

// Tokenized is the criteria as a judge sees them: statements under opaque tokens,
// in an order that depends on the test only.
type Tokenized struct {
	// Text lists `token: statement` lines in the permuted order.
	Text string
	// Tokens lists the tokens in the order of Text.
	Tokens []string
	// Statements maps a token to the statement shown under it.
	Statements map[string]string
	// Refs maps a token back to its criterion; it is never shown.
	Refs map[string]criterion.Ref
}

// Tokenize shows every criterion under a token drawn from the seed, which names
// the test. Tokens and order are a pure function of the seed and the criteria, so
// identical work gets an identical request.
func Tokenize(criteria []criterion.Criterion, seed string) Tokenized {
	refs := make([]criterion.Ref, len(criteria))
	for index, c := range criteria {
		refs[index] = c.Ref
	}
	tokenOf := drawTokens(refs, seed)
	ordered := slices.Clone(criteria)
	sort.SliceStable(ordered, func(i, j int) bool {
		return cmp.Or(
			strings.Compare(hashOf(seed, orderField, ordered[i].Ref.String()), hashOf(seed, orderField, ordered[j].Ref.String())),
			ordered[i].Ref.Compare(ordered[j].Ref),
		) < 0
	})
	result := Tokenized{Refs: make(map[string]criterion.Ref, len(criteria)), Statements: make(map[string]string, len(criteria))}
	lines := make([]string, len(ordered))
	for index, c := range ordered {
		token := tokenOf[c.Ref]
		statement := Scrub(c.Statement, refs)
		lines[index] = token + ": " + statement
		result.Statements[token] = statement
		result.Tokens = append(result.Tokens, token)
		result.Refs[token] = c.Ref
	}
	result.Text = strings.Join(lines, statementJoin)
	return result
}

func hashOf(fields ...string) string {
	return format.SHA256Hex([]byte(strings.Join(fields, fieldSeparator)))
}

// drawTokens gives each criterion the first hex digits of the hash of its
// reference, redrawn until every token is unique.
func drawTokens(refs []criterion.Ref, seed string) map[criterion.Ref]string {
	ordered := slices.Clone(refs)
	slices.SortFunc(ordered, criterion.Ref.Compare)
	taken := map[string]bool{}
	tokens := make(map[criterion.Ref]string, len(refs))
	for _, ref := range ordered {
		for attempt := range tokenRedrawLimit {
			fields := []string{seed, ref.String()}
			if attempt > 0 {
				fields = append(fields, strconv.Itoa(attempt))
			}
			token := TokenPrefix + hashOf(fields...)[:tokenHexDigits]
			if !taken[token] {
				taken[token] = true
				tokens[ref] = token
				break
			}
		}
	}
	return tokens
}

// Scrub removes every declared criterion reference, `ID~REV` and bare `ID`, from
// a text, so that a payload never names the criterion it is about.
func Scrub(text string, refs []criterion.Ref) string {
	ordered := slices.Clone(refs)
	sort.Slice(ordered, func(i, j int) bool { return len(ordered[i].ID) > len(ordered[j].ID) })
	for _, ref := range ordered {
		text = strings.ReplaceAll(text, ref.String(), Redacted)
		text = strings.ReplaceAll(text, ref.ID, Redacted)
	}
	return text
}
