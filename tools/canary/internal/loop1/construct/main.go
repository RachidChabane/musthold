package construct

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
)

// ExitHarnessFailure is the exit status of a run that stopped because a call
// failed on the harness side (sign-in, quota, network, time limit). Nothing was
// repaired or dropped; running the verb again resumes at the same call.
const ExitHarnessFailure = 3

// Command is the construct verb; Wire builds the adapters of acceptance.
type Command struct {
	Wire accept.WireFunc
}

// Main runs the construct verb: it builds the fixed task and the domains in
// order until enough of them pass acceptance, printing its progress and the
// domains it dropped. It returns 0 when construction is complete, 1 when it
// stopped or ran out of domains, 2 for a usage error, and ExitHarnessFailure
// when a call failed on the harness side.
func (c Command) Main(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("construct", flag.ContinueOnError)
	set.SetOutput(stderr)
	flags := accept.RegisterFlags(set)
	if err := set.Parse(args); err != nil || set.NArg() > 0 {
		return accept.ExitUsage
	}
	wiring, err := c.Wire(*flags)
	if err != nil {
		fmt.Fprintf(stderr, "construct: %v\n", err)
		return accept.ExitFailure
	}
	model := CodexModel{Process: ExecProcess{}, Now: time.Now}
	constructor, err := NewConstructor(*flags, wiring, model, stdout)
	if err != nil {
		fmt.Fprintf(stderr, "construct: %v\n", err)
		return accept.ExitFailure
	}
	summary, err := constructor.Run(ctx)
	for _, drop := range summary.Dropped {
		fmt.Fprintf(stdout, "dropped %s at %s: %s\n", drop.Domain, drop.Check, drop.Reason)
	}
	if err != nil {
		return reportStop(stderr, err)
	}
	fmt.Fprintf(stdout, "accepted %d domains: %v\n", len(summary.Accepted), summary.Accepted)
	return accept.ExitOK
}

// reportStop prints why construction stopped and returns the exit status.
func reportStop(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "construct: %v\n", err)
	switch {
	case errors.Is(err, ErrHarnessFailure):
		fmt.Fprintln(stderr, "construct: the harness failed, not the model; no repair was used and nothing was dropped; run construct again to resume at the same call")
		return ExitHarnessFailure
	case errors.Is(err, ErrFixtureAmendment):
		fmt.Fprintln(stderr, "construct: the change is an amendment of the protocol; no task was repaired by hand")
	}
	return accept.ExitFailure
}
