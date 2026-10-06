package format

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// Kind names the alignment proof kind.
func (Alignment) Kind() Kind { return KindAlignment }

// Kind names the discriminating-power proof kind.
func (DiscriminatingPower) Kind() Kind { return KindDiscriminatingPower }

// Kind names the completeness proof kind.
func (Completeness) Kind() Kind { return KindCompleteness }

func (p Alignment) check(sink *collector, subject []Subject) {
	checkRequirement(sink, "predicate.requirement", p.Requirement)
	checkRequirementSubject(sink, subject, p.Requirement)
	checkCore(sink, "predicate", p.Core, subject)
	checkNotEmpty(sink, "predicate.test", p.Test)
	p.Verdict.check(sink, "predicate.verdict", ClassJudged)
	checkMember(sink, "predicate.verdict.value", p.Verdict.Value, verdicts)
	checkNotEmpty(sink, "predicate.judge.model", p.Judge.Model)
	checkDigest(sink, "predicate.judge.promptDigest", p.Judge.PromptDigest)
	if !p.Judge.Blind {
		sink.add(CodeNotBlind, "predicate.judge.blind", "the judge must have been blind")
	}
}

func (p DiscriminatingPower) check(sink *collector, subject []Subject) {
	checkRequirement(sink, "predicate.requirement", p.Requirement)
	checkRequirementSubject(sink, subject, p.Requirement)
	checkCore(sink, "predicate", p.Core, subject)
	checkNotEmptyList(sink, "predicate.tests", p.Tests)
	checkAscending(sink, "predicate.tests", p.Tests, strings.Compare)
	p.CorrectCodePasses.check(sink, "predicate.correctCodePasses", ClassExecuted)
	checkNotEmpty(sink, "predicate.generator.model", p.Generator.Model)
	checkAscending(sink, "predicate.mutants", p.Mutants, func(a, b Mutant) int { return strings.Compare(a.Digest, b.Digest) })
	for index, mutant := range p.Mutants {
		checkMutant(sink, fmt.Sprintf("predicate.mutants[%d]", index), mutant)
	}
	checkAscending(sink, "predicate.examples", p.Examples, func(a, b ExampleRun) int { return cmp.Compare(a.Index, b.Index) })
	for index, example := range p.Examples {
		path := fmt.Sprintf("predicate.examples[%d]", index)
		example.Result.check(sink, path+".result", ClassExecuted)
		checkMember(sink, path+".result.value", example.Result.Value, exampleResults)
	}
}

func (p Completeness) check(sink *collector, subject []Subject) {
	checkSpecSubject(sink, subject)
	checkCore(sink, "predicate", p.Core, subject)
	if !p.Advisory {
		sink.add(CodeNotAdvisory, "predicate.advisory", "a completeness proof is advisory")
	}
	checkAscending(sink, "predicate.questions", p.Questions, func(a, b Question) int {
		return cmp.Or(strings.Compare(a.Dimension, b.Dimension), strings.Compare(a.Question, b.Question))
	})
	for index, question := range p.Questions {
		path := fmt.Sprintf("predicate.questions[%d]", index)
		checkNotEmpty(sink, path+".dimension", question.Dimension)
		checkNotEmpty(sink, path+".question", question.Question)
		checkAscending(sink, path+".requirementIds", question.RequirementIDs, strings.Compare)
	}
	checkAscending(sink, "predicate.outOfScopeDeclared", p.OutOfScopeDeclared, func(a, b OutOfScope) int {
		return cmp.Or(strings.Compare(a.Dimension, b.Dimension), strings.Compare(a.Reason, b.Reason))
	})
	for index, declared := range p.OutOfScopeDeclared {
		path := fmt.Sprintf("predicate.outOfScopeDeclared[%d]", index)
		checkNotEmpty(sink, path+".dimension", declared.Dimension)
		checkNotEmpty(sink, path+".reason", declared.Reason)
	}
}

func checkMutant(sink *collector, path string, mutant Mutant) {
	checkDigest(sink, path+".digest", mutant.Digest)
	mutant.Validity.check(sink, path+".validity", ClassJudged)
	checkMember(sink, path+".validity.value", mutant.Validity.Value, validities)
	if mutant.Sentinel && mutant.Validity.Value != ValidityEquivalent {
		sink.add(CodeSentinelNotEquivalent, path+".validity.value", "a sentinel mutant is EQUIVALENT by construction, not %q", mutant.Validity.Value)
	}
	checkNotEmptyList(sink, path+".results", mutant.Results)
	checkAscending(sink, path+".results", mutant.Results, func(a, b TestResult) int { return strings.Compare(a.Test, b.Test) })
	for index, result := range mutant.Results {
		resultPath := fmt.Sprintf("%s.results[%d]", path, index)
		checkNotEmpty(sink, resultPath+".test", result.Test)
		result.Outcome.check(sink, resultPath+".outcome", ClassExecuted)
		checkMember(sink, resultPath+".outcome.value", result.Outcome.Value, outcomes)
	}
	if mutant.Extensions != nil {
		checkDecision(sink, path+".extensions.decision", mutant.Extensions.Decision)
	}
}

func checkCore(sink *collector, path string, core Core, subject []Subject) {
	checkNotEmpty(sink, path+".module.name", core.Module.Name)
	checkNotEmpty(sink, path+".module.version", core.Module.Version)
	checkNotEmptyList(sink, path+".inputs", core.Inputs)
	checkAscending(sink, path+".inputs", core.Inputs, func(a, b Input) int {
		return cmp.Or(strings.Compare(string(a.Role), string(b.Role)), strings.Compare(a.Name, b.Name))
	})
	for index, input := range core.Inputs {
		checkInput(sink, fmt.Sprintf("%s.inputs[%d]", path, index), input)
	}
	checkDigest(sink, path+".cacheKey", core.CacheKey)
	checkCacheKey(sink, path+".cacheKey", core, subject)
	checkMember(sink, path+".producedBy", core.ProducedBy, producers)
	checkDecision(sink, path+".extensions.decision", core.Extensions.Decision)
}

func checkInput(sink *collector, path string, input Input) {
	checkMember(sink, path+".role", input.Role, inputRoles)
	checkNotEmpty(sink, path+".name", input.Name)
	checkDigest(sink, path+".digest.sha256", input.Digest.SHA256)
	if input.Annotations != nil {
		checkAscending(sink, path+".annotations.globs", input.Annotations.Globs, strings.Compare)
	}
}

func checkCacheKey(sink *collector, path string, core Core, subject []Subject) {
	if !IsSHA256Hex(core.CacheKey) {
		return
	}
	expected, err := CacheKey(subject, core.Inputs)
	if err != nil {
		sink.add(CodeCacheKeyMismatch, path, "cache key cannot be recomputed: %v", err)
		return
	}
	if expected != core.CacheKey {
		sink.add(CodeCacheKeyMismatch, path, "cache key %s is not the digest of subject and inputs %s", core.CacheKey, expected)
	}
}

func checkDecision(sink *collector, path string, decision *Decision) {
	if decision == nil {
		return
	}
	checkMember(sink, path+".basis", decision.Basis, bases)
	checkNotEmptyList(sink, path+".deciders", decision.Deciders)
	checkAscending(sink, path+".deciders", decision.Deciders, strings.Compare)
	checkConfidence(sink, path+".rawConfidence", decision.RawConfidence)
	if decision.CalibratedConfidence != nil {
		checkConfidence(sink, path+".calibratedConfidence", *decision.CalibratedConfidence)
	}
	checkConfidence(sink, path+".threshold", decision.Threshold)
}

func checkConfidence(sink *collector, path string, value float64) {
	if !confidenceInRange(value) {
		sink.add(CodeConfidenceRange, path, "%v is outside [%v,%v]", value, MinConfidence, MaxConfidence)
	}
}

func checkRequirement(sink *collector, path string, requirement Requirement) {
	if requirement.ID == "" || strings.Contains(requirement.ID, RevisionSeparator) {
		sink.add(CodeRequirement, path+".id", "%q is not a requirement ID", requirement.ID)
	}
	if requirement.Revision < MinimumRevision {
		sink.add(CodeRequirement, path+".revision", "revision %d is below %d", requirement.Revision, MinimumRevision)
	}
	if requirement.StatementCanonicalization != StatementCanonicalizationC1 {
		sink.add(CodeRequirement, path+".statementCanonicalization", "%q is not %q", requirement.StatementCanonicalization, StatementCanonicalizationC1)
	}
}

// checkRequirementSubject enforces the subject rule of alignment and
// discriminating power: the criterion `ID~REV` and its statement digest.
func checkRequirementSubject(sink *collector, subject []Subject, requirement Requirement) {
	if !checkSingleSubject(sink, subject) {
		return
	}
	expected := SubjectName(requirement.ID, requirement.Revision)
	if subject[0].Name != expected {
		sink.add(CodeSubject, "subject[0].name", "%q is not the requirement %q", subject[0].Name, expected)
	}
}

// checkSpecSubject enforces the subject rule of completeness: a spec path and the
// hash of the file.
func checkSpecSubject(sink *collector, subject []Subject) {
	if !checkSingleSubject(sink, subject) {
		return
	}
	if !IsRelativePOSIXPath(subject[0].Name) {
		sink.add(CodeSubject, "subject[0].name", "%q is not a relative POSIX spec path", subject[0].Name)
	}
}

func checkSingleSubject(sink *collector, subject []Subject) bool {
	if len(subject) != 1 {
		sink.add(CodeSubject, "subject", "a statement has exactly one subject, found %d", len(subject))
		return false
	}
	if !IsSHA256Hex(subject[0].Digest.SHA256) {
		sink.add(CodeBadDigest, "subject[0].digest.sha256", "%q is not a SHA-256 hex digest", subject[0].Digest.SHA256)
	}
	return true
}

func checkDigest(sink *collector, path, value string) {
	if !IsSHA256Hex(value) {
		sink.add(CodeBadDigest, path, "%q is not a SHA-256 hex digest", value)
	}
}

func checkNotEmpty(sink *collector, path, value string) {
	if value == "" {
		sink.add(CodeMissingField, path, "must not be empty")
	}
}

func checkNotEmptyList[T any](sink *collector, path string, items []T) {
	if len(items) == 0 {
		sink.add(CodeMissingField, path, "must not be empty")
	}
}

func checkMember[T comparable](sink *collector, path string, value T, allowed []T) {
	if !slices.Contains(allowed, value) {
		sink.add(CodeUnknownValue, path, "%v is not one of %v", value, allowed)
	}
}

// checkAscending reports an array that is not strictly ascending under compare.
// Equal neighbours count as a violation, so duplicates are rejected too.
func checkAscending[T any](sink *collector, path string, items []T, compare func(a, b T) int) {
	for index := 1; index < len(items); index++ {
		if compare(items[index-1], items[index]) >= 0 {
			sink.add(CodeUnsorted, path, "element %d is not strictly after element %d", index, index-1)
			return
		}
	}
}
