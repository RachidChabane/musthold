package prove

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/alignment/internal/app/hostbridge"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/criterion"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/prompt"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// Names of the inputs of a Statement.
const (
	inputNamePolicy     = "decision-policy"
	inputNameMaskedTest = "masked-input:"
	inputNameModels     = "host-models"
	inputNameResponses  = "host-responses"
	modelSeparator      = "@"
	responseSeparator   = "\x00"
	// noExecutorHarness names the executor of a Statement that rests on no answer.
	noExecutorHarness  = "spec-proof-alignment"
	noExecutorVersion  = "none"
	noExecutorModelTag = ""
)

func descriptor(role format.InputRole, name, digest string) format.Input {
	return format.Input{Role: role, Name: name, Digest: format.Digest{SHA256: digest}}
}

func sortInputs(inputs []format.Input) []format.Input {
	slices.SortFunc(inputs, func(a, b format.Input) int {
		return cmp.Or(strings.Compare(string(a.Role), string(b.Role)), strings.Compare(a.Name, b.Name))
	})
	return slices.CompactFunc(inputs, func(a, b format.Input) bool { return a.Role == b.Role && a.Name == b.Name })
}

// baseInputs are the inputs of a Statement that exist before any answer does: the
// statement, the test and code files, the policy, the templates, the masked
// input and the discriminating-power proofs the matrix is read from.
func (s *subjectRun) baseInputs(declared criterion.Criterion, evidence testEvidence) []format.Input {
	inputs := []format.Input{
		format.SpecInput(declared.Ref.ID, declared.Ref.Revision, declared.StatementDigest),
		descriptor(format.RoleDecisionPolicy, inputNamePolicy, s.pass.policyDigest),
		descriptor(format.RoleHostResponse, inputNameMaskedTest+s.subject.Test.Label(), s.maskedDigest),
	}
	for _, file := range s.sourceFiles() {
		digest, readable := s.pass.digestOfFile(file)
		if !readable {
			continue
		}
		inputs = append(inputs, format.FileInput(format.RoleTestSource, file, digest))
	}
	for _, file := range s.masked.CodeFiles {
		digest, readable := s.pass.digestOfFile(file)
		if !readable {
			continue
		}
		inputs = append(inputs, format.FileInput(format.RoleCodeTree, file, digest))
	}
	for kind, digest := range prompt.TemplateDigests() {
		inputs = append(inputs, descriptor(format.RolePromptTemplate, string(kind), digest))
	}
	inputs = append(inputs, evidence.proofInputs...)
	return sortInputs(inputs)
}

// sourceFiles lists the test files the Statement rests on: those the masked input
// was built from, and always the file the test itself is in.
func (s *subjectRun) sourceFiles() []string {
	files := slices.Clone(s.masked.TestFiles)
	if !slices.Contains(files, s.subject.Test.File) {
		files = append(files, s.subject.Test.File)
	}
	return files
}

// finalInputs adds the inputs that depend on the host's answers: the models that
// answered and the answers themselves.
func finalInputs(base []format.Input, used []hostbridge.Answer) []format.Input {
	inputs := slices.Clone(base)
	if len(used) == 0 {
		return inputs
	}
	models, responses := map[string]bool{}, map[string]bool{}
	for _, answer := range used {
		for _, vote := range answer.Votes {
			models[vote.Executor.Model.Name+modelSeparator+string(vote.Executor.Model.Source)] = true
			digest, err := hostproto.OutputDigest(vote.Output)
			if err != nil {
				digest = format.SHA256Hex(vote.Output)
			}
			responses[fmt.Sprintf("%s.%d.%s", answer.ID, vote.Ticket.Vote, digest)] = true
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

// weakest picks the executor with the weakest isolation, then the weakest
// channel; ok is false for no executor.
func weakest(profiles []format.ExecutorProfile) (format.ExecutorProfile, bool) {
	if len(profiles) == 0 {
		return format.ExecutorProfile{}, false
	}
	channels := format.Channels()
	return slices.MinFunc(profiles, func(a, b format.ExecutorProfile) int {
		return cmp.Or(
			cmp.Compare(a.Isolation.Strength(), b.Isolation.Strength()),
			cmp.Compare(slices.Index(channels, b.Channel), slices.Index(channels, a.Channel)),
			strings.Compare(a.Model.Name, b.Model.Name),
			strings.Compare(a.Attestation.AgentID, b.Attestation.AgentID),
			strings.Compare(a.Attestation.CapDigest, b.Attestation.CapDigest),
		)
	}), true
}

// proofExecutor is the executor of the Statement: the weakest of those whose
// answers it rests on. A Statement that rests on no answer records an inline,
// unknown executor, the weakest claim the format admits.
func proofExecutor(used []hostbridge.Answer, basis format.DecisionBasis) format.Executor {
	var profiles []format.ExecutorProfile
	for _, answer := range used {
		for _, vote := range answer.Votes {
			profiles = append(profiles, vote.Executor)
		}
	}
	profile, ok := weakest(profiles)
	if !ok {
		profile = format.ExecutorProfile{
			Channel:   format.ChannelHostInline,
			Harness:   format.Harness{Name: noExecutorHarness, Version: noExecutorVersion},
			Isolation: format.IsolationInline,
			Model:     format.ExecutorModel{Name: noExecutorModelTag, Source: format.ModelSourceUnknown},
		}
	}
	kind := hostproto.KindJudgeAlignment
	if basis == format.BasisSystemOne {
		kind = hostproto.KindDecideClosed
	}
	return format.Executor{ExecutorProfile: profile, PromptDigest: prompt.TemplateDigests()[kind], Blind: true}
}

// thresholdOf is the threshold a decision of the basis was held to.
func (s *subjectRun) thresholdOf(basis format.DecisionBasis) float64 {
	switch basis {
	case format.BasisSystemOne:
		return s.pass.settings.Policy.TauRoute
	case format.BasisJudges:
		return s.pass.settings.Policy.ModalMin
	}
	return format.MaxConfidence
}

// writeStatement assembles, validates and stores the Statement of a test and its
// declared criterion, and returns its cache key.
func (s *subjectRun) writeStatement(declared criterion.Criterion, evidence testEvidence, base []format.Input, outcome decided) (string, error) {
	used := outcome.used
	inputs := finalInputs(base, used)
	subject := []format.Subject{format.RequirementSubject(declared.Ref.ID, declared.Ref.Revision, declared.StatementDigest)}
	cacheKey, err := format.CacheKey(subject, inputs)
	if err != nil {
		return "", err
	}
	decision := outcome.decision
	claim, err := format.Judged(decision.Verdict, decision.Confidence)
	if err != nil {
		return "", err
	}
	if decision.Corroborated {
		claim = claim.Corroborated(format.Corroboration{Kind: format.CorroborationSpecificMatrix, Digest: evidence.corroboration})
	}
	matrixJSON, err := json.Marshal(evidence.matrix)
	if err != nil {
		return "", err
	}
	deciders := slices.Sorted(slices.Values(decision.Deciders))
	confidence := s.confidenceOf(decision.Basis, outcome.closed)
	predicate := format.Alignment{
		Requirement: format.Requirement{ID: declared.Ref.ID, Revision: declared.Ref.Revision, StatementCanonicalization: format.StatementCanonicalizationC1},
		Core: format.Core{
			Module: s.pass.settings.Module, Inputs: inputs, CacheKey: cacheKey, ProducedBy: format.ProducedByLocal,
			Extensions: format.Extensions{
				Decision: &format.Decision{
					Basis: decision.Basis, Deciders: deciders, ConfidenceSource: confidence.source, ConfidenceMapping: confidence.mapping,
					RawConfidence: decision.Confidence, Threshold: s.thresholdOf(decision.Basis), EscalationReason: decision.Reason,
				},
				Matrix: matrixJSON,
			},
		},
		Test:     s.subject.Test.Label(),
		Verdict:  claim,
		Executor: proofExecutor(used, decision.Basis),
	}
	encoded, err := format.NewStatement(subject, predicate).Encode()
	if err != nil {
		return "", fmt.Errorf("encode the Statement of %s for %s: %w", s.subject.Test.Label(), declared.Ref, err)
	}
	if err := s.pass.deps.Proofs.Write(declared.Ref, cacheKey, encoded); err != nil {
		return "", fmt.Errorf("write the Statement of %s for %s: %w", s.subject.Test.Label(), declared.Ref, err)
	}
	return cacheKey, nil
}

// confidenceOf says where the confidence of a decision of the basis came from:
// the closed answers for System One, the judges' modal share for the judges, and
// execution for a verdict the module fixed itself.
func (s *subjectRun) confidenceOf(basis format.DecisionBasis, closed sourced) sourced {
	switch basis {
	case format.BasisSystemOne:
		return closed
	case format.BasisJudges:
		return sourced{source: format.HostVotesSource(s.pass.settings.Policy.AlignK), mapping: format.MappingModalShare}
	}
	return sourced{source: format.SourceExecution}
}
