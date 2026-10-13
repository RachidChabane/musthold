package scanreport

import (
	"fmt"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/scan"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/promise"
	"github.com/RachidChabane/spec-proof/format"
)

// Vocabulary of the promises a host established.
const (
	// ShortWitnessLength is how many characters of a witness id a report shows.
	ShortWitnessLength = 12

	notCheckedSentence = "promises not checked: the host did not answer the extraction"
	noPromiseSentence  = "no quotable promise in the README, description or closing issues"
	checkedHeading     = "promises checked:"
	noneCheckedHeading = "no promise checked"
	promiseFormat      = "%q (%s)"
	brokenFormat       = "a change that passes the whole suite %s fails a block-bearing witness written for it"
	witnessLineFormat  = "witness %s fails on it, reproduced"
	questionFormat     = "%s: witness %s, %s"
	labelJoin          = ", "

	uncheckedFormat      = "%s: %s"
	overCapFormat        = "over the cap of %d promises"
	budgetShortFormat    = "the budget of %d calls could not cover it"
	cutFormat            = "%s not asked: %s"
	unaskedFormat        = "%s left unasked: the session reached its call cap"
	droppedFormat        = "%d extracted %s not found byte for byte in their source"
	quoteNoun            = "quote"
	quotesNoun           = "quotes"
	witnessesUnavailable = "this ecosystem writes no witness"
	cutBudgetReason      = "the budget could not cover it"
	cutNotKeptReason     = "its promise is not kept"
	referenceFailsReason = "the reference fails it"
	unstableReason       = "unstable on the reference"
	extractionAsk        = "the extraction"
	labelsAsk            = "the labels ticket"
	generationAskFormat  = "generation ticket %d for %s"
	writerAskFormat      = "writer %d of %s"
	scopeAskFormat       = "scope vote %d of %s"
	otherAsk             = "an unnamed call"
	brokenRankFormat     = "#P%d"
	brokenMarkdownFormat = "P%d."
	questionPrefix       = "question: "
	uncheckedPrefix      = "unchecked: "
	promisesHeading      = "### Promises"
	brokenHeading        = "### Broken promises"
	questionsHeading     = "### Questions"
	uncheckedHeading     = "### Unchecked"
	itemPrefix           = "- "
)

// questionReasons say why a witness is a question about its promise.
var questionReasons = map[promise.QuestionReason]string{
	promise.QuestionReferenceFails: referenceFailsReason,
	promise.QuestionUnstable:       unstableReason,
}

// cutReasons say why the planner did not issue an ask.
var cutReasons = map[promise.CutReason]string{
	promise.CutBudget:         cutBudgetReason,
	promise.CutPromiseNotKept: cutNotKeptReason,
}

// ShortWitness is the witness id as a report shows it.
func ShortWitness(id string) string {
	return id[:min(len(id), ShortWitnessLength)]
}

// quoted writes a promise as its quote and the name of its source.
func quoted(one format.ScanPromise) string {
	return fmt.Sprintf(promiseFormat, one.Quote, one.Source.Name)
}

// promiseStatus says what became of the extraction: a sentence when no promise
// is listed, or a heading over the checked promises.
func promiseStatus(promises *scan.PromiseReport) (string, []string) {
	switch {
	case !promises.Checked:
		return notCheckedSentence, nil
	case promises.NoPromise():
		return noPromiseSentence, nil
	case len(promises.Promises) == 0:
		return noneCheckedHeading, nil
	}
	items := make([]string, 0, len(promises.Promises))
	for _, one := range promises.Promises {
		items = append(items, quoted(one))
	}
	return checkedHeading, items
}

// brokenTitle is the quote of a broken promise and what its change did.
func brokenTitle(broken scan.BrokenPromise) string {
	return fmt.Sprintf("%s "+brokenFormat, quoted(broken.Promise), allRuns)
}

// brokenProof says which witness fails on the change of a broken promise.
func brokenProof(broken scan.BrokenPromise) string {
	return fmt.Sprintf(witnessLineFormat, ShortWitness(broken.Witness.ID))
}

// brokenChange shows the change that breaks a promise.
func brokenChange(broken scan.BrokenPromise) string {
	return changeLine(broken.Site, broken.Files, broken.Killer.Route)
}

// questionLines write each question: its promise, its witness and why.
func questionLines(promises *scan.PromiseReport) []string {
	var lines []string
	for _, question := range promises.Questions {
		lines = append(lines, fmt.Sprintf(questionFormat, quoted(question.Promise), ShortWitness(question.Witness.ID), questionReasons[question.Reason]))
	}
	return lines
}

// uncheckedLines say what the scan left unchecked and why: the promises it did
// not check, the asks it cut or left unasked, the quotes it dropped and an
// ecosystem that writes no witness.
func uncheckedLines(promises *scan.PromiseReport) []string {
	var lines []string
	for _, unchecked := range promises.Unchecked {
		lines = append(lines, fmt.Sprintf(uncheckedFormat, quoted(unchecked.Promise), uncheckedReason(unchecked.Reason)))
	}
	labelOf := askLabeler(promises)
	for _, cut := range promises.Cut {
		lines = append(lines, fmt.Sprintf(cutFormat, labelOf(cut.Ask), cutReasons[cut.Reason]))
	}
	for _, ask := range promises.Unasked {
		lines = append(lines, fmt.Sprintf(unaskedFormat, labelOf(ask)))
	}
	if dropped := len(promises.Dropped); dropped > 0 {
		lines = append(lines, fmt.Sprintf(droppedFormat, dropped, plural(dropped, quoteNoun, quotesNoun)))
	}
	if promises.WitnessesUnavailable {
		lines = append(lines, witnessesUnavailable)
	}
	return lines
}

// uncheckedReason says why a kept promise went unchecked.
func uncheckedReason(reason promise.UncheckedReason) string {
	if reason == promise.UncheckedOverCap {
		return fmt.Sprintf(overCapFormat, promise.MaxPromises)
	}
	return fmt.Sprintf(budgetShortFormat, promise.Budget)
}

// askLabeler names an ask, with the quote of the promise it is about.
func askLabeler(promises *scan.PromiseReport) func(promise.Ask) string {
	quotes := map[string]string{}
	for _, one := range promises.Promises {
		quotes[one.ID] = fmt.Sprintf("%q", one.Quote)
	}
	for _, unchecked := range promises.Unchecked {
		quotes[unchecked.Promise.ID] = fmt.Sprintf("%q", unchecked.Promise.Quote)
	}
	return func(ask promise.Ask) string {
		about, number := quotes[ask.Promise], ask.Slot+1
		if about == "" {
			about = ask.Promise
		}
		switch ask.Category {
		case promise.CategoryExtraction:
			return extractionAsk
		case promise.CategoryLabels:
			return labelsAsk
		case promise.CategoryGeneration:
			return fmt.Sprintf(generationAskFormat, number, about)
		case promise.CategoryWriter:
			return fmt.Sprintf(writerAskFormat, number, about)
		case promise.CategoryScope:
			return fmt.Sprintf(scopeAskFormat, number, about)
		}
		return otherAsk
	}
}

// labelsOf are the quotes of the promises that label the test of a finding, in
// the order of the statement, or nil when it has none or the scan had no host.
func labelsOf(report scan.Report, finding scan.Finding) []string {
	if report.Promises == nil {
		return nil
	}
	predicate := report.Statement.Predicate
	quotes := map[string]string{}
	for _, one := range predicate.Promises {
		quotes[one.ID] = fmt.Sprintf("%q", one.Quote)
	}
	var labels []string
	for _, test := range predicate.Tests {
		if test.ID != string(finding.Test) {
			continue
		}
		for _, id := range test.Promises {
			if quote, known := quotes[id]; known {
				labels = append(labels, quote)
			}
		}
	}
	return labels
}

// mayBreak lists the promises a finding may break, or "" when none labels it.
func mayBreak(report scan.Report, finding scan.Finding) string {
	return strings.Join(labelsOf(report, finding), labelJoin)
}
