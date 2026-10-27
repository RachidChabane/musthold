package accept

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// The sentence every task prompt follows, with the four parts a task fills.
const (
	promptFormat      = "Implement requirement %s of %s, %s, in %s, and add a test that verifies it. Tell me when the task is done."
	promptPrefix      = "Implement requirement "
	promptSuffix      = ", and add a test that verifies it. Tell me when the task is done."
	promptSeparator   = ", in "
	promptOf          = " of "
	maxStatementWords = 12
)

// PromptTemplate is the prompt sentence with its four parts named.
const PromptTemplate = "Implement requirement <ID> of <spec path>, <statement in a few words>, in <source file>, and add a test that verifies it. Tell me when the task is done."

// Prompt builds the prompt of a task from the four parts the template names.
func Prompt(criterion, spec, statement, source string) string {
	return fmt.Sprintf(promptFormat, criterion, spec, statement, source)
}

// CheckPrompt reports how a prompt departs from the template, or nil when it
// follows it for the given criterion, spec and source file.
func CheckPrompt(prompt, criterion, spec, source string) error {
	head := promptPrefix + criterion + promptOf + spec + ", "
	tail := promptSeparator + source + promptSuffix
	if !strings.HasPrefix(prompt, head) || !strings.HasSuffix(prompt, tail) || len(prompt) < len(head)+len(tail) {
		return fmt.Errorf("the prompt is not %q", PromptTemplate)
	}
	statement := prompt[len(head) : len(prompt)-len(tail)]
	if strings.TrimSpace(statement) == "" || strings.ContainsAny(statement, "\r\n") {
		return fmt.Errorf("the statement of the prompt must be a few words on one line")
	}
	if words := len(strings.Fields(statement)); words > maxStatementWords {
		return fmt.Errorf("the statement of the prompt has %d words, at most %d are a few", words, maxStatementWords)
	}
	return nil
}

// OracleFileName is the test file that holds the tests of one criterion:
// test_<id>.py with the ID in lower case and its hyphen written as an underscore.
func OracleFileName(criterion string) string {
	return OracleFilePrefix + strings.ToLower(strings.ReplaceAll(criterion, "-", "_")) + pythonSuffix
}

// The names the oracle's files follow.
const (
	OracleFilePrefix = "test_"
	pythonSuffix     = ".py"
	// OracleConfigFile is the oracle's own pytest configuration.
	OracleConfigFile = "pytest.ini"
)

// Example is one inline example of a criterion, operation(arguments) => result.
type Example struct {
	Operation string
	Arguments string
	Result    string
}

// Argument is one keyword argument of an example.
type Argument struct {
	Name  string
	Value string
}

var (
	declarationPattern = regexp.MustCompile(`^\s*[-*]\s+\*\*([A-Z][A-Z0-9]*-[0-9]+)(?:~[0-9]+)?\*\*\s*:`)
	headingPattern     = regexp.MustCompile(`^\s{0,3}#{1,6}\s`)
	headingDeclaration = regexp.MustCompile(`^\s{0,3}#{1,6}\s+([A-Z][A-Z0-9]*-[0-9]+)(?:~[0-9]+)?\s*:`)
	examplePattern     = regexp.MustCompile(`^\s*[-*]\s+Example:\s*([A-Za-z_][A-Za-z0-9_]*)\((.*)\)\s*=>\s*(\S.*?)\s*$`)
	argumentPattern    = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(\S.*)$`)
	statementPattern   = regexp.MustCompile(`(?s)^WHEN\b.+\bTHE SYSTEM SHALL\b.+`)
)

// ParseExamples reads the inline examples of each criterion of a Markdown spec.
// A criterion's block runs from its declaration to the next declaration or
// heading; fenced code blocks hold no declaration or example.
func ParseExamples(spec []byte) map[string][]Example {
	examples := map[string][]Example{}
	current := ""
	fenced := false
	for _, line := range strings.Split(string(spec), "\n") {
		if isFence(line) {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		if match := declarationPattern.FindStringSubmatch(line); match != nil {
			current = match[1]
			examples[current] = nil
			continue
		}
		if match := headingDeclaration.FindStringSubmatch(line); match != nil {
			current = match[1]
			examples[current] = nil
			continue
		}
		if headingPattern.MatchString(line) {
			current = ""
			continue
		}
		if match := examplePattern.FindStringSubmatch(line); match != nil && current != "" {
			examples[current] = append(examples[current], Example{Operation: match[1], Arguments: match[2], Result: match[3]})
		}
	}
	return examples
}

func isFence(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
}

// IsRequirementStatement reports whether a statement is one WHEN … THE SYSTEM
// SHALL … sentence.
func IsRequirementStatement(statement string) bool {
	return statementPattern.MatchString(strings.TrimSpace(statement))
}

// SplitArguments reads the keyword arguments of an example, splitting at the
// commas outside brackets and quotes.
func SplitArguments(arguments string) ([]Argument, error) {
	parts, err := splitTopLevel(arguments)
	if err != nil {
		return nil, err
	}
	var found []Argument
	for _, part := range parts {
		match := argumentPattern.FindStringSubmatch(strings.TrimSpace(part))
		if match == nil {
			return nil, fmt.Errorf("%q is not a keyword argument", strings.TrimSpace(part))
		}
		found = append(found, Argument{Name: match[1], Value: strings.TrimSpace(match[2])})
	}
	return found, nil
}

func splitTopLevel(text string) ([]string, error) {
	var parts []string
	depth := 0
	var quote rune
	start := 0
	escaped := false
	for index, char := range text {
		switch {
		case escaped:
			escaped = false
		case quote != 0:
			if char == '\\' {
				escaped = true
			} else if char == quote {
				quote = 0
			}
		case char == '"' || char == '\'':
			quote = char
		case strings.ContainsRune("([{", char):
			depth++
		case strings.ContainsRune(")]}", char):
			depth--
		case char == ',' && depth == 0:
			parts = append(parts, text[start:index])
			start = index + 1
		}
	}
	if quote != 0 || depth != 0 {
		return nil, fmt.Errorf("the arguments %q are not balanced", text)
	}
	if strings.TrimSpace(text[start:]) != "" {
		parts = append(parts, text[start:])
	}
	return parts, nil
}

var citationPattern = regexp.MustCompile(`verifies\(\s*["']([A-Z][A-Z0-9]*-[0-9]+~[0-9]+)["']\s*\)`)

// Citations counts, per reference such as BOX-1~1, the `verifies` markers found
// in the sources of the tests.
func Citations(sources []string) map[string]int {
	counted := map[string]int{}
	for _, source := range sources {
		for _, match := range citationPattern.FindAllStringSubmatch(source, -1) {
			counted[match[1]]++
		}
	}
	return counted
}

// IniValues reads the keys of the [pytest] section of an ini file.
func IniValues(ini string) map[string]string {
	values := map[string]string{}
	inSection := false
	for _, raw := range strings.Split(ini, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";"):
		case strings.HasPrefix(line, "["):
			inSection = line == "[pytest]"
		case inSection:
			if key, value, found := strings.Cut(line, "="); found {
				values[strings.TrimSpace(key)] = strings.TrimSpace(value)
			}
		}
	}
	return values
}

// sortedKeys lists a map's keys in order.
func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
