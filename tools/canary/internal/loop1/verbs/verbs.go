// Package verbs is the composition root of the loop-1 study's verbs: it names
// each verb as experiments/loop-1/script/<verb> runs it, and wires each to its
// adapters (the study's docker runner, the run-phase rule of analyze/phase, the
// equipping of a run, the canary's last output and the canary's detectors). It
// holds no rule of its own.
package verbs

import (
	"context"
	"io"
	"slices"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/canary"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/detect"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/deviation"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/freeze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/oracle"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/reportlint"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/runs"
)

// Main is the entry of a verb: it returns 0 on success, 1 on a refused or failed
// check, and 2 on a usage error.
type Main func(ctx context.Context, args []string, stdout, stderr io.Writer) int

// The verbs, by the subcommand of the runner each one is.
const (
	Construct    = "construct"
	Accept       = "accept"
	Canary       = "canary"
	Resolve      = "resolve"
	FreezeCheck  = "freeze-check"
	Run          = "run"
	Oracle       = "oracle"
	Detect       = "detect"
	Analyze      = "analyze"
	ReportLint   = "report-lint"
	LogDeviation = "log-deviation"
)

// Table maps each verb to its entry, wired to the study's adapters.
func Table() map[string]Main {
	return map[string]Main{
		Construct:    construct.Command{Wire: wireAccept}.Main,
		Accept:       accept.Command{Wire: wireAccept}.Main,
		Canary:       canary.Command{Detectors: canaryDetectors}.Main,
		Resolve:      freeze.ResolveCommand{Wire: wireResolve}.Main,
		FreezeCheck:  freeze.CheckMain,
		Run:          runs.Command{}.Main,
		Oracle:       oracle.Command{Wire: wireOracle}.Main,
		Detect:       detect.Command{Wire: wireDetect}.Main,
		Analyze:      analyze.Main,
		ReportLint:   reportlint.Main,
		LogDeviation: deviation.Main,
	}
}

// Names lists the verbs, sorted.
func Names() []string {
	table := Table()
	names := make([]string, 0, len(table))
	for name := range table {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// Lookup returns the entry of a verb.
func Lookup(name string) (Main, bool) {
	main, found := Table()[name]
	return main, found
}
