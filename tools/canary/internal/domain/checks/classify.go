package checks

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
)

// checkClaudeClassifyWitness is the name of the check of the raw API log.
const checkClaudeClassifyWitness = "classify_witness_recorded"

const classifyWitnessProves = "After a model-free `prove --resume`, the raw API log of the session " +
	"(OTEL_LOG_RAW_API_BODIES=file:<dir>) holds the classifier calls the mod made for a closed request, " +
	"inside the window of its ticket and with the labels it returned, so that the module's own check " +
	"recognises them: the version of the host writes classifier requests under the query source the check reads, " +
	"and its request and response bodies have the shape the check rebuilds. It records the query source observed. " +
	"It does not show that a forged label would be refused."

// classifyWitnessCheck reads what the module journaled about the classifications
// the mod made in shadow during the session. A classification the raw API log
// holds as dictated passes; one it does not hold, though the log is not empty,
// fails; no classification, no check of the log, or an empty index is
// inconclusive and touches no other check.
func classifyWitnessCheck() Check {
	return newCheck(checkClaudeClassifyWitness, classifyWitnessProves, func(found evidence.Evidence) verdict.Verdict {
		shadows := found.Events(hostproto.EventClassifyShadow)
		if len(shadows) == 0 {
			return verdict.Undecided("the mod made no classification in shadow, so there is nothing to check against the raw API log")
		}
		observations := found.Events(hostproto.EventClassifyShadowWitnessed)
		if len(observations) == 0 {
			return verdict.Undecided("a classification was made in shadow, but the module journaled no check of the raw API log",
				evidence.JournalAt(shadows[0].Line, string(hostproto.EventClassifyShadow)))
		}
		return judgeObservations(observations)
	})
}

// judgeObservations concludes from the checks of the raw API log the module journaled.
func judgeObservations(observations []evidence.JournalEntry) verdict.Verdict {
	var raised, failed, empty []string
	var pointers []verdict.Pointer
	sources := map[string]bool{}
	for _, entry := range observations {
		witness, parsed := hostproto.ParseShadowWitness(entry.Event.Detail)
		pointer := evidence.JournalAt(entry.Line, string(hostproto.EventClassifyShadowWitnessed))
		switch {
		case !parsed:
			failed = append(failed, "an observation the check cannot read")
		case witness.Verdict == hostproto.WitnessRaised:
			raised = append(raised, witness.Model)
			for _, source := range witness.QuerySources {
				sources[source] = true
			}
		case witness.LoggedCalls == 0:
			empty = append(empty, witness.Reason)
		default:
			failed = append(failed, fmt.Sprintf("%s (%s) with %d logged calls", witness.Verdict, witness.Reason, witness.LoggedCalls))
		}
		pointers = append(pointers, pointer)
	}
	observed := strings.Join(slices.Sorted(maps.Keys(sources)), ", ")
	switch {
	case len(failed) > 0:
		return verdict.Failed("the raw API log does not hold the classifier calls as dictated: "+strings.Join(failed, "; "), pointers...)
	case len(raised) > 0:
		return verdict.Passed(fmt.Sprintf("the raw API log holds %d classifications as dictated, answered by %s; query source observed: %s",
			len(raised), raised[0], observed), pointers...)
	default:
		return verdict.Undecided("the index of the raw API log is empty: classifier requests were not logged ("+strings.Join(empty, ", ")+")", pointers...)
	}
}
