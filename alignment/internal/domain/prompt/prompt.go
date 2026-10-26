// Package prompt builds the blind requests of the host protocol that alignment
// emits and reads their answers. The builders are pure: a payload shows the
// criteria under opaque tokens, never the declared criterion, and the prompts are
// embedded as data, so the digest of a template changes exactly when its text, or
// the shape of its response schema, does.
package prompt

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/RachidChabane/musthold/alignment/internal/domain/aggregate"
	"github.com/RachidChabane/musthold/alignment/internal/domain/masking"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
)

//go:embed templates
var data embed.FS

// Names of the embedded templates.
const (
	judgeSystem          = "templates/judge.system.txt"
	judgeInstructions    = "templates/judge.instructions.txt"
	decideSystem         = "templates/decide.system.txt"
	decideInstructions   = "templates/decide.instructions.txt"
	decideCriterionAsk   = "templates/decide.verifies.txt"
	decideCoverageAsk    = "templates/decide.coverage.txt"
	sectionBreak         = "\n\n"
	partSeparator        = "\x00"
	requirementsHeader   = "=== Requirements ==="
	testHeader           = "=== Test ==="
	signaturesHeader     = "=== Public signatures ==="
	questionsHeader      = "=== Questions ==="
	maxConfidencePercent = 100
	noneCriteria         = "No listed requirement is checked."
)

// Members and question identifiers of the responses.
const (
	// MemberRequirement names the criterion token in a judge response.
	MemberRequirement = "requirement"
	// MemberCoverage names the coverage in a judge response.
	MemberCoverage = "coverage"
	// MemberConfidence names the confidence, an integer percentage, in a judge response.
	MemberConfidence = "confidence"
	// QuestionCriterion is the id of the closed question that names the criterion.
	QuestionCriterion = "criterion"
	// QuestionCoverage is the id of the closed question that rates the coverage.
	QuestionCoverage = "coverage"
)

func embedded(name string) string {
	content, err := data.ReadFile(name)
	if err != nil {
		panic(fmt.Sprintf("prompt: embedded file %s: %v", name, err))
	}
	return strings.TrimSpace(string(content))
}

// Built is a prompt with the schema its answer must satisfy and, for a closed
// request, the question set.
type Built struct {
	Prompt hostproto.Prompt
	Schema json.RawMessage
	Closed json.RawMessage
}

// digestOf is the template digest: the texts a request's wording depends on and
// the skeleton of its schema, which does not depend on the tokens of one test.
func digestOf(system, skeleton string, parts ...string) string {
	return format.SHA256Hex([]byte(strings.Join(append([]string{system, skeleton}, parts...), partSeparator)))
}

// TestInput is what every request shows besides the criteria: the masked test
// unit and the public signatures of the code it calls.
type TestInput struct {
	Criteria   masking.Tokenized
	Unit       string
	Signatures string
}

func (t TestInput) body() string {
	return strings.Join([]string{
		requirementsHeader + "\n" + t.Criteria.Text,
		testHeader + "\n" + t.Unit,
		signaturesHeader + "\n" + t.Signatures,
	}, sectionBreak)
}

type schemaMember map[string]any

// BuildJudge builds the `judge.alignment` prompt and its response schema: the
// token of the criterion the test checks, or NONE, the coverage, and a confidence.
// The schema ties NONE to the coverage NONE.
func BuildJudge(in TestInput) (Built, error) {
	skeleton, err := judgeSchema([]string{"__TOKENS__"})
	if err != nil {
		return Built{}, err
	}
	schema, err := judgeSchema(in.Criteria.Tokens)
	if err != nil {
		return Built{}, err
	}
	system, instructions := embedded(judgeSystem), embedded(judgeInstructions)
	user := instructions + sectionBreak + in.body()
	return Built{
		Prompt: hostproto.Prompt{System: system, User: user, TemplateDigest: digestOf(system, string(skeleton), instructions)},
		Schema: schema,
	}, nil
}

func judgeSchema(tokens []string) (json.RawMessage, error) {
	names := append(append([]string{}, tokens...), masking.NoneAnswer)
	coverages := []string{string(aggregate.CoverageFull), string(aggregate.CoveragePartial), string(aggregate.CoverageWeak)}
	schema := schemaMember{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{MemberRequirement, MemberCoverage, MemberConfidence},
		"properties": schemaMember{
			MemberRequirement: schemaMember{"type": "string", "enum": names},
			MemberCoverage:    schemaMember{"type": "string", "enum": append(coverages, string(aggregate.CoverageNone))},
			MemberConfidence:  schemaMember{"type": "integer", "minimum": 0, "maximum": maxConfidencePercent},
		},
		"oneOf": []schemaMember{
			{"properties": schemaMember{
				MemberRequirement: schemaMember{"const": masking.NoneAnswer},
				MemberCoverage:    schemaMember{"const": string(aggregate.CoverageNone)},
			}},
			{"properties": schemaMember{
				MemberRequirement: schemaMember{"not": schemaMember{"const": masking.NoneAnswer}},
				MemberCoverage:    schemaMember{"enum": coverages},
			}},
		},
	}
	return json.Marshal(schema)
}

// JudgeAnswer is one judge's answer: the token it named, the coverage and the
// confidence in [0,1].
type JudgeAnswer struct {
	Token      string
	Coverage   aggregate.Coverage
	Confidence float64
}

// ParseJudge reads a response that satisfies the judge schema.
func ParseJudge(output []byte) (JudgeAnswer, error) {
	var document struct {
		Requirement string `json:"requirement"`
		Coverage    string `json:"coverage"`
		Confidence  int    `json:"confidence"`
	}
	if err := json.Unmarshal(output, &document); err != nil {
		return JudgeAnswer{}, fmt.Errorf("read the judge response: %w", err)
	}
	return JudgeAnswer{
		Token:      document.Requirement,
		Coverage:   aggregate.Coverage(document.Coverage),
		Confidence: float64(document.Confidence) / maxConfidencePercent,
	}, nil
}

// ClosedInput is a round of the criterion question: the options are the tokens
// of one group. The final round also asks for the coverage.
type ClosedInput struct {
	Test  TestInput
	Group []string
	Final bool
}

// BuildClosed builds the `decide.closed` prompt, its question set and its
// response schema: the criterion question over the group and NONE, and, in the
// final round, the coverage question.
func BuildClosed(in ClosedInput) (Built, error) {
	statements := in.Test.Criteria.Statements
	options := make([]hostproto.ClosedOption, 0, len(in.Group)+1)
	for _, token := range in.Group {
		options = append(options, hostproto.ClosedOption{Name: token, Description: statements[token]})
	}
	options = append(options, hostproto.ClosedOption{Name: masking.NoneAnswer, Description: noneCriteria})
	questions := []hostproto.ClosedQuestion{hostproto.ChoiceQuestion(QuestionCriterion, embedded(decideCriterionAsk), options)}
	if in.Final {
		questions = append(questions, coverageQuestion())
	}
	state := stateOf(in.Test, in.Group)
	set := hostproto.ClosedSet{State: state, Questions: questions}
	closed, err := set.Marshal()
	if err != nil {
		return Built{}, err
	}
	schema, err := set.ResponseSchema()
	if err != nil {
		return Built{}, err
	}
	skeleton, err := hostproto.ClosedSet{Questions: []hostproto.ClosedQuestion{
		hostproto.ChoiceQuestion(QuestionCriterion, "", []hostproto.ClosedOption{{Name: "__TOKENS__"}}), coverageQuestion(),
	}}.ResponseSchema()
	if err != nil {
		return Built{}, err
	}
	system, instructions := embedded(decideSystem), embedded(decideInstructions)
	user := state + sectionBreak + questionsHeader + "\n" + describe(questions) + sectionBreak + instructions
	return Built{
		Prompt: hostproto.Prompt{System: system, User: user, TemplateDigest: digestOf(system, string(skeleton), instructions, embedded(decideCriterionAsk), embedded(decideCoverageAsk))},
		Schema: schema,
		Closed: closed,
	}, nil
}

// coverageLevelCriteria says what each coverage level means to the answerer.
var coverageLevelCriteria = map[aggregate.Coverage]string{
	aggregate.CoverageNone:    noneCriteria,
	aggregate.CoverageWeak:    "An implementation violating the requirement would plausibly still pass.",
	aggregate.CoveragePartial: "The assertions cover a correct but strict subset of the clauses of the requirement.",
	aggregate.CoverageFull:    "The assertions cover the required outcome of the requirement under its stated condition.",
}

func coverageQuestion() hostproto.ClosedQuestion {
	levels := make([]hostproto.ClosedOption, 0, len(aggregate.Levels()))
	for _, level := range aggregate.Levels() {
		levels = append(levels, hostproto.ClosedOption{Name: string(level), Description: coverageLevelCriteria[level]})
	}
	return hostproto.ScoreQuestion(QuestionCoverage, embedded(decideCoverageAsk), levels)
}

// stateOf is what the questions are about: the requirements of the group, the
// test and the signatures.
func stateOf(in TestInput, group []string) string {
	statements := in.Criteria.Statements
	lines := make([]string, 0, len(group))
	for _, token := range group {
		lines = append(lines, token+": "+statements[token])
	}
	return strings.Join([]string{
		requirementsHeader + "\n" + strings.Join(lines, "\n"),
		testHeader + "\n" + in.Unit,
		signaturesHeader + "\n" + in.Signatures,
	}, sectionBreak)
}

func describe(questions []hostproto.ClosedQuestion) string {
	lines := make([]string, len(questions))
	for index, question := range questions {
		lines[index] = fmt.Sprintf("%s: %s Options: %s.", question.ID, question.Instructions, strings.Join(question.OptionNames(), ", "))
	}
	return strings.Join(lines, "\n")
}

// TemplateDigests returns the digest of the template of every kind of request the
// module emits.
func TemplateDigests() map[hostproto.Kind]string {
	empty := masking.Tokenized{}
	judge, err := BuildJudge(TestInput{Criteria: empty})
	if err != nil {
		panic(fmt.Sprintf("prompt: the judge template does not build: %v", err))
	}
	closed, err := BuildClosed(ClosedInput{Test: TestInput{Criteria: empty}, Final: true})
	if err != nil {
		panic(fmt.Sprintf("prompt: the closed template does not build: %v", err))
	}
	return map[hostproto.Kind]string{
		hostproto.KindJudgeAlignment: judge.Prompt.TemplateDigest,
		hostproto.KindDecideClosed:   closed.Prompt.TemplateDigest,
	}
}
