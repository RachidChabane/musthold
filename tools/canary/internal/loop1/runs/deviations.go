package runs

import (
	"context"
	"slices"
	"strings"

	canary "github.com/RachidChabane/musthold/tools/canary/internal/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// DeviationWriter appends an entry to the work directory's deviation log and
// timestamps the kinds the protocol timestamps (PREREG §12); the log-deviation
// verb's appender is one.
type DeviationWriter interface {
	Append(ctx context.Context, entry study.Deviation) error
}

// logged reports whether the deviation log holds an entry of the kind for the
// scope.
func logged(files canary.Files, layout study.Layout, kind study.Kind, scope string) (bool, error) {
	current, err := files.Read(layout.Deviations())
	if err != nil {
		return false, err
	}
	for _, entry := range study.ParseDeviations(string(current)) {
		if entry.Kind == kind && slices.Contains(entry.Scope, scope) {
			return true, nil
		}
	}
	return false, nil
}

// entryOf is an entry of the deviation log for one run.
func entryOf(kind study.Kind, scope, section, says, happened string) study.Deviation {
	return study.Deviation{Kind: kind, Scope: []string{scope}, Section: section, Says: says, Happened: oneLine(happened)}
}

// oneLine keeps what happened on one line of the log.
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// The protocol's sections the pipeline's entries cite, and what they say.
const (
	sectionQuota      = "6.5"
	sectionOutcomes   = "6.4"
	sectionProvenance = "2"
	sectionToken      = "7.5"
	saysQuota         = "a quota stop voids the attempt and the run is rerun"
	saysHarness       = "a harness error voids the attempt and the run is rerun"
	saysBlocked       = "a run whose attempts end void is blocked after the cause of its last attempt"
	saysProvenance    = "a recorded version or model equals inputs.json"
	saysToken         = "no record holds the token"
)
