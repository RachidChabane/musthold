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

// Main runs the construct verb: it builds the fixed task and the domains in
// order until enough of them pass acceptance, printing its progress and the
// domains it dropped. It returns 0 when construction is complete, 1 when it
// stopped or ran out of domains, and 2 for a usage error.
func Main(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("construct", flag.ContinueOnError)
	set.SetOutput(stderr)
	flags := accept.RegisterFlags(set)
	if err := set.Parse(args); err != nil || set.NArg() > 0 {
		return accept.ExitUsage
	}
	if accept.Wire == nil {
		fmt.Fprintf(stderr, "construct: %v\n", accept.ErrNotWired)
		return accept.ExitFailure
	}
	wiring, err := accept.Wire(flags.Repo, flags.Layout())
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
		fmt.Fprintf(stderr, "construct: %v\n", err)
		if errors.Is(err, ErrFixtureAmendment) {
			fmt.Fprintln(stderr, "construct: the change is an amendment of the protocol; no task was repaired by hand")
		}
		return accept.ExitFailure
	}
	fmt.Fprintf(stdout, "accepted %d domains: %v\n", len(summary.Accepted), summary.Accepted)
	return accept.ExitOK
}
