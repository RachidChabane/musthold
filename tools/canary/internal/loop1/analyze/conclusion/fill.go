package conclusion

import (
	"regexp"
	"strings"
)

// placeholder is a slot of a template, such as <k_t>.
var placeholder = regexp.MustCompile(`<[a-z_A-Z]+>`)

// Templates lists every conclusion template.
func Templates() []string {
	return []string{H1Supported, H1RejectionOnly, H1Contradicted, H1Inconclusive, H2}
}

// Fill replaces each placeholder of the template by its value, the name given
// without its angle brackets. A placeholder without a value stays as it is.
func Fill(template string, values map[string]string) string {
	return placeholder.ReplaceAllStringFunc(template, func(slot string) string {
		if value, found := values[strings.Trim(slot, "<>")]; found {
			return value
		}
		return slot
	})
}

// Pattern matches any filling of the template: its text verbatim, each
// placeholder standing for any text.
func Pattern(template string) *regexp.Regexp {
	var pattern strings.Builder
	last := 0
	for _, slot := range placeholder.FindAllStringIndex(template, -1) {
		pattern.WriteString(regexp.QuoteMeta(template[last:slot[0]]))
		pattern.WriteString(`.+?`)
		last = slot[1]
	}
	pattern.WriteString(regexp.QuoteMeta(template[last:]))
	return regexp.MustCompile(pattern.String())
}
