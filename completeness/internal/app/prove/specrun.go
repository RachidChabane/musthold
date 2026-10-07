package prove

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/completeness/internal/domain/advice"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/blind"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/criterion"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/dimension"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/hunk"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/prompt"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/question"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/tournament"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto/requests"
)

const (
	freshNamespace = "fresh-completeness"
	keySeparator   = "\x00"
	// truncationMarker ends a change text that was cut to the size bound.
	truncationMarker = "\n[truncated]"
	changeNew        = "new"
	changeChanged    = "changed"
	beforeLabel      = "Before:"
	afterLabel       = "After:"
)

// specRun is the work of one pass on one spec file: the questions its signals
// raised, the answers they rest on and where the host stands.
type specRun struct {
	pass       *passState
	spec       criterion.Spec
	specDigest string
	// criteria are the `ID~REV` names of the spec's criteria, for the queue and the
	// pending file.
	criteria []string
	owned    []hunk.Hunk
	// questions are the questions raised so far.
	questions []question.Question
	// used are the closed answers the Statement rests on.
	used         []*closedSignal
	pending      bool
	unresponsive bool
}

func (p *passState) proveSpec(spec criterion.Spec) (SpecResult, error) {
	digest, readable := p.digestOfFile(spec.Path)
	if !readable {
		return SpecResult{}, fmt.Errorf("the spec file %s cannot be read", spec.Path)
	}
	run := &specRun{pass: p, spec: spec, specDigest: digest, owned: p.hunksOwnedBy(spec.Path)}
	for _, c := range spec.Criteria {
		run.criteria = append(run.criteria, c.Ref.String())
	}
	result := SpecResult{Spec: spec.Path, criteria: run.criteria}
	base := run.baseInputs()
	subject := []format.Subject{format.SpecSubject(spec.Path, digest)}
	baseKey, err := format.CacheKey(subject, base)
	if err != nil {
		return SpecResult{}, err
	}
	if p.options.Selection == SelectChanged {
		if record, fresh := run.freshRecord(baseKey); fresh {
			result.Status, result.CacheKey, result.Questions = StatusFresh, record.CacheKey, record.Questions
			return result, nil
		}
	}
	if err := run.signals(); err != nil {
		return SpecResult{}, err
	}
	result.Unresponsive = run.unresponsive
	if run.pending {
		result.Status = StatusAwaitingHost
		return result, nil
	}
	cacheKey, questions, err := run.writeStatement(subject, base)
	if err != nil {
		return SpecResult{}, err
	}
	if err := run.rememberFresh(baseKey, freshRecord{CacheKey: cacheKey, Questions: questions}); err != nil {
		return SpecResult{}, err
	}
	if err := run.consume(); err != nil {
		return SpecResult{}, err
	}
	result.Status, result.CacheKey, result.Questions = StatusProved, cacheKey, questions
	return result, nil
}

// hunksOwnedBy lists the changed units whose question belongs to the spec: the
// units of a file that the discriminating-power Statement of some criterion of
// the spec executes, or, for a file no spec claims, of the first spec.
func (p *passState) hunksOwnedBy(specPath string) []hunk.Hunk {
	var owned []hunk.Hunk
	for _, candidate := range p.changes.hunks {
		if p.ownerOf(candidate.Unit.File) == specPath {
			owned = append(owned, candidate)
		}
	}
	return owned
}

func (p *passState) ownerOf(file string) string {
	for _, spec := range p.project.Specs {
		for _, c := range spec.Criteria {
			if slices.Contains(p.powers[c.Ref].files, file) {
				return spec.Path
			}
		}
	}
	return p.project.Specs[0].Path
}

// signals gathers the three signals of the spec and the questions they raise.
func (s *specRun) signals() error {
	if err := s.dimensions(); err != nil {
		return err
	}
	if err := s.hunks(); err != nil {
		return err
	}
	s.unconstrained()
	return s.testability()
}

func (s *specRun) refs() []criterion.Ref { return s.pass.project.Refs() }

func (s *specRun) statementDigests(criteria []criterion.Criterion) []string {
	digests := make([]string, len(criteria))
	for index, c := range criteria {
		digests[index] = c.StatementDigest
	}
	return digests
}

// record notes how a closed request stood and keeps its answer when it has one.
func (s *specRun) record(signal *closedSignal) (answered bool) {
	switch signal.state {
	case askPending:
		s.pending = true
	case askUnresponsive:
		s.unresponsive = true
	default:
		s.used = append(s.used, signal)
		return true
	}
	return false
}

// dimensions asks System One, once per spec digest, which dimensions of the
// checklist the spec leaves unaddressed. A dimension the owner declared out of
// scope is not asked, so it can never yield a question.
func (s *specRun) dimensions() error {
	asked := dimension.Remaining(s.spec.DeclaredDimensions())
	if len(asked) == 0 || len(s.spec.Criteria) == 0 {
		return nil
	}
	tokenized := blind.Tokenize(s.spec.Criteria, s.specDigest, s.refs())
	built, err := prompt.BuildDimensions(prompt.DimensionsInput{Criteria: tokenized, Dimensions: asked})
	if err != nil {
		return err
	}
	signal, err := s.pass.ask(closedRequest{built: built, statementDigests: s.statementDigests(s.spec.Criteria), criteria: s.criteria})
	if err != nil {
		return err
	}
	if !s.record(signal) {
		return nil
	}
	for _, gap := range advice.DimensionGaps(asked, signal.reading.Questions, s.pass.settings.Policy.TauAdv) {
		s.questions = append(s.questions, question.Missing(gap.Dimension, gap.LowConfidence))
	}
	return nil
}

// hunks asks, for every changed unit of the spec that no example probe and no
// ALIGNED test may execute, which requirement states its behaviour.
func (s *specRun) hunks() error {
	var unjustified []hunk.Hunk
	for _, candidate := range s.owned {
		if len(s.pass.justifiers[candidate.Unit.Key()]) == 0 {
			unjustified = append(unjustified, candidate)
		}
	}
	limit := s.pass.settings.Policy.MaxHunks
	if len(unjustified) > limit {
		s.pass.addFinding(fmt.Sprintf("%s %s: %d changed units were not asked about, beyond max_hunks=%d", findingHunksUnreviewed, s.spec.Path, len(unjustified)-limit, limit))
		unjustified = unjustified[:limit]
	}
	for _, candidate := range unjustified {
		stated, err := s.askHunk(candidate)
		if err != nil {
			return err
		}
		if stated {
			s.questions = append(s.questions, question.UnjustifiedChange(candidate.Unit.File, candidate.Unit.Qualname))
		}
	}
	return nil
}

// askHunk plays the closed question over the requirements and NONE for a changed
// unit, as a tournament above 23 requirements, and reports whether it ended on NONE
// at or above tau_adv. A project without requirements justifies nothing, so the
// change is unjustified without asking.
func (s *specRun) askHunk(changed hunk.Hunk) (unjustified bool, err error) {
	criteria := s.pass.project.Criteria()
	if len(criteria) == 0 {
		return true, nil
	}
	tokenized := blind.Tokenize(criteria, changed.ID(), s.refs())
	change := s.pass.changeText(changed, s.refs())
	fileDigests := []string{s.pass.changes.files[changed.Unit.File]}
	var failure error
	degraded, waiting := false, false
	lookup := func(group []string, _ bool) (tournament.Answer, bool) {
		built, buildErr := prompt.BuildHunk(prompt.HunkInput{Criteria: tokenized, Group: group, Change: change})
		if buildErr != nil {
			failure = buildErr
			return tournament.Answer{}, true
		}
		request := closedRequest{built: built, statementDigests: s.groupDigests(tokenized, group), fileDigests: fileDigests, criteria: s.criteria}
		signal, askErr := s.pass.ask(request)
		if askErr != nil {
			failure = askErr
			return tournament.Answer{}, true
		}
		if !s.record(signal) {
			if signal.state == askPending {
				waiting = true
				return tournament.Answer{}, false
			}
			degraded = true
			return tournament.Answer{Choice: blind.NoneAnswer}, true
		}
		decided := signal.reading.Questions[prompt.QuestionCriterion]
		if !slices.Contains(group, decided.Choice) {
			decided.Choice = blind.NoneAnswer
		}
		return tournament.Answer{Choice: decided.Choice, Confidence: decided.Confidence}, true
	}
	outcome := tournament.Resolve(tokenized.Tokens, lookup)
	switch {
	case failure != nil:
		return false, failure
	case waiting, degraded:
		return false, nil
	}
	return advice.Unjustified(outcome, s.pass.settings.Policy.TauAdv), nil
}

func (s *specRun) groupDigests(tokenized blind.Tokenized, group []string) []string {
	var digests []string
	for _, token := range group {
		if c, found := s.pass.project.Find(tokenized.Refs[token]); found {
			digests = append(digests, c.StatementDigest)
		}
	}
	return digests
}

// changeText is what a host is shown of a changed unit: where it is, and its code,
// before and after when it existed. IDs are scrubbed and the text is bounded.
func (p *passState) changeText(changed hunk.Hunk, refs []criterion.Ref) string {
	state := changeChanged
	if changed.New {
		state = changeNew
	}
	parts := []string{fmt.Sprintf("%s %s in %s", state, changed.Unit.Qualname, changed.Unit.File)}
	if !changed.New {
		parts = append(parts, beforeLabel, changed.Before)
	}
	parts = append(parts, afterLabel, changed.Unit.Text)
	text := blind.Scrub(strings.Join(parts, "\n"), refs)
	if limit := p.settings.Policy.MaxHunkBytes; len(text) > limit {
		text = text[:limit] + truncationMarker
	}
	return text
}

// unconstrained raises a question for each requirement of the spec whose
// discriminating-power Statement lists mutants no requirement owns and executes a
// changed file: the Statement does not say where its mutants lie, so the question
// names the changed functions of the file.
func (s *specRun) unconstrained() {
	changedFiles := slices.Sorted(maps.Keys(s.pass.changes.files))
	for _, c := range s.spec.Criteria {
		power := s.pass.powers[c.Ref]
		if len(power.unattributed) == 0 {
			continue
		}
		for _, file := range changedFiles {
			if !slices.Contains(power.files, file) {
				continue
			}
			var functions []string
			for _, changed := range s.pass.changes.hunksIn(file) {
				functions = append(functions, changed.Unit.Qualname)
			}
			s.questions = append(s.questions, question.UnconstrainedBehaviour(file, functions, c.Ref.ID, power.unattributed))
		}
	}
}

// testability asks System One whether a test could verify each requirement of the
// spec, in groups of at most one request holds.
func (s *specRun) testability() error {
	if len(s.spec.Criteria) == 0 {
		return nil
	}
	tokenized := blind.Tokenize(s.spec.Criteria, s.specDigest+keySeparator+string(prompt.SignalTestability), s.refs())
	for _, group := range prompt.TestabilityGroups(tokenized.Tokens) {
		built, err := prompt.BuildTestability(prompt.TestabilityInput{Criteria: tokenized, Group: group})
		if err != nil {
			return err
		}
		signal, err := s.pass.ask(closedRequest{built: built, statementDigests: s.groupDigests(tokenized, group), criteria: s.criteria})
		if err != nil {
			return err
		}
		if !s.record(signal) {
			continue
		}
		for token, standing := range advice.Unclear(group, signal.reading.Questions) {
			s.questions = append(s.questions, question.Unclear(tokenized.Refs[token].ID, standing))
		}
	}
	return nil
}

// consume records the answers the written Statement rests on, and keeps each vote
// when a recorder is attached.
func (s *specRun) consume() error {
	answers := map[string]*closedSignal{}
	for _, signal := range s.used {
		answers[string(signal.answer.ID)] = signal
	}
	for _, id := range slices.Sorted(maps.Keys(answers)) {
		answer := answers[id].answer
		if err := s.pass.bridge.Consume(answer); err != nil {
			return err
		}
		if s.pass.deps.Record == nil {
			continue
		}
		for _, vote := range answer.Votes {
			name := requests.RecordingName(string(answer.ID), vote.Ticket.Vote)
			if err := s.pass.deps.Record.Record(name, vote.Output, vote.Executor); err != nil {
				return fmt.Errorf("record the answer to %s: %w", name, err)
			}
		}
	}
	return nil
}

// freshRecord remembers the Statement written for a base cache key.
type freshRecord struct {
	CacheKey  string `json:"cacheKey"`
	Questions int    `json:"questions"`
}

func (s *specRun) freshMemoKey(baseKey string) string {
	return format.SHA256Hex([]byte(freshNamespace + keySeparator + baseKey))
}

// freshRecord returns the record of the Statement written for the base cache key,
// when that Statement still exists.
func (s *specRun) freshRecord(baseKey string) (freshRecord, bool) {
	raw, found, err := s.pass.deps.Memo.Get(s.freshMemoKey(baseKey))
	if err != nil || !found {
		return freshRecord{}, false
	}
	var record freshRecord
	if json.Unmarshal(raw, &record) != nil {
		return freshRecord{}, false
	}
	exists, err := s.pass.deps.Proofs.Has(s.spec.Path, record.CacheKey)
	return record, err == nil && exists
}

func (s *specRun) rememberFresh(baseKey string, record freshRecord) error {
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return s.pass.deps.Memo.Put(s.freshMemoKey(baseKey), raw)
}
