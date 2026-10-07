package projectconfig

import (
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/format"
)

// PolicyOverrides are the decision parameters a project sets; a nil member keeps
// the default.
type PolicyOverrides struct {
	TauJudge        *float64
	JudgeK          *int
	TauScreen       *float64
	DecideK         *int
	MinValid        *int
	MinSites        *int
	MaxRounds       *int
	KOp             *int
	MaxHostTickets  *int
	SignedPassBasis *policy.SignedPassBasis
	MinIsolation    *format.Isolation
}

// Apply returns the default policy with the overrides applied.
func (o PolicyOverrides) Apply() policy.Policy {
	result := policy.Default()
	setFloat(&result.TauJudge, o.TauJudge)
	setInt(&result.JudgeK, o.JudgeK)
	setFloat(&result.TauScreen, o.TauScreen)
	setInt(&result.DecideK, o.DecideK)
	setInt(&result.MinValid, o.MinValid)
	setInt(&result.MinSites, o.MinSites)
	setInt(&result.MaxRounds, o.MaxRounds)
	setInt(&result.KOp, o.KOp)
	setInt(&result.MaxHostTickets, o.MaxHostTickets)
	if o.SignedPassBasis != nil {
		result.SignedPassBasis = *o.SignedPassBasis
	}
	if o.MinIsolation != nil {
		result.MinIsolation = *o.MinIsolation
	}
	return result
}

func setFloat(target *float64, value *float64) {
	if value != nil {
		*target = *value
	}
}

func setInt(target *int, value *int) {
	if value != nil {
		*target = *value
	}
}

// languageOf names the language an ecosystem's operations are bound under.
var languageOf = map[ecosystem.Name]string{
	ecosystem.Pytest:    "python",
	ecosystem.Jest:      "typescript",
	ecosystem.Vitest:    "typescript",
	ecosystem.Maven:     "java",
	ecosystem.Gradle:    "java",
	ecosystem.GoTesting: "go",
}

// OperationsFor resolves the operation bindings of an ecosystem: each operation
// name to the target it is bound to, looked up under the ecosystem's name and,
// failing that, under its language.
func (s Settings) OperationsFor(name ecosystem.Name) map[string]string {
	bound := map[string]string{}
	for operation, targets := range s.Operations {
		if target, found := targets[string(name)]; found {
			bound[operation] = target
		} else if target, found := targets[languageOf[name]]; found {
			bound[operation] = target
		}
	}
	return bound
}

// Policy returns the decision policy of the project.
func (s Settings) Policy() policy.Policy { return s.PolicyOverrides.Apply() }
