package runs

import (
	"context"
	"slices"
	"strings"

	canary "github.com/RachidChabane/musthold/tools/canary/internal/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Stamper timestamps an entry of the deviation log (PREREG §12): it stores the
// entry's line with an RFC 3161 token of it. The log-deviation verb provides it;
// the log calls it for the kinds that are timestamped, once the line is written.
type Stamper interface {
	Stamp(ctx context.Context, entry study.Deviation, line string) error
}

// DeviationLog appends the pipeline's entries to the work directory's
// DEVIATIONS.md.
type DeviationLog struct {
	Files   canary.Files
	Path    string
	Stamper Stamper
}

// Log appends one entry, writing the log's title first when the log is new, and
// hands an entry of a timestamped kind to the stamper.
func (d DeviationLog) Log(ctx context.Context, entry study.Deviation) error {
	line, err := entry.Line()
	if err != nil {
		return err
	}
	current, err := d.Files.Read(d.Path)
	if err != nil {
		return err
	}
	text := string(current)
	if strings.TrimSpace(text) == "" {
		text = study.DeviationsTitle + "\n\n"
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if err := d.Files.Write(d.Path, []byte(text+line+"\n"), false); err != nil {
		return err
	}
	if !entry.Kind.Timestamped() {
		return nil
	}
	return d.Stamper.Stamp(ctx, entry, line)
}

// Logged reports whether the log holds an entry of the kind for the scope.
func (d DeviationLog) Logged(kind study.Kind, scope string) (bool, error) {
	current, err := d.Files.Read(d.Path)
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
