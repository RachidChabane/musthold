// Package prompt builds the blind requests of the host protocol and reads their
// answers. The builders are pure: generation never sees tests, no payload names a
// criterion by its identifier, and the prompts and response schemas are embedded
// as data, so the digest of a template changes exactly when its text does.
package prompt

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/differential"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

//go:embed templates schemas
var data embed.FS

// Names of the embedded templates and schemas.
const (
	generateSystem        = "templates/generate.system.txt"
	generateInstructions  = "templates/generate.instructions.txt"
	judgeSystem           = "templates/judge.system.txt"
	validityInstructions  = "templates/judge.validity.instructions.txt"
	equivalentInstruction = "templates/judge.equivalent.instructions.txt"
	screenSystem          = "templates/screen.system.txt"
	screenInstructions    = "templates/screen.instructions.txt"
	generateSchema        = "schemas/generate.json"
	validitySchema        = "schemas/judge-validity.json"
	equivalentSchema      = "schemas/judge-equivalent.json"
)

const (
	redacted        = "the requirement"
	sectionBreak    = "\n\n"
	partSeparator   = "\x00"
	questionPrefix  = "q"
	statementHeader = "Requirement:"
)

// MaxQuestions is the most questions one closed request carries.
const MaxQuestions = hostproto.MaxClosedQuestions

// Criteria text of the two answers of a screen question.
const (
	violatesCriteria   = "The change breaks the requirement."
	consistentCriteria = "The requirement is still met."
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

func build(system, user, schema string, closed json.RawMessage, parts ...string) Built {
	digest := format.SHA256Hex([]byte(strings.Join(append([]string{system, schema}, parts...), partSeparator)))
	return Built{
		Prompt: hostproto.Prompt{System: system, User: user, TemplateDigest: digest},
		Schema: json.RawMessage(schema),
		Closed: closed,
	}
}

// Scrub removes every declared criterion reference, `ID~REV` and bare `ID`, from
// a text, so that a payload never names the criterion it is about.
func Scrub(text string, refs []spec.Ref) string {
	ordered := append([]spec.Ref(nil), refs...)
	sort.Slice(ordered, func(i, j int) bool { return len(ordered[i].ID) > len(ordered[j].ID) })
	for _, ref := range ordered {
		text = strings.ReplaceAll(text, ref.String(), redacted)
		text = strings.ReplaceAll(text, ref.ID, redacted)
	}
	return text
}

// SliceEntry is one unit of the footprint slice a generation request shows.
type SliceEntry struct {
	File     string
	Qualname string
	Source   string
}

// GenerationInput is what a generation request is built from.
type GenerationInput struct {
	Statement string
	Slice     []SliceEntry
	// Avoid lists the mechanisms of earlier rounds.
	Avoid []string
}

// BuildGeneration builds the `generate.targeted` prompt of one round.
func BuildGeneration(input GenerationInput) Built {
	var user strings.Builder
	user.WriteString(statementHeader + "\n" + input.Statement + sectionBreak)
	user.WriteString("Code under test:\n")
	for _, entry := range input.Slice {
		fmt.Fprintf(&user, "--- %s :: %s\n%s\n", entry.File, entry.Qualname, strings.TrimRight(entry.Source, "\n"))
	}
	user.WriteString("\n" + embedded(generateInstructions))
	if len(input.Avoid) > 0 {
		user.WriteString("\nMechanisms already tried; use different ones:\n")
		for _, mechanism := range input.Avoid {
			user.WriteString("- " + mechanism + "\n")
		}
	}
	return build(embedded(generateSystem), user.String(), embedded(generateSchema), nil, embedded(generateInstructions))
}

// EditView shows one changed unit before and after the mutant.
type EditView struct {
	File     string
	Qualname string
	Before   string
	After    string
}

// OperationView names an operation a witness may call, with its parameters.
type OperationView struct {
	Name   string
	Params []string
}

// WitnessView is an observed difference between the original and the mutant.
type WitnessView struct {
	Call      spec.Call
	Reference spec.Observation
	Mutant    spec.Observation
}

// WitnessViews converts the witnesses of a differential search.
func WitnessViews(witnesses []power.Witness) []WitnessView {
	views := make([]WitnessView, len(witnesses))
	for index, witness := range witnesses {
		views[index] = WitnessView{Call: witness.Call, Reference: witness.Reference, Mutant: witness.Mutant}
	}
	return views
}

// OperationViews lists the operations of the signatures, sorted by name.
func OperationViews(signatures []differential.Signature) []OperationView {
	views := make([]OperationView, 0, len(signatures))
	for _, signature := range signatures {
		view := OperationView{Name: signature.Operation}
		for _, param := range signature.Params {
			view.Params = append(view.Params, param.Name)
		}
		views = append(views, view)
	}
	sort.Slice(views, func(i, j int) bool { return views[i].Name < views[j].Name })
	return views
}

// JudgeInput is what a judge request is built from.
type JudgeInput struct {
	Statement  string
	Operations []OperationView
	Edits      []EditView
	Witnesses  []WitnessView
}

func writeJudgeBody(user *strings.Builder, statement string, operations []OperationView, edits []EditView, witnesses []WitnessView) {
	user.WriteString(statementHeader + "\n" + statement + sectionBreak)
	if len(operations) > 0 {
		user.WriteString("Operations you may use in a witness:\n")
		for _, operation := range operations {
			fmt.Fprintf(user, "- %s(%s)\n", operation.Name, strings.Join(operation.Params, ", "))
		}
		user.WriteString("\n")
	}
	writeChange(user, edits, witnesses)
}

// BuildValidity builds the `judge.validity` prompt.
func BuildValidity(input JudgeInput) Built {
	var user strings.Builder
	writeJudgeBody(&user, input.Statement, input.Operations, input.Edits, input.Witnesses)
	user.WriteString("\n" + embedded(validityInstructions))
	return build(embedded(judgeSystem), user.String(), embedded(validitySchema), nil, embedded(validityInstructions))
}

// BuildEquivalence builds the `judge.equivalent` prompt.
func BuildEquivalence(input JudgeInput) Built {
	var user strings.Builder
	writeJudgeBody(&user, input.Statement, nil, input.Edits, nil)
	user.WriteString("\n" + embedded(equivalentInstruction))
	return build(embedded(judgeSystem), user.String(), embedded(equivalentSchema), nil, embedded(equivalentInstruction))
}

// ScreenInput is what a screen request is built from: the statements of the
// criteria in scope, in a fixed order, and the change.
type ScreenInput struct {
	Statements []string
	Edits      []EditView
	Witnesses  []WitnessView
}

// QuestionID is the opaque id of the question about the statement at index.
func QuestionID(index int) string { return fmt.Sprintf("%s%d", questionPrefix, index+1) }

// BuildScreen builds the `decide.closed` prompt: one choice question per
// statement, at most MaxQuestions of them.
func BuildScreen(input ScreenInput) (Built, error) {
	if len(input.Statements) == 0 || len(input.Statements) > MaxQuestions {
		return Built{}, fmt.Errorf("a screen request asks 1 to %d questions, not %d", MaxQuestions, len(input.Statements))
	}
	var state strings.Builder
	questions := make([]hostproto.ClosedQuestion, len(input.Statements))
	for index, statement := range input.Statements {
		fmt.Fprintf(&state, "Requirement %d:\n%s\n\n", index+1, statement)
		questions[index] = screenQuestion(index)
	}
	writeChange(&state, input.Edits, input.Witnesses)
	set := hostproto.ClosedSet{State: state.String(), Questions: questions}
	closed, err := set.Marshal()
	if err != nil {
		return Built{}, err
	}
	schema, err := set.ResponseSchema()
	if err != nil {
		return Built{}, err
	}
	skeleton, err := hostproto.ClosedSet{Questions: []hostproto.ClosedQuestion{screenQuestion(0)}}.ResponseSchema()
	if err != nil {
		return Built{}, err
	}
	user := state.String() + "\n" + embedded(screenInstructions)
	built := build(embedded(screenSystem), user, string(skeleton), closed, embedded(screenInstructions))
	built.Schema = schema
	return built, nil
}

// screenQuestion is the question about the requirement at index: does the change
// break it.
func screenQuestion(index int) hostproto.ClosedQuestion {
	return hostproto.ChoiceQuestion(QuestionID(index), fmt.Sprintf("Does the change break requirement %d?", index+1), []hostproto.ClosedOption{
		{Name: string(power.ChoiceViolates), Description: violatesCriteria},
		{Name: string(power.ChoiceConsistent), Description: consistentCriteria},
	})
}

func writeChange(out *strings.Builder, edits []EditView, witnesses []WitnessView) {
	out.WriteString("Changed units, before then after:\n")
	for _, edit := range edits {
		fmt.Fprintf(out, "--- %s :: %s\nbefore:\n%s\nafter:\n%s\n", edit.File, edit.Qualname, strings.TrimRight(edit.Before, "\n"), strings.TrimRight(edit.After, "\n"))
	}
	if len(witnesses) > 0 {
		out.WriteString("\nObserved differences, input then original result then mutant result:\n")
		for _, witness := range witnesses {
			fmt.Fprintf(out, "- %s: original %s, mutant %s\n", FormatCall(witness.Call), FormatObservation(witness.Reference), FormatObservation(witness.Mutant))
		}
	}
}

// FormatCall renders a call as `operation(name=value, ...)`, arguments sorted.
func FormatCall(call spec.Call) string {
	names := make([]string, 0, len(call.Args))
	for name := range call.Args {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, len(names))
	for index, name := range names {
		parts[index] = name + "=" + formatLiteral(call.Args[name])
	}
	return call.Operation + "(" + strings.Join(parts, ", ") + ")"
}

func formatLiteral(literal spec.Literal) string {
	if literal.Type == spec.TypeString || literal.Type == spec.TypeDatetime {
		return fmt.Sprintf("%q", literal.Value)
	}
	return literal.Value
}

// FormatObservation renders what a call did.
func FormatObservation(observation spec.Observation) string {
	if observation.Kind == spec.Raises {
		return "raises " + observation.Value
	}
	return formatLiteral(spec.Literal{Type: observation.Type, Value: observation.Value})
}

// TemplateDigests returns the digest of the template of every kind of request the
// module emits: it changes exactly when a prompt text or a response schema does.
func TemplateDigests() map[hostproto.Kind]string {
	screen, err := BuildScreen(ScreenInput{Statements: []string{""}})
	if err != nil {
		panic(fmt.Sprintf("prompt: the screen template does not build: %v", err))
	}
	return map[hostproto.Kind]string{
		hostproto.KindGenerateTargeted: BuildGeneration(GenerationInput{}).Prompt.TemplateDigest,
		hostproto.KindJudgeValidity:    BuildValidity(JudgeInput{}).Prompt.TemplateDigest,
		hostproto.KindJudgeEquivalent:  BuildEquivalence(JudgeInput{}).Prompt.TemplateDigest,
		hostproto.KindDecideClosed:     screen.Prompt.TemplateDigest,
		hostproto.KindGenerateWitness:  BuildWitness(WitnessInput{}).Prompt.TemplateDigest,
		hostproto.KindJudgeScope:       BuildScope(ScopeInput{}).Prompt.TemplateDigest,
		hostproto.KindGeneratePromises: BuildPromises(PromisesInput{}).Prompt.TemplateDigest,
		hostproto.KindJudgeLabels:      BuildLabels(LabelsInput{}).Prompt.TemplateDigest,
	}
}
