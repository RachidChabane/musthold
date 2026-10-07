package prove

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/completeness/internal/domain/closedq"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/criterion"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/prompt"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/question"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// Names of the inputs of a Statement.
const (
	inputNamePolicy    = "decision-policy"
	inputNameHunks     = "diff-hunks"
	inputNameModels    = "host-models"
	inputNameResponses = "host-responses"
	templateNamePrefix = "decide."
	modelSeparator     = "@"
	responseSeparator  = "\x00"
)

func descriptor(role format.InputRole, name, digest string) format.Input {
	return format.Input{Role: role, Name: name, Digest: format.Digest{SHA256: digest}}
}

func compareInputs(a, b format.Input) int {
	return cmp.Or(strings.Compare(string(a.Role), string(b.Role)), strings.Compare(a.Name, b.Name))
}

func sortInputs(inputs []format.Input) []format.Input {
	slices.SortFunc(inputs, compareInputs)
	return slices.CompactFunc(inputs, func(a, b format.Input) bool { return a.Role == b.Role && a.Name == b.Name })
}

// changedFiles lists the changed files that bear on the spec: those holding units
// it owns, and those a discriminating-power Statement of one of its criteria
// executes.
func (s *specRun) changedFiles() []string {
	files := map[string]bool{}
	for _, owned := range s.owned {
		files[owned.Unit.File] = true
	}
	for _, c := range s.spec.Criteria {
		for _, file := range s.pass.powers[c.Ref].files {
			if _, changed := s.pass.changes.files[file]; changed {
				files[file] = true
			}
		}
	}
	return slices.Sorted(maps.Keys(files))
}

// baseInputs are the inputs of a Statement that exist before any answer does: the
// spec file and the statements of its criteria, the policy, the templates, the
// changed files and units, and the Statements of the other modules the run read.
func (s *specRun) baseInputs() []format.Input {
	p := s.pass
	inputs := []format.Input{
		descriptor(format.RoleSpec, s.spec.Path, s.specDigest),
		descriptor(format.RoleDecisionPolicy, inputNamePolicy, p.policyDigest),
	}
	for _, c := range s.spec.Criteria {
		inputs = append(inputs, descriptor(format.RoleSpec, c.Ref.String(), c.StatementDigest))
	}
	templates := prompt.TemplateDigests()
	for _, signal := range prompt.Signals() {
		inputs = append(inputs, descriptor(format.RolePromptTemplate, templateNamePrefix+string(signal), templates[signal]))
	}
	files := s.changedFiles()
	var hunkIDs []string
	for _, file := range files {
		inputs = append(inputs, descriptor(format.RoleCodeTree, file, p.changes.files[file]))
		for _, changed := range p.changes.hunksIn(file) {
			hunkIDs = append(hunkIDs, changed.ID())
		}
	}
	slices.Sort(hunkIDs)
	inputs = append(inputs, descriptor(format.RoleHostResponse, inputNameHunks, format.SHA256Hex([]byte(p.changes.base+responseSeparator+strings.Join(hunkIDs, responseSeparator)))))
	inputs = append(inputs, p.proofInputs...)
	return sortInputs(inputs)
}

// finalInputs adds the inputs that depend on the host's answers: the models that
// answered and the answers themselves.
func (s *specRun) finalInputs(base []format.Input) []format.Input {
	inputs := slices.Clone(base)
	if len(s.used) == 0 {
		return inputs
	}
	models, responses := map[string]bool{}, map[string]bool{}
	for _, signal := range s.used {
		for _, vote := range signal.answer.Votes {
			models[vote.Executor.Model.Name+modelSeparator+string(vote.Executor.Model.Source)] = true
			digest, err := hostproto.OutputDigest(vote.Output)
			if err != nil {
				digest = format.SHA256Hex(vote.Output)
			}
			responses[fmt.Sprintf("%s.%d.%s", signal.answer.ID, vote.Ticket.Vote, digest)] = true
		}
	}
	return sortInputs(append(inputs,
		descriptor(format.RoleModelID, inputNameModels, digestOfSet(models)),
		descriptor(format.RoleHostResponse, inputNameResponses, digestOfSet(responses)),
	))
}

func digestOfSet(set map[string]bool) string {
	return format.SHA256Hex([]byte(strings.Join(slices.Sorted(maps.Keys(set)), responseSeparator)))
}

// decision records how the closed answers the Statement rests on were read: where
// their confidence came from, the lowest of them and the threshold they were held
// to. It is absent when no answer was used.
func (s *specRun) decision() *format.Decision {
	deciders := map[string]bool{}
	confidence, answered := format.MaxConfidence, false
	for _, signal := range s.used {
		deciders[closedq.Decider(signal.reading.Source)] = true
		for _, decided := range signal.reading.Questions {
			confidence, answered = min(confidence, decided.Confidence), true
		}
	}
	if !answered {
		return nil
	}
	return &format.Decision{
		Basis: format.BasisSystemOne, Deciders: slices.Sorted(maps.Keys(deciders)),
		RawConfidence: confidence, Threshold: s.pass.settings.Policy.TauAdv,
	}
}

// writeStatement assembles, validates and stores the Statement of the spec, and
// returns its cache key and the number of its questions.
func (s *specRun) writeStatement(subject []format.Subject, base []format.Input) (string, int, error) {
	inputs := s.finalInputs(base)
	cacheKey, err := format.CacheKey(subject, inputs)
	if err != nil {
		return "", 0, err
	}
	questions := question.Normalize(s.questions)
	predicate := format.Completeness{
		Core: format.Core{
			Module: s.pass.settings.Module, Inputs: inputs, CacheKey: cacheKey, ProducedBy: format.ProducedByLocal,
			Extensions: format.Extensions{Decision: s.decision()},
		},
		Advisory:           true,
		Questions:          formatQuestions(questions),
		OutOfScopeDeclared: formatDeclarations(s.spec.OutOfScope),
	}
	encoded, err := format.NewStatement(subject, predicate).Encode()
	if err != nil {
		return "", 0, fmt.Errorf("encode the Statement of %s: %w", s.spec.Path, err)
	}
	if err := s.pass.deps.Proofs.Write(s.spec.Path, cacheKey, encoded); err != nil {
		return "", 0, fmt.Errorf("write the Statement of %s: %w", s.spec.Path, err)
	}
	return cacheKey, len(questions), nil
}

func formatQuestions(questions []question.Question) []format.Question {
	converted := make([]format.Question, len(questions))
	for index, q := range questions {
		converted[index] = format.Question{Dimension: q.Dimension, Question: q.Text, RequirementIDs: q.RequirementIDs}
	}
	return converted
}

// formatDeclarations lists the out-of-scope declarations sorted by dimension, then
// reason, each pair once.
func formatDeclarations(declared []criterion.Declaration) []format.OutOfScope {
	converted := make([]format.OutOfScope, len(declared))
	for index, declaration := range declared {
		converted[index] = format.OutOfScope{Dimension: declaration.Dimension, Reason: declaration.Reason}
	}
	slices.SortFunc(converted, func(a, b format.OutOfScope) int {
		return cmp.Or(strings.Compare(a.Dimension, b.Dimension), strings.Compare(a.Reason, b.Reason))
	})
	return slices.Compact(converted)
}
