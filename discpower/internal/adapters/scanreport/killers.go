package scanreport

import (
	"fmt"
	"path"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/scan"
	"github.com/RachidChabane/spec-proof/format"
)

// Vocabulary of the killers a folder offers.
const (
	killersFile    = "killers.md"
	brokenScript   = "killer.sh"
	killerFormat   = "killer-%d.sh"
	witnessDirName = "witness"

	killersTitle       = "# Tests that fail on this mutant"
	killerHeadingForm  = "## %d. %s"
	killedByTestForm   = "A test that fails on this mutant: `%s`, which fails %s. Run it with `sh %s`."
	witnessKillerForm  = "A test that fails on this mutant: witness %s, in `%s`. Run it with `sh %s`."
	differenceKillForm = "A test that fails on this mutant: a differential run of %s, input %s, reference %s, mutant %s. Re-run it with `spec-proof scan replay %s`."
	killerScriptNote   = "# Runs %s on %s with mutant.patch applied; the exit status is non-zero when the test fails on the mutant."
	killedByTestLabel  = "the test"
	witnessLabel       = "witness %s"
	differenceLabel    = "the differential run"
)

// scriptNamer names the script of the killer at an index, from 0.
type scriptNamer func(index int) string

// findingScript names the scripts of a finding folder: one per killer, by its
// place among the killers from 1.
func findingScript(index int) string { return fmt.Sprintf(killerFormat, index+1) }

// brokenScriptName names the one script of a broken-promise folder.
func brokenScriptName(int) string { return brokenScript }

// killerFiles lays out the killers of a folder: the file that lists them, a
// script for each that has a command and the witness files the scripts copy.
func killerFiles(report scan.Report, folder string, killers []scan.Killer, scriptOf scriptNamer) []File {
	if len(killers) == 0 {
		return nil
	}
	files := []File{{Path: path.Join(folder, killersFile), Content: []byte(killersDocument(report, folder, killers, scriptOf))}}
	written := map[string]bool{}
	for index, killer := range killers {
		if len(killer.Command) == 0 {
			continue
		}
		files = append(files, File{Path: path.Join(folder, scriptOf(index)), Content: []byte(killerScript(report, killer)), Executable: true})
		if killer.WitnessFile == nil || written[killer.WitnessFile.Path] {
			continue
		}
		written[killer.WitnessFile.Path] = true
		files = append(files, File{Path: path.Join(folder, witnessDirName, killer.WitnessFile.Path), Content: killer.WitnessFile.Content})
	}
	return files
}

// killersDocument lists each killer: what fails on the mutant and how to run
// it.
func killersDocument(report scan.Report, folder string, killers []scan.Killer, scriptOf scriptNamer) string {
	lines := []string{killersTitle}
	for index, killer := range killers {
		lines = append(lines, "", fmt.Sprintf(killerHeadingForm, index+1, killer.Route.Kind), "", killerSentence(killer, folder, scriptOf(index)))
	}
	return strings.Join(append(lines, ""), newline)
}

// killerSentence says which test fails on the mutant and how to run it.
func killerSentence(killer scan.Killer, folder, script string) string {
	route := killer.Route
	switch route.Kind {
	case format.ScanRouteKilledByTest:
		return fmt.Sprintf(killedByTestForm, route.Test, allRuns, script)
	case format.ScanRouteDifferential:
		return fmt.Sprintf(differenceKillForm, route.Operation,
			renderInput(route.Input), renderObservation(route.Reference), renderObservation(route.Mutant), folder)
	}
	return fmt.Sprintf(witnessKillerForm, ShortWitness(route.Witness), witnessPath(killer), script)
}

// witnessPath is where a killer's witness file sits in its folder.
func witnessPath(killer scan.Killer) string {
	if killer.WitnessFile == nil {
		return witnessDirName
	}
	return path.Join(witnessDirName, killer.WitnessFile.Path)
}

// killerScript checks the scanned commit out into a temporary directory,
// applies the patch, puts the witness file in place when the killer is a
// witness and runs the killing test.
func killerScript(report scan.Report, killer scan.Killer) string {
	lines := []string{shebang, fmt.Sprintf(killerScriptNote, killerLabel(killer.Route), ShortCommit(report.Commit))}
	lines = append(lines, checkoutLines(report)...)
	if killer.WitnessFile != nil {
		target := killer.WitnessFile.Path
		lines = append(lines,
			fmt.Sprintf(`mkdir -p "$(dirname %s)"`, quote(target)),
			fmt.Sprintf(`cp "$finding"/%s %s`, quote(path.Join(witnessDirName, target)), quote(target)))
	}
	return strings.Join(append(lines, commandLine(killer.Command), ""), newline)
}

// killerLabel names the test a killer script runs.
func killerLabel(route format.ScanRoute) string {
	switch route.Kind {
	case format.ScanRouteKilledByTest:
		return killedByTestLabel
	case format.ScanRouteWitness:
		return fmt.Sprintf(witnessLabel, ShortWitness(route.Witness))
	}
	return differenceLabel
}

// commandLine writes a command with each argument quoted for the shell.
func commandLine(command []string) string {
	quoted := make([]string, 0, len(command))
	for _, argument := range command {
		quoted = append(quoted, quote(argument))
	}
	return strings.Join(quoted, " ")
}
