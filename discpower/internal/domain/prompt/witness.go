package prompt

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/witness"
)

// Names of the embedded templates and schemas of the witness and scope requests.
const (
	witnessSystem       = "templates/witness.system.txt"
	witnessInstructions = "templates/witness.instructions.txt"
	scopeSystem         = "templates/scope.system.txt"
	scopeInstructions   = "templates/scope.instructions.txt"
	witnessSchema       = "schemas/witness.json"
	scopeSchema         = "schemas/scope.json"
)

const (
	labelPrefix        = "R"
	witnessLabelPrefix = "w"
	codeFence          = "```"
)

// Errors of a witness or scope response that cannot be used.
var (
	ErrIncompleteWitnesses = errors.New("the witnesses are not 3 to 6 examples and 1 to 3 properties")
	ErrDuplicateWitness    = errors.New("two witnesses share a name")
	ErrUnansweredWitness   = errors.New("a witness has no verdict")
)

// Labels names the criteria of a spec by position, `R1`, `R2`, …, in the order
// of their references, so that a payload can show the spec and its precedence
// without naming a criterion by its identifier.
type Labels struct {
	refs []spec.Ref
}

// NewLabels labels the criteria.
func NewLabels(refs []spec.Ref) Labels {
	ordered := slices.Clone(refs)
	slices.SortFunc(ordered, spec.Ref.Compare)
	return Labels{refs: slices.CompactFunc(ordered, func(a, b spec.Ref) bool { return a == b })}
}

// Of returns the label of a criterion, or "" for one the labels do not know.
func (l Labels) Of(ref spec.Ref) string {
	index := slices.Index(l.refs, ref)
	if index < 0 {
		return ""
	}
	return fmt.Sprintf("%s%d", labelPrefix, index+1)
}

// Relabel replaces, in one pass, every `ID~REV` and every bare `ID` of a
// labelled criterion in a text by its label, the longest match first.
func (l Labels) Relabel(text string) string {
	type replacement struct{ old, new string }
	var replacements []replacement
	for _, ref := range l.refs {
		replacements = append(replacements, replacement{ref.String(), l.Of(ref)}, replacement{ref.ID, l.ofID(ref.ID)})
	}
	slices.SortStableFunc(replacements, func(a, b replacement) int { return len(b.old) - len(a.old) })
	pairs := make([]string, 0, 2*len(replacements))
	for _, item := range replacements {
		pairs = append(pairs, item.old, item.new)
	}
	return strings.NewReplacer(pairs...).Replace(text)
}

// ofID labels a bare identifier by its lowest revision.
func (l Labels) ofID(id string) string {
	for _, ref := range l.refs {
		if ref.ID == id {
			return l.Of(ref)
		}
	}
	return ""
}

// WitnessInput is what a witness request is built from.
type WitnessInput struct {
	// Spec is the text of the spec documents, relabelled.
	Spec string
	// Target is the label of the criterion to check and Statement its text,
	// relabelled.
	Target    string
	Statement string
	// Language names the language and test framework the witnesses are written
	// for, and Conventions how a witness file of that ecosystem is written.
	Language    string
	Conventions string
	// Interface lists the public signatures of the code under test and Helpers
	// those of the project's test helpers, never a body.
	Interface []string
	Helpers   []string
}

// BuildWitness builds the `generate.witness` prompt.
func BuildWitness(input WitnessInput) Built {
	var user strings.Builder
	user.WriteString("Specification, its requirements labelled R1, R2 and so on:\n" + input.Spec + sectionBreak)
	fmt.Fprintf(&user, "Requirement to check: %s\n%s%s", input.Target, input.Statement, sectionBreak)
	user.WriteString("Language and test framework: " + input.Language + sectionBreak)
	writeList(&user, "Public interface of the code under test:", input.Interface)
	writeList(&user, "Test helpers of the project:", input.Helpers)
	user.WriteString("How a witness is written in this project:\n" + input.Conventions + sectionBreak)
	user.WriteString(embedded(witnessInstructions))
	return build(embedded(witnessSystem), user.String(), embedded(witnessSchema), nil, embedded(witnessInstructions))
}

func writeList(out *strings.Builder, header string, items []string) {
	out.WriteString(header + "\n")
	if len(items) == 0 {
		out.WriteString("(none)\n")
	}
	for _, item := range items {
		out.WriteString(strings.TrimRight(item, "\n") + "\n")
	}
	out.WriteString("\n")
}

// ParseWitnesses reads a response that satisfies the witness schema into the set
// of witnesses it gives for the criterion. A response that is not 3 to 6
// examples and 1 to 3 properties, or that names two witnesses alike, is refused.
func ParseWitnesses(output []byte, criterion spec.Ref) (witness.Set, error) {
	var document struct {
		Imports   string `json:"imports"`
		Witnesses []struct {
			Name   string       `json:"name"`
			Clause string       `json:"clause"`
			Kind   witness.Kind `json:"kind"`
			Code   string       `json:"code"`
		} `json:"witnesses"`
	}
	if err := json.Unmarshal(output, &document); err != nil {
		return witness.Set{}, fmt.Errorf("read the witness response: %w", err)
	}
	set := witness.Set{Criterion: criterion, Imports: document.Imports}
	names := map[string]bool{}
	for _, item := range document.Witnesses {
		if names[item.Name] {
			return witness.Set{}, fmt.Errorf("%w: %s", ErrDuplicateWitness, item.Name)
		}
		names[item.Name] = true
		if !item.Kind.Valid() {
			return witness.Set{}, fmt.Errorf("read the witness response: unknown kind %q", item.Kind)
		}
		set.Witnesses = append(set.Witnesses, witness.New(criterion, item.Name, item.Clause, item.Kind, item.Code))
	}
	if !set.Complete() {
		return witness.Set{}, ErrIncompleteWitnesses
	}
	return set, nil
}

// ScopeWitness is one witness as a scope judge sees it.
type ScopeWitness struct {
	Clause string
	Code   string
}

// ScopeInput is what a scope request is built from.
type ScopeInput struct {
	Spec      string
	Target    string
	Statement string
	Witnesses []ScopeWitness
}

// ScopeLabel is the opaque id of the witness at index in a scope request.
func ScopeLabel(index int) string { return fmt.Sprintf("%s%d", witnessLabelPrefix, index+1) }

// BuildScope builds the `judge.scope` prompt.
func BuildScope(input ScopeInput) Built {
	var user strings.Builder
	user.WriteString("Specification, its requirements labelled R1, R2 and so on:\n" + input.Spec + sectionBreak)
	fmt.Fprintf(&user, "Requirement the witnesses check: %s\n%s%s", input.Target, input.Statement, sectionBreak)
	for index, item := range input.Witnesses {
		fmt.Fprintf(&user, "Witness %s, for the clause %q:\n%s\n%s\n%s\n\n", ScopeLabel(index), item.Clause, codeFence, strings.TrimRight(item.Code, "\n"), codeFence)
	}
	user.WriteString(embedded(scopeInstructions))
	return build(embedded(scopeSystem), user.String(), embedded(scopeSchema), nil, embedded(scopeInstructions))
}

// ParseScope reads a response that satisfies the scope schema into one vote per
// witness label. A response that leaves one of the labels unanswered is refused.
func ParseScope(output []byte, labels []string) (map[string]witness.ScopeVote, error) {
	var document struct {
		Verdicts map[string]struct {
			Answer     witness.ScopeAnswer `json:"answer"`
			Confidence *float64            `json:"confidence"`
		} `json:"verdicts"`
	}
	if err := json.Unmarshal(output, &document); err != nil {
		return nil, fmt.Errorf("read the scope response: %w", err)
	}
	votes := make(map[string]witness.ScopeVote, len(labels))
	for _, label := range labels {
		verdict, answered := document.Verdicts[label]
		if !answered {
			return nil, fmt.Errorf("%w: %s", ErrUnansweredWitness, label)
		}
		votes[label] = witness.ScopeVote{Answer: verdict.Answer, Confidence: confidenceOf(verdict.Confidence)}
	}
	return votes, nil
}
