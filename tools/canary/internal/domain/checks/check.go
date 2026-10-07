// Package checks holds the canary checks of each harness. A check reads a
// recorded session and concludes PASS, FAIL or INCONCLUSIVE; it makes no model
// call and touches no file, so that it can be tested against recorded streams.
package checks

import (
	"fmt"

	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
)

// Check is one question a canary answers about a harness.
type Check interface {
	// Name is the stable identifier of the check.
	Name() string
	// Proves says what a PASS establishes and what it leaves open.
	Proves() string
	// Evaluate concludes from the recorded session.
	Evaluate(evidence.Evidence) verdict.Verdict
}

// ErrUnknownHarness is returned for a harness flag the canary has no checks for.
var ErrUnknownHarness = fmt.Errorf("no canary for the harness")

// ForHarness returns the checks of a harness flag, in the order they are reported.
func ForHarness(flag string) ([]Check, error) {
	switch flag {
	case harness.FlagClaude:
		return claudeChecks(), nil
	case harness.FlagCodex:
		return codexChecks(), nil
	default:
		return nil, fmt.Errorf("%w %q", ErrUnknownHarness, flag)
	}
}

// Evaluate runs every check on the evidence and collects the verdicts.
func Evaluate(flag string, found evidence.Evidence) (verdict.Report, error) {
	set, err := ForHarness(flag)
	if err != nil {
		return verdict.Report{}, err
	}
	report := verdict.Report{Harness: flag, Verdicts: make([]verdict.Verdict, 0, len(set))}
	for _, check := range set {
		report.Verdicts = append(report.Verdicts, check.Evaluate(found).Named(check.Name(), check.Proves()))
	}
	return report, nil
}

// funcCheck is a Check backed by a function.
type funcCheck struct {
	name, proves string
	evaluate     func(evidence.Evidence) verdict.Verdict
}

func (c funcCheck) Name() string   { return c.name }
func (c funcCheck) Proves() string { return c.proves }
func (c funcCheck) Evaluate(found evidence.Evidence) verdict.Verdict {
	return c.evaluate(found)
}

func newCheck(name, proves string, evaluate func(evidence.Evidence) verdict.Verdict) Check {
	return funcCheck{name: name, proves: proves, evaluate: evaluate}
}

// NoCallNames lists the checks whose evidence is gathered before the session and
// makes no model call.
func NoCallNames() []string {
	return []string{checkCLIVersion, checkClaudePluginsValid, checkCapWithoutHooks, checkServersStart}
}
