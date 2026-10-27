package deviation

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Exit codes of the verb.
const (
	ExitOK     = 0
	ExitFailed = 1
	ExitUsage  = 2
)

const scopeSeparator = ","

// Main is the log-deviation verb (PREREG §12): it appends an operator's entry to
// the deviation log, and timestamps it when its kind requires.
func Main(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return run(ctx, args, stdout, stderr, DefaultStamper())
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, stamper Stamping) int {
	flags := flag.NewFlagSet("log-deviation", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.String("repo", ".", "repository root")
	flags.String("config", "", "run configuration")
	work := flags.String("work", study.DefaultWork(), "work directory")
	kind := flags.String("kind", string(study.KindProtocol), "kind of the deviation")
	scope := flags.String("scope", "", "none, all, or run or task names separated by commas")
	section := flags.String("section", "", "section of the protocol, without its sign")
	says := flags.String("says", "", "what the protocol says")
	happened := flags.String("happened", "", "what happened")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	entry := study.Deviation{
		Kind:     study.Kind(*kind),
		Scope:    splitScope(*scope),
		Section:  strings.TrimPrefix(*section, "§"),
		Says:     *says,
		Happened: *happened,
	}
	layout := study.Layout{Root: *work}
	appender := NewAppender(layout, stamper, func() time.Time { return time.Now().UTC() })
	err := appender.Append(ctx, entry)
	switch {
	case errors.Is(err, study.ErrDeviation):
		fmt.Fprintln(stderr, "log-deviation:", err)
		return ExitUsage
	case err != nil:
		fmt.Fprintln(stderr, "log-deviation:", err)
		return ExitFailed
	}
	fmt.Fprintln(stdout, "logged in", layout.Deviations())
	return ExitOK
}

func splitScope(scope string) []string {
	var names []string
	for _, name := range strings.Split(scope, scopeSeparator) {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	return names
}
