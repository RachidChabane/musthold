// Package verbs is the composition root of the loop-2 study's analysis verbs:
// it names each verb and wires it to its adapters (the records directories,
// openssl for the timestamp tokens, and experiments/loop-2/script/power for the
// reach). It holds no rule of its own.
package verbs

import (
	"slices"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/stamp"
	l1verbs "github.com/RachidChabane/musthold/tools/canary/internal/loop1/verbs"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/analyze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/calibrate"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/reportlint"
)

// Main is the entry of a verb, as loop-1's verbs have it.
type Main = l1verbs.Main

// The verbs, by the subcommand each one is.
const (
	Analyze    = "analyze"
	Calibrate  = "calibrate"
	ReportLint = "report-lint"
)

var _ analyze.Loader = record.Files{}

// Table maps each verb to its entry, wired to its adapters.
func Table() map[string]Main {
	return map[string]Main{
		Analyze:    analyze.Command{Loader: record.Files{}, Imprints: stamp.OpenSSL{}}.Main,
		Calibrate:  calibrate.Command{Reacher: func(path string) calibrate.Reacher { return calibrate.PowerScript{Path: path} }}.Main,
		ReportLint: reportlint.Main,
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
