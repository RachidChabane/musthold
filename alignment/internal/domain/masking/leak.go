package masking

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/RachidChabane/musthold/alignment/internal/domain/criterion"
)

// Leak rules.
const (
	// ForbiddenRunLength is the number of consecutive words of the test name that
	// may not appear in a masked item.
	ForbiddenRunLength = 3
	// MarkerWord is the word that introduces a criterion citation in a test.
	MarkerWord = "verifies"
	// testPrefixWord is dropped from a test name before its words are compared.
	testPrefixWord = "test"
)

// ErrLeak is returned for a masked input that reveals which criterion its test
// was written for.
var ErrLeak = errors.New("the masked input leaks")

var wordPattern = regexp.MustCompile(`[A-Za-z]+|[0-9]+`)

// LeakKind names the way an item leaks.
type LeakKind string

// Kinds of leak.
const (
	LeakReference LeakKind = "criterion_reference"
	LeakMarker    LeakKind = "marker_word"
	LeakNameRun   LeakKind = "test_name_run"
)

// Leak is one finding of a leak check.
type Leak struct {
	Kind   LeakKind
	Detail string
}

func (l Leak) String() string { return string(l.Kind) + ": " + l.Detail }

// words splits text into lower-case words: runs of letters or digits, with a
// camel-case run cut where a lower-case letter meets an upper-case one.
func words(text string) []string {
	var found []string
	for _, chunk := range wordPattern.FindAllString(text, -1) {
		found = append(found, splitCamel(chunk)...)
	}
	return found
}

func splitCamel(chunk string) []string {
	var parts []string
	var current []rune
	runes := []rune(chunk)
	for index, r := range runes {
		startsWord := index > 0 && unicode.IsUpper(r) &&
			(unicode.IsLower(runes[index-1]) || (index+1 < len(runes) && unicode.IsLower(runes[index+1]) && unicode.IsUpper(runes[index-1])))
		if startsWord && len(current) > 0 {
			parts = append(parts, strings.ToLower(string(current)))
			current = nil
		}
		current = append(current, r)
	}
	if len(current) > 0 {
		parts = append(parts, strings.ToLower(string(current)))
	}
	return parts
}

// parameterOpen starts the parametrization suffix of a test name, as in
// `test_total[10-200-180.0]`.
const parameterOpen = "["

// NameWithoutParameters is the name its author gave a test: a parametrized run is
// reported with the ids of its parameters appended in brackets, which are data the
// test ran with and not part of its name.
func NameWithoutParameters(testName string) string {
	name, _, _ := strings.Cut(testName, parameterOpen)
	return name
}

// nameWords are the words of a test name without its parametrization suffix and
// without the `test` prefix word.
func nameWords(testName string) []string {
	var kept []string
	for _, word := range words(NameWithoutParameters(testName)) {
		if word != testPrefixWord {
			kept = append(kept, word)
		}
	}
	return kept
}

// NameRuns lists the runs of ForbiddenRunLength or more consecutive words of the
// test name found in the item.
func NameRuns(item, testName string) []string {
	name, itemWords := nameWords(testName), words(item)
	var found []string
	for start := 0; start+ForbiddenRunLength <= len(name); start++ {
		run := name[start : start+ForbiddenRunLength]
		if containsRun(itemWords, run) {
			found = append(found, strings.Join(run, " "))
		}
	}
	return found
}

func containsRun(haystack, run []string) bool {
	for start := 0; start+len(run) <= len(haystack); start++ {
		if equalWords(haystack[start:start+len(run)], run) {
			return true
		}
	}
	return false
}

func equalWords(a, b []string) bool {
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

// Check lists the ways an item that a judge sees, apart from the criteria
// statements, reveals the declared criterion: a criterion reference or ID, the
// marker word, or a run of words of the test name. The statements are exempt
// from the name check, because they are the same for every test.
func Check(item, testName string, refs []criterion.Ref) []Leak {
	leaks := CheckText(item, refs)
	for _, run := range NameRuns(item, testName) {
		leaks = append(leaks, Leak{Kind: LeakNameRun, Detail: run})
	}
	return leaks
}

// CheckText lists the criterion references and marker words a text holds.
func CheckText(text string, refs []criterion.Ref) []Leak {
	var leaks []Leak
	for _, ref := range refs {
		if strings.Contains(text, ref.ID) {
			leaks = append(leaks, Leak{Kind: LeakReference, Detail: ref.ID})
		}
	}
	if strings.Contains(strings.ToLower(text), MarkerWord) {
		leaks = append(leaks, Leak{Kind: LeakMarker, Detail: MarkerWord})
	}
	return leaks
}

// Ensure returns ErrLeak, listing every leak, unless the item is clean.
func Ensure(item, testName string, refs []criterion.Ref) error {
	leaks := Check(item, testName, refs)
	if len(leaks) == 0 {
		return nil
	}
	details := make([]string, len(leaks))
	for index, leak := range leaks {
		details[index] = leak.String()
	}
	return fmt.Errorf("%w: %s", ErrLeak, strings.Join(details, "; "))
}
