// Package reportlint is the `report-lint` verb of the loop-1 study (PREREG
// §11.4). It checks results/REPORT.md, outside fenced blocks, inline code spans
// and lines that start with `>`, for the words a result never claims, fractions
// that summary.json does not hold, percentages without their fraction, dollars
// not labelled notional, and the text of a deviation entry. Lint is pure; Main
// reads the files.
package reportlint

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/conclusion"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Rule names a rule of PREREG §11.4.
type Rule string

// The rules.
const (
	RuleWord      Rule = "banned word"
	RuleFraction  Rule = "fraction not in summary.json"
	RulePercent   Rule = "percentage without its fraction"
	RuleDollar    Rule = "dollars not labelled notional"
	RuleDeviation Rule = "text of a deviation entry"
)

// Problem is one breach of a rule, on a line of the report counted from 1.
type Problem struct {
	Line int
	Rule Rule
	Text string
}

func (p Problem) String() string { return fmt.Sprintf("%d: %s: %s", p.Line, p.Rule, p.Text) }

var (
	bannedWords = regexp.MustCompile(`(?i)\b(proves|proven|guarantee|guarantees|safe|validated|confirms|works)\b`)
	fraction    = regexp.MustCompile(`\b\d+/\d+\b`)
	wholeFrac   = regexp.MustCompile(`^\d+/\d+$`)
	dollar      = regexp.MustCompile(`\$|\bUSD\b`)
	notional    = regexp.MustCompile(`(?i)\bnotional\b`)
)

// The marks of the parts the lint skips.
const (
	backtickFence = "```"
	tildeFence    = "~~~"
	quoteMark     = ">"
	backtick      = '`'
	percent       = "%"
)

// Input is what the lint reads.
type Input struct {
	Report     string
	Summary    []byte
	Deviations string
}

// Lint checks the report and returns every problem in line order.
func Lint(in Input) ([]Problem, error) {
	fractions, err := summaryFractions(in.Summary)
	if err != nil {
		return nil, err
	}
	entries := deviationTexts(in.Deviations)
	var templates []*regexp.Regexp
	for _, template := range conclusion.Templates() {
		templates = append(templates, conclusion.Pattern(template))
	}
	var problems []Problem
	fenced := false
	for index, raw := range strings.Split(in.Report, "\n") {
		trimmed := strings.TrimLeft(raw, " ")
		if strings.HasPrefix(trimmed, backtickFence) || strings.HasPrefix(trimmed, tildeFence) {
			fenced = !fenced
			continue
		}
		if fenced || strings.HasPrefix(trimmed, quoteMark) {
			continue
		}
		problems = append(problems, lintLine(index+1, withoutCode(raw), fractions, entries, templates)...)
	}
	return problems, nil
}

func lintLine(number int, line string, fractions map[string]bool, entries []string, templates []*regexp.Regexp) []Problem {
	var problems []Problem
	add := func(rule Rule, text string) {
		problems = append(problems, Problem{Line: number, Rule: rule, Text: text})
	}
	outside := line
	for _, template := range templates {
		outside = template.ReplaceAllString(outside, "")
	}
	for _, word := range bannedWords.FindAllString(outside, -1) {
		add(RuleWord, word)
	}
	for _, found := range fraction.FindAllString(line, -1) {
		if !fractions[found] {
			add(RuleFraction, found)
		}
	}
	for offset := 0; ; {
		at := strings.Index(line[offset:], percent)
		if at < 0 {
			break
		}
		if !fraction.MatchString(line[:offset+at]) {
			add(RulePercent, strings.TrimSpace(line))
		}
		offset += at + len(percent)
	}
	if dollar.MatchString(line) && !notional.MatchString(line) {
		add(RuleDollar, strings.TrimSpace(line))
	}
	for _, entry := range entries {
		if strings.Contains(line, entry) {
			add(RuleDeviation, entry)
		}
	}
	return problems
}

// withoutCode removes the inline code spans of a line: a run of backticks up to
// the next run of the same length. An unclosed run is kept as text.
func withoutCode(line string) string {
	var kept strings.Builder
	for index := 0; index < len(line); {
		if line[index] != backtick {
			kept.WriteByte(line[index])
			index++
			continue
		}
		run := 0
		for index+run < len(line) && line[index+run] == backtick {
			run++
		}
		closing := closingRun(line, index+run, run)
		if closing < 0 {
			kept.WriteString(line[index : index+run])
			index += run
			continue
		}
		kept.WriteByte(' ')
		index = closing + run
	}
	return kept.String()
}

// closingRun finds, from start, a run of exactly n backticks.
func closingRun(line string, start, n int) int {
	for index := start; index < len(line); {
		if line[index] != backtick {
			index++
			continue
		}
		run := 0
		for index+run < len(line) && line[index+run] == backtick {
			run++
		}
		if run == n {
			return index
		}
		index += run
	}
	return -1
}

// summaryFractions collects every string of summary.json that is a fraction.
func summaryFractions(summary []byte) (map[string]bool, error) {
	var document any
	if err := json.Unmarshal(summary, &document); err != nil {
		return nil, fmt.Errorf("summary.json: %w", err)
	}
	found := map[string]bool{}
	var walk func(value any)
	walk = func(value any) {
		switch typed := value.(type) {
		case string:
			if wholeFrac.MatchString(typed) {
				found[typed] = true
			}
		case []any:
			for _, item := range typed {
				walk(item)
			}
		case map[string]any:
			for _, item := range typed {
				walk(item)
			}
		}
	}
	walk(document)
	return found, nil
}

// entryPrefix is the list marker of a deviation entry.
const entryPrefix = "- "

// deviationTexts lists the texts of the log's entries: each line without its
// list marker, and what a well-formed entry says happened.
func deviationTexts(log string) []string {
	var texts []string
	for _, line := range strings.Split(log, "\n") {
		trimmed := strings.TrimSpace(line)
		entries := study.ParseDeviations(trimmed)
		if len(entries) != 1 {
			continue
		}
		texts = append(texts, strings.TrimPrefix(trimmed, entryPrefix))
		if happened := strings.TrimSpace(entries[0].Happened); happened != "" {
			texts = append(texts, happened)
		}
	}
	slices.Sort(texts)
	return slices.Compact(texts)
}
