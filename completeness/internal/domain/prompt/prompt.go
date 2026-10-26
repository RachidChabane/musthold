// Package prompt builds the closed requests of the host protocol that completeness
// emits. The builders are pure: a payload shows the requirements under opaque
// tokens, never by identifier, and the prompts are embedded as data, so the digest
// of a template changes exactly when its text, or the shape of its response
// schema, does.
package prompt

import (
	"embed"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/completeness/internal/domain/blind"
	"github.com/RachidChabane/musthold/completeness/internal/domain/closedq"
	"github.com/RachidChabane/musthold/completeness/internal/domain/dimension"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
)

//go:embed templates
var data embed.FS

// Names of the embedded templates.
const (
	systemTemplate       = "templates/closed.system.txt"
	dimensionsPrompt     = "templates/dimensions.instructions.txt"
	dimensionsQuestion   = "templates/dimensions.question.txt"
	hunkPrompt           = "templates/hunk.instructions.txt"
	hunkQuestion         = "templates/hunk.question.txt"
	testabilityPrompt    = "templates/testability.instructions.txt"
	testabilityQuestion  = "templates/testability.question.txt"
	sectionBreak         = "\n\n"
	partSeparator        = "\x00"
	requirementsHeader   = "=== Requirements ==="
	changeHeader         = "=== Change ==="
	questionsHeader      = "=== Questions ==="
	placeholderToken     = "__TOKEN__"
	noneDescription      = "No listed requirement states the behaviour of this change."
	coveredDescription   = "The requirements state the expected behaviour for this dimension."
	notApplicableMeaning = "The dimension is irrelevant to this software."
	missingDescription   = "The dimension concerns this software and the requirements are silent about it."
)

// QuestionCriterion is the id of the closed question that names the requirement a
// change implements.
const QuestionCriterion = "criterion"

// Signal names the three questions completeness asks System One; each has its own
// template.
type Signal string

// Signals of completeness.
const (
	SignalDimensions  Signal = "dimensions"
	SignalHunk        Signal = "hunk"
	SignalTestability Signal = "testability"
)

// Signals lists the signals in a stable order.
func Signals() []Signal { return []Signal{SignalDimensions, SignalHunk, SignalTestability} }

// Testability is the answer to the testability question of one requirement.
type Testability string

// Options of the testability question.
const (
	Testable   Testability = "TESTABLE"
	Ambiguous  Testability = "AMBIGUOUS"
	Untestable Testability = "UNTESTABLE"
)

// Descriptions of the testability options.
const (
	testableDescription   = "A condition and an observable outcome are stated, so a test can verify it."
	ambiguousDescription  = "More than one reading is possible, or a term is left undefined."
	untestableDescription = "No observable outcome is stated, so no test could verify it."
)

func embedded(name string) string {
	content, err := data.ReadFile(name)
	if err != nil {
		panic(fmt.Sprintf("prompt: embedded file %s: %v", name, err))
	}
	return strings.TrimSpace(string(content))
}

// Built is a prompt with the schema its answer must satisfy and the question set.
type Built struct {
	Prompt hostproto.Prompt
	Schema json.RawMessage
	Closed json.RawMessage
}

func digestOf(system string, skeleton json.RawMessage, parts ...string) string {
	return format.SHA256Hex([]byte(strings.Join(append([]string{system, string(skeleton)}, parts...), partSeparator)))
}

func assemble(set closedq.Set, skeleton closedq.Set, instructions, user string, parts ...string) (Built, error) {
	closed, err := set.Marshal()
	if err != nil {
		return Built{}, err
	}
	schema, err := set.ResponseSchema()
	if err != nil {
		return Built{}, err
	}
	skeletonSchema, err := skeleton.ResponseSchema()
	if err != nil {
		return Built{}, err
	}
	system := embedded(systemTemplate)
	return Built{
		Prompt: hostproto.Prompt{System: system, User: user, TemplateDigest: digestOf(system, skeletonSchema, append([]string{instructions}, parts...)...)},
		Schema: schema,
		Closed: closed,
	}, nil
}

func describe(questions []closedq.Question) string {
	lines := make([]string, len(questions))
	for index, question := range questions {
		lines[index] = fmt.Sprintf("%s: %s Options: %s.", question.ID, question.Instructions, strings.Join(question.OptionNames(), ", "))
	}
	return strings.Join(lines, "\n")
}

// DimensionsInput is the request about the dimensions of one spec: the criteria
// of the spec and the dimensions not declared out of scope.
type DimensionsInput struct {
	Criteria   blind.Tokenized
	Dimensions []dimension.Dimension
}

func dimensionOptions() []closedq.Option {
	return []closedq.Option{
		{Name: string(dimension.Covered), Description: coveredDescription},
		{Name: string(dimension.NotApplicable), Description: notApplicableMeaning},
		{Name: string(dimension.Missing), Description: missingDescription},
	}
}

func dimensionQuestions(dimensions []dimension.Dimension) []closedq.Question {
	questions := make([]closedq.Question, len(dimensions))
	for index, d := range dimensions {
		questions[index] = closedq.Choice(string(d), embedded(dimensionsQuestion)+" "+d.Description(), dimensionOptions())
	}
	return questions
}

// BuildDimensions builds the request with one choice question per dimension:
// COVERED, NOT_APPLICABLE or MISSING.
func BuildDimensions(in DimensionsInput) (Built, error) {
	state := requirementsHeader + "\n" + in.Criteria.Text
	set := closedq.Set{State: state, Questions: dimensionQuestions(in.Dimensions)}
	skeleton := closedq.Set{Questions: dimensionQuestions(dimension.Checklist())}
	user := state + sectionBreak + questionsHeader + "\n" + describe(set.Questions) + sectionBreak + embedded(dimensionsPrompt)
	return assemble(set, skeleton, embedded(dimensionsPrompt), user, embedded(dimensionsQuestion), optionsText(dimensionOptions()))
}

// HunkInput is a round of the question that names the requirement a change
// implements: the options are the tokens of one group.
type HunkInput struct {
	Criteria blind.Tokenized
	Group    []string
	// Change is the changed code as shown, already scrubbed and bounded.
	Change string
}

func hunkQuestionOf(tokens []string, statements map[string]string) closedq.Question {
	options := make([]closedq.Option, 0, len(tokens)+1)
	for _, token := range tokens {
		options = append(options, closedq.Option{Name: token, Description: statements[token]})
	}
	options = append(options, closedq.Option{Name: blind.NoneAnswer, Description: noneDescription})
	return closedq.Choice(QuestionCriterion, embedded(hunkQuestion), options)
}

// BuildHunk builds the choice question over the tokens of the group and NONE.
func BuildHunk(in HunkInput) (Built, error) {
	lines := make([]string, 0, len(in.Group))
	for _, token := range in.Group {
		lines = append(lines, token+": "+in.Criteria.Statements[token])
	}
	state := requirementsHeader + "\n" + strings.Join(lines, "\n") + sectionBreak + changeHeader + "\n" + in.Change
	question := hunkQuestionOf(in.Group, in.Criteria.Statements)
	set := closedq.Set{State: state, Questions: []closedq.Question{question}}
	skeleton := closedq.Set{Questions: []closedq.Question{hunkQuestionOf([]string{placeholderToken}, nil)}}
	user := state + sectionBreak + questionsHeader + "\n" + describe(set.Questions) + sectionBreak + embedded(hunkPrompt)
	return assemble(set, skeleton, embedded(hunkPrompt), user, embedded(hunkQuestion), noneDescription)
}

// TestabilityInput is the request about a group of requirements.
type TestabilityInput struct {
	Criteria blind.Tokenized
	Group    []string
}

func testabilityOptions() []closedq.Option {
	return []closedq.Option{
		{Name: string(Testable), Description: testableDescription},
		{Name: string(Ambiguous), Description: ambiguousDescription},
		{Name: string(Untestable), Description: untestableDescription},
	}
}

func testabilityQuestions(tokens []string) []closedq.Question {
	questions := make([]closedq.Question, len(tokens))
	for index, token := range tokens {
		questions[index] = closedq.Choice(token, "Requirement "+token+": "+embedded(testabilityQuestion), testabilityOptions())
	}
	return questions
}

// BuildTestability builds the request with one choice question per requirement of
// the group: TESTABLE, AMBIGUOUS or UNTESTABLE.
func BuildTestability(in TestabilityInput) (Built, error) {
	lines := make([]string, 0, len(in.Group))
	for _, token := range in.Group {
		lines = append(lines, token+": "+in.Criteria.Statements[token])
	}
	state := requirementsHeader + "\n" + strings.Join(lines, "\n")
	set := closedq.Set{State: state, Questions: testabilityQuestions(in.Group)}
	skeleton := closedq.Set{Questions: testabilityQuestions([]string{placeholderToken})}
	user := state + sectionBreak + questionsHeader + "\n" + describe(set.Questions) + sectionBreak + embedded(testabilityPrompt)
	return assemble(set, skeleton, embedded(testabilityPrompt), user, embedded(testabilityQuestion), optionsText(testabilityOptions()))
}

// TestabilityGroups splits the tokens in order into groups of at most the number
// of questions a request holds.
func TestabilityGroups(tokens []string) [][]string {
	var groups [][]string
	for start := 0; start < len(tokens); start += closedq.MaxQuestions {
		groups = append(groups, slices.Clone(tokens[start:min(start+closedq.MaxQuestions, len(tokens))]))
	}
	return groups
}

func optionsText(options []closedq.Option) string {
	parts := make([]string, len(options))
	for index, option := range options {
		parts[index] = option.Name + "=" + option.Description
	}
	return strings.Join(parts, partSeparator)
}

// TemplateDigests returns the digest of the template of every signal.
func TemplateDigests() map[Signal]string {
	empty := blind.Tokenized{Statements: map[string]string{}}
	dimensions, err := BuildDimensions(DimensionsInput{Criteria: empty, Dimensions: dimension.Checklist()})
	if err != nil {
		panic(fmt.Sprintf("prompt: the dimensions template does not build: %v", err))
	}
	hunk, err := BuildHunk(HunkInput{Criteria: empty})
	if err != nil {
		panic(fmt.Sprintf("prompt: the hunk template does not build: %v", err))
	}
	testability, err := BuildTestability(TestabilityInput{Criteria: empty})
	if err != nil {
		panic(fmt.Sprintf("prompt: the testability template does not build: %v", err))
	}
	return map[Signal]string{
		SignalDimensions:  dimensions.Prompt.TemplateDigest,
		SignalHunk:        hunk.Prompt.TemplateDigest,
		SignalTestability: testability.Prompt.TemplateDigest,
	}
}
