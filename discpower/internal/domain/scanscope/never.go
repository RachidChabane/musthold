package scanscope

import (
	"regexp"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

// pathSeparator separates the segments of a relative POSIX path.
const pathSeparator = "/"

// headerLines is how many first lines of a file a generated-code marker is
// looked for in.
const headerLines = 10

var (
	// thirdPartySegments are directories that hold code the project vendors or
	// installs rather than writes.
	thirdPartySegments = []string{"vendor", "third_party", "node_modules", "site-packages", ".venv", "venv"}
	// migrationSegments are directories that hold schema migrations.
	migrationSegments = []string{"migrations"}
	// testSegments are directories that hold tests and their helpers.
	testSegments = []string{"tests", "test", "testdata", "__tests__", "__mocks__", "__fixtures__"}
	// generatedMarker is the conventional header of generated code, Go's
	// `Code generated ... DO NOT EDIT.` and the `@generated` tag.
	generatedMarker = regexp.MustCompile(`(?m)(^\s*(//|#)\s*Code generated .* DO NOT EDIT\.?\s*$)|@generated\b`)
	// sinkCall matches a call whose effect is a log line, printed output or a
	// metric: changing its arguments changes nothing a test may assert on.
	sinkCall = regexp.MustCompile(`\b(print|pprint|fmt\.(Print|Println|Printf|Fprint|Fprintln|Fprintf)|log\.\w+|slog\.\w+|logging\.\w+|logger\.\w+|log\w*\.(debug|info|warning|warn|error|exception|critical)|warnings\.warn|console\.\w+|metrics\.\w+|statsd\.\w+)\s*\(`)
)

// Mutable reports whether a scan may mutate a file: a source file the project
// writes, never a test file or a file of a test directory, vendored or
// installed code, generated code or a migration.
func Mutable(path string, content []byte, layout ecosystem.Layout) bool {
	if layout.IsTestFile(path) {
		return false
	}
	segments := strings.Split(path, pathSeparator)
	directories := segments[:len(segments)-1]
	if slices.ContainsFunc(directories, func(segment string) bool {
		return slices.Contains(thirdPartySegments, segment) || slices.Contains(migrationSegments, segment) || slices.Contains(testSegments, segment)
	}) {
		return false
	}
	return !generatedMarker.Match(header(content))
}

// header is the first lines of the content.
func header(content []byte) []byte {
	end, seen := 0, 0
	for end < len(content) && seen < headerLines {
		if content[end] == lineBreak {
			seen++
		}
		end++
	}
	return content[:end]
}

// SinkLine reports whether a line of source calls a sink: a log, a print or a
// metric. A scan never mutates such a line.
func SinkLine(text string) bool {
	return sinkCall.MatchString(text)
}
