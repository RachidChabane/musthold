package accept

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const maxMessageRunes = 200

var parameterName = regexp.MustCompile(`^\*{0,2}([A-Za-z_][A-Za-z0-9_]*)`)

// SignatureParameters lists the parameter names of a Python signature such as
// ticket_price(age, *, member=False).
func SignatureParameters(signature string) []string {
	open := strings.Index(signature, "(")
	closing := strings.LastIndex(signature, ")")
	if open < 0 || closing < open {
		return nil
	}
	parts, err := splitTopLevel(signature[open+1 : closing])
	if err != nil {
		return nil
	}
	var names []string
	for _, part := range parts {
		if match := parameterName.FindStringSubmatch(strings.TrimSpace(part)); match != nil {
			names = append(names, match[1])
		}
	}
	return names
}

func writeFile(location string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(location), treeDirectoryMode); err != nil {
		return fmt.Errorf("create the directory of %s: %w", location, err)
	}
	if err := os.WriteFile(location, content, treeFileMode); err != nil {
		return fmt.Errorf("write %s: %w", location, err)
	}
	return nil
}

func firstLine(data []byte) string {
	line, _, _ := strings.Cut(strings.TrimSpace(string(data)), "\n")
	return truncate(line)
}

func truncate(text string) string {
	runes := []rune(text)
	if len(runes) <= maxMessageRunes {
		return text
	}
	return string(runes[:maxMessageRunes]) + "…"
}

// describeFailures lists failing cases with the first line of their message.
func describeFailures(cases []CaseResult) string {
	var lines []string
	for _, c := range cases {
		line := c.ID
		if message, _, _ := strings.Cut(c.Message, "\n"); message != "" {
			line += ": " + truncate(message)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
