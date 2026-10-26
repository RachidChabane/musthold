package prove

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/domain/power"
	"github.com/RachidChabane/musthold/discpower/internal/domain/prompt"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
)

// Names of the inputs whose role admits only one descriptor.
const (
	inputNameReference = "reference-outcomes"
	inputNameBinding   = "example-binding"
	inputNamePolicy    = "decision-policy"
	inputNameMutantSet = "mutant-set"
	inputNameModels    = "host-models"
	inputNameResponses = "host-responses"
	inputNameWitnesses = "witness-universe"
	noExecutorHarness  = "musthold-dp"
	noExecutorHarnessV = "none"
	modelSeparator     = "@"
	responseSeparator  = "\x00"
)

// baseInputs are the inputs of a proof that exist before any mutant does: the
// statement, the tests, the code tree, the reference results, the examples and
// their bindings, the policy and the prompt templates.
func (r *critRun) baseInputs() ([]format.Input, error) {
	inputs := []format.Input{format.SpecInput(r.c.Ref.ID, r.c.Ref.Revision, r.c.StatementDigest)}
	for _, file := range r.st.testFiles {
		inputs = append(inputs, format.FileInput(format.RoleTestSource, file, r.p.testFileDigests[file]))
	}
	for _, file := range r.st.codeFiles {
		content, ok := r.p.sourceContent(file)
		if !ok {
			return nil, fmt.Errorf("the code tree file %q is not a source file", file)
		}
		inputs = append(inputs, format.FileInput(format.RoleCodeTree, file, format.SHA256Hex(content)))
	}
	outcomes := map[string]string{}
	for _, test := range r.c.TestIDs() {
		outcomes[string(test)] = string(r.p.referenceOutcomes[test])
	}
	resultDigest, err := format.ResultDigest(outcomes)
	if err != nil {
		return nil, err
	}
	bindingDigest, err := r.p.bindingDigest()
	if err != nil {
		return nil, err
	}
	policyDigest, err := r.rules.Digest()
	if err != nil {
		return nil, err
	}
	inputs = append(inputs,
		descriptor(format.RoleJUnitResult, inputNameReference, resultDigest),
		descriptor(format.RoleExampleBinding, inputNameBinding, bindingDigest),
		descriptor(format.RoleDecisionPolicy, inputNamePolicy, policyDigest),
	)
	for kind, digest := range prompt.TemplateDigests() {
		inputs = append(inputs, descriptor(format.RolePromptTemplate, string(kind), digest))
	}
	return inputs, nil
}

func descriptor(role format.InputRole, name, digest string) format.Input {
	return format.Input{Role: role, Name: name, Digest: format.Digest{SHA256: digest}}
}

// bindingDigest digests the examples of every criterion and the operation
// bindings, which every example result depends on.
func (p *project) bindingDigest() (string, error) {
	canonical, err := format.Canonicalize(struct {
		Operations map[string]string `json:"operations"`
		Examples   []spec.Example    `json:"examples"`
	}{p.settings.Operations, p.allExamples()})
	if err != nil {
		return "", err
	}
	return format.SHA256Hex(canonical), nil
}

func (p *project) sourceContent(file string) ([]byte, bool) {
	for _, source := range p.sources {
		if source.Path == file {
			return source.Content, true
		}
	}
	return nil, false
}

// subject is the in-toto subject of the criterion's Statement.
func (r *critRun) statementSubject() []format.Subject {
	return []format.Subject{format.RequirementSubject(r.c.Ref.ID, r.c.Ref.Revision, r.c.StatementDigest)}
}

// freshnessKey names the inputs that exist before any mutant does: it tells
// whether the Statement of the current inputs was already written.
func (r *critRun) freshnessKey(inputs []format.Input) (string, error) {
	key, err := format.CacheKey(r.statementSubject(), inputs)
	if err != nil {
		return "", err
	}
	return digestText(key + seedSeparator + fmt.Sprint(r.p.settings.Seed)), nil
}

// finalInputs adds the inputs that depend on the pool and on the host's answers:
// the mutant set, the models that answered and the answers themselves.
func (r *critRun) finalInputs(base []format.Input) []format.Input {
	ids := make([]string, len(r.pool))
	for index, m := range r.pool {
		ids[index] = m.spec.ID
	}
	slices.Sort(ids)
	return append(slices.Clone(base),
		descriptor(format.RoleMutantSet, inputNameMutantSet, digestText(strings.Join(ids, responseSeparator))),
		descriptor(format.RoleModelID, inputNameModels, digestText(strings.Join(r.modelNames(), responseSeparator))),
		descriptor(format.RoleHostResponse, inputNameResponses, digestText(strings.Join(r.responseDigests(), responseSeparator))),
		descriptor(format.RoleHostResponse, inputNameWitnesses, universeDigest(r.records)),
	)
}

func (r *critRun) modelNames() []string {
	var names []string
	for _, profile := range r.executors() {
		names = append(names, profile.Model.Name+modelSeparator+string(profile.Model.Source))
	}
	slices.Sort(names)
	return slices.Compact(names)
}

// responseDigests lists the answers the Statement rests on as sorted
// `requestId.vote.outputDigest` lines, so that neither the nonces nor the order
// they were consumed in reach the Statement.
func (r *critRun) responseDigests() []string {
	var lines []string
	for _, answer := range r.answers {
		for _, vote := range answer.Votes {
			digest, err := hostproto.OutputDigest(vote.Output)
			if err != nil {
				digest = digestText(string(vote.Output))
			}
			lines = append(lines, fmt.Sprintf("%s.%d.%s", answer.ID, vote.Ticket.Vote, digest))
		}
	}
	slices.Sort(lines)
	return slices.Compact(lines)
}

func (r *critRun) executors() []format.ExecutorProfile {
	var profiles []format.ExecutorProfile
	for _, answer := range r.answers {
		for _, vote := range answer.Votes {
			profiles = append(profiles, vote.Executor)
		}
	}
	return profiles
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
		)
	}), true
}

// proofExecutor is the executor of the Statement: the weakest of those whose
// answers it rests on. A Statement that rests on no answer records an inline,
// unknown executor, the weakest claim the format admits.
func (r *critRun) proofExecutor() format.Executor {
	profile, ok := weakest(r.executors())
	if !ok {
		profile = format.ExecutorProfile{
			Channel:   format.ChannelHostInline,
			Harness:   format.Harness{Name: noExecutorHarness, Version: noExecutorHarnessV},
			Isolation: format.IsolationInline,
			Model:     format.ExecutorModel{Source: format.ModelSourceUnknown},
		}
	}
	return format.Executor{ExecutorProfile: profile, PromptDigest: prompt.TemplateDigests()[hostproto.KindGenerateTargeted], Blind: true}
}

// predicate assembles the discriminating-power predicate of the criterion.
func (r *critRun) predicate(inputs []format.Input, cacheKey string, correctCodePasses bool, evaluation power.Evaluation) (format.DiscriminatingPower, error) {
	tests := make([]string, 0, len(r.c.Tests))
	for _, test := range r.subject.Tests {
		tests = append(tests, string(test))
	}
	slices.Sort(tests)
	mutants, err := r.mutantEntries(evaluation.Decisions)
	if err != nil {
		return format.DiscriminatingPower{}, err
	}
	extensions := format.Extensions{
		OperatorScreen: &format.OperatorScreen{
			Flagged: r.flagged, ScreenedOut: r.screenedOut, FloorEntries: r.screenFloorEntries, Classify: r.classifications(),
		},
		Witnesses: r.witnessSummary(),
		Faults:    faultClasses(evaluation.Faults),
	}
	return format.DiscriminatingPower{
		Requirement: format.Requirement{ID: r.c.Ref.ID, Revision: r.c.Ref.Revision, StatementCanonicalization: format.StatementCanonicalizationC1},
		Core: format.Core{
			Module: r.p.settings.Module, Inputs: inputs, CacheKey: cacheKey,
			ProducedBy: r.p.producedBy(), Extensions: extensions,
		},
		Tests:             tests,
		CorrectCodePasses: format.Executed(correctCodePasses),
		Mutants:           mutants,
		Executor:          r.proofExecutor(),
		Examples:          r.exampleRuns(),
	}, nil
}

func (r *critRun) exampleRuns() []format.ExampleRun {
	var runs []format.ExampleRun
	for _, result := range r.p.exampleResults {
		if result.Owner != r.c.Ref {
			continue
		}
		value := format.ExamplePassed
		if !result.Passed {
			value = format.ExampleFailed
		}
		runs = append(runs, format.ExampleRun{Index: result.Index, Result: format.Executed(value)})
	}
	slices.SortFunc(runs, func(a, b format.ExampleRun) int { return cmp.Compare(a.Index, b.Index) })
	return runs
}

// mutantEntries lists the pool as the Statement's mutants, sorted by digest.
func (r *critRun) mutantEntries(decisions map[string]power.Decision) ([]format.Mutant, error) {
	entries := make([]format.Mutant, 0, len(r.pool))
	for _, m := range r.pool {
		entry, err := r.mutantEntry(m, decisions[m.spec.ID])
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	slices.SortFunc(entries, func(a, b format.Mutant) int { return strings.Compare(a.Digest, b.Digest) })
	return entries, nil
}

// claimProfiles are the executors a mutant's claim rests on: the writer and the
// scope judges of the witness a VALID_e claim rests on, the validity judges of a
// judged one, and none for a claim execution alone made.
func (r *critRun) claimProfiles(m *poolMutant, decision power.Decision) []format.ExecutorProfile {
	switch {
	case decision.Witness != "":
		return r.profilesOf(decision.Witness)
	case decision.Basis == format.BasisJudges:
		return m.profiles
	}
	return nil
}

// witnessSummary counts the criterion's witnesses by status and names the
// universe of every criterion's witnesses.
func (r *critRun) witnessSummary() *format.WitnessSummary {
	own := r.witnessRecords()
	if len(own) == 0 && len(r.records) == 0 {
		return nil
	}
	rules := r.g3w()
	summary := &format.WitnessSummary{Universe: universeDigest(r.records), Written: len(own)}
	for _, record := range own {
		switch record.Status(rules.Admission, rules.ScopeRule()) {
		case witness.StatusReadingConflict:
			summary.ReadingConflicts++
		case witness.StatusFlaky:
			summary.Flaky++
		case witness.StatusUnrunnable:
			summary.Unrunnable++
		case witness.StatusRefactorDropped:
			summary.RefactorDropped++
		case witness.StatusEscalateOnly:
			summary.EscalateOnly++
		case witness.StatusBlockBearing:
			summary.BlockBearing++
		}
	}
	return summary
}

// universeDigest digests the ids of every witness the pool was run against.
func universeDigest(records []witness.Record) string {
	ids := make([]string, len(records))
	for index, record := range records {
		ids[index] = record.Witness.ID
	}
	slices.Sort(ids)
	return digestText(strings.Join(ids, responseSeparator))
}

func faultClasses(faults []power.FaultClass) []format.FaultClass {
	var classes []format.FaultClass
	for _, fault := range faults {
		classes = append(classes, format.FaultClass{Key: fault.Key, Sites: fault.Sites, Witnesses: fault.Witnesses, Mutants: fault.Mutants})
	}
	return classes
}

func (r *critRun) mutantEntry(m *poolMutant, decision power.Decision) (format.Mutant, error) {
	validity, err := validityClaim(decision, r.claimProfiles(m, decision))
	if err != nil {
		return format.Mutant{}, err
	}
	results := make([]format.TestResult, 0, len(r.subject.Tests))
	for _, test := range r.subject.Tests {
		results = append(results, format.TestResult{Test: string(test), Outcome: format.Executed(m.facts.Outcomes[test])})
	}
	slices.SortFunc(results, func(a, b format.TestResult) int { return strings.Compare(a.Test, b.Test) })
	extensions := &format.MutantExtensions{
		Decision: &format.Decision{
			Basis: decision.Basis, Deciders: decision.Deciders, ConfidenceSource: decision.Source, ConfidenceMapping: decision.Mapping,
			RawConfidence: decision.Confidence, Threshold: r.rules.TauJudge.For(decision.Source), EscalationReason: string(decision.Reason),
		},
		Collateral: decision.Class == power.ClassCollateral,
		Class:      string(decision.Class),
		Route:      string(decision.Route),
		Witnesses:  decision.Failing,
	}
	extensions.Counterexample = r.counterexample(m, decision)
	if decision.OffTarget != nil {
		extensions.OffTarget = decision.OffTarget.String()
	}
	for _, ref := range decision.CollateralWith {
		extensions.CollateralWith = append(extensions.CollateralWith, ref.String())
	}
	return format.Mutant{Digest: m.spec.ID, Validity: validity, Sentinel: m.origin == power.OriginSentinel, Results: results, Extensions: extensions}, nil
}

// validityClaim renders a decision as the judged claim of the format, with the
// executed corroboration it rests on and, for a claim the host's judges made, the
// weakest executor among them.
func validityClaim(decision power.Decision, profiles []format.ExecutorProfile) (format.Claim[format.Validity], error) {
	claim, err := format.Judged(decision.Validity, decision.Confidence)
	if err != nil {
		return claim, err
	}
	if len(decision.Corroboration) > 0 {
		corroboration := make([]format.Corroboration, len(decision.Corroboration))
		for index, evidence := range decision.Corroboration {
			corroboration[index] = format.Corroboration{Kind: evidence.Kind, Digest: evidence.Digest}
		}
		claim = claim.Corroborated(corroboration...)
	}
	if profile, ok := weakest(profiles); ok {
		kind := hostproto.KindJudgeValidity
		if decision.Witness != "" {
			kind = hostproto.KindGenerateWitness
		}
		claim = claim.ProducedByExecutor(format.Executor{ExecutorProfile: profile, PromptDigest: prompt.TemplateDigests()[kind], Blind: true})
	}
	return claim, nil
}

// canonicalWitnesses digests the witnesses of a differential search.
func canonicalWitnesses(witnesses []power.Witness) (string, error) {
	encoded, err := json.Marshal(witnesses)
	if err != nil {
		return "", err
	}
	return digestText(string(encoded)), nil
}

// producedBy is where the Statements of the run are produced.
func (p *project) producedBy() format.ProducedBy {
	if p.settings.ProducedBy == "" {
		return format.ProducedByLocal
	}
	return p.settings.ProducedBy
}

// classifications lists the classifications the screen answers were read from, by
// request id.
func (r *critRun) classifications() []format.ClassifyRecord {
	var records []format.ClassifyRecord
	for _, answer := range r.answers {
		for _, vote := range answer.Votes {
			if vote.Classify != nil && !slices.ContainsFunc(records, func(known format.ClassifyRecord) bool { return known.RequestID == vote.Classify.RequestID }) {
				records = append(records, *vote.Classify)
			}
		}
	}
	slices.SortFunc(records, func(a, b format.ClassifyRecord) int { return strings.Compare(a.RequestID, b.RequestID) })
	return records
}
