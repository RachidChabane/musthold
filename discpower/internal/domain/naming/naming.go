// Package naming holds the spelling rules that tie the names a spec uses for the
// parameters of an operation to the identifiers the code declares them by.
package naming

import (
	"strings"
	"unicode"
)

const wordSeparator = "_"

// SnakeCase spells an identifier in lower case with its words joined by
// underscores: `unitPrice` and `UnitPrice` become `unit_price`, `parseURLPath`
// becomes `parse_url_path`, and an identifier already in snake case is kept.
func SnakeCase(name string) string {
	runes := []rune(name)
	var builder strings.Builder
	for index, current := range runes {
		if index > 0 && unicode.IsUpper(current) && startsWord(runes, index) {
			builder.WriteString(wordSeparator)
		}
		builder.WriteRune(unicode.ToLower(current))
	}
	return builder.String()
}

// startsWord tells whether the upper-case rune at index opens a new word: it
// follows a lower-case letter or a digit, or ends a run of capitals before a
// lower-case letter.
func startsWord(runes []rune, index int) bool {
	previous := runes[index-1]
	if unicode.IsLower(previous) || unicode.IsDigit(previous) {
		return true
	}
	return unicode.IsUpper(previous) && index+1 < len(runes) && unicode.IsLower(runes[index+1])
}
