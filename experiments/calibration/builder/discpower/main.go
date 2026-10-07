// Command calibration-build renders the closed screen requests that the discpower module sends to a System One
// endpoint, for the calibration experiment, and applies the module's own reading of an answer and of a
// threshold entry:
//
//	build            one request per mutant: the module's Shallnot gateway, Scrub, canonical edit order and
//	                 prompt.BuildScreen, then the host protocol's rendering of the closed set;
//	read             prompt.ParseScreen and power.ViolationProbability over answer documents;
//	check-threshold  format.ThresholdOverride applied with ApplyTo to a default, resolved with For;
//	defaults         the module's default thresholds and template digests.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/process"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/shallnot"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/prompt"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

const (
	commandBuild          = "build"
	commandRead           = "read"
	commandCheckThreshold = "check-threshold"
	commandDefaults       = "defaults"
	exitUsage             = 2
	exitFailure           = 1
	listSeparator         = ","
)

// mutantInput is the input the harness writes for one mutant.
type mutantInput struct {
	MutantID  string          `json:"mutant_id"`
	Edits     []editInput     `json:"edits"`
	Witnesses []power.Witness `json:"witnesses"`
}

type editInput struct {
	File     string `json:"file"`
	Qualname string `json:"qualname"`
	Before   string `json:"before"`
	After    string `json:"after"`
}

type buildInput struct {
	Mutants []mutantInput `json:"mutants"`
}

type itemOutput struct {
	MutantID       string            `json:"mutant_id"`
	Closed         string            `json:"closed"`
	TemplateDigest string            `json:"template_digest"`
	QuestionToRef  map[string]string `json:"question_to_ref"`
	Bodies         map[string]string `json:"bodies"`
}

type buildOutput struct {
	TemplateDigest string       `json:"template_digest"`
	Refs           []string     `json:"refs"`
	Statements     []string     `json:"statements"`
	Items          []itemOutput `json:"items"`
}

type readRequest struct {
	Key      string          `json:"key"`
	Document json.RawMessage `json:"document"`
	Question string          `json:"question"`
}

type readResult struct {
	Key              string  `json:"key"`
	Error            string  `json:"error,omitempty"`
	Choice           string  `json:"choice,omitempty"`
	HasProbability   bool    `json:"has_probability"`
	ViolatesReturned float64 `json:"violates_returned"`
	ViolationP       float64 `json:"violation_probability"`
}

type thresholdOutput struct {
	Default  float64            `json:"default"`
	BySource map[string]float64 `json:"by_source"`
	Resolved map[string]float64 `json:"resolved"`
}

type defaultsOutput struct {
	TauScreen       float64           `json:"tau_screen"`
	TauJudge        float64           `json:"tau_judge"`
	DecideK         int               `json:"decide_k"`
	TemplateDigests map[string]string `json:"template_digests"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: calibration-build build|read|check-threshold|defaults")
		os.Exit(exitUsage)
	}
	var err error
	switch os.Args[1] {
	case commandBuild:
		err = runBuild(os.Args[2:], os.Stdin, os.Stdout)
	case commandRead:
		err = runRead(os.Stdin, os.Stdout)
	case commandCheckThreshold:
		err = runCheckThreshold(os.Args[2:], os.Stdin, os.Stdout)
	case commandDefaults:
		err = printDefaults(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "calibration-build: unknown command %q\n", os.Args[1])
		os.Exit(exitUsage)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "calibration-build: %v\n", err)
		os.Exit(exitFailure)
	}
}

func printDefaults(out io.Writer) error {
	defaults := policy.Default()
	digests := map[string]string{}
	for kind, digest := range prompt.TemplateDigests() {
		digests[string(kind)] = digest
	}
	return writeJSON(out, defaultsOutput{
		TauScreen: defaults.TauScreen.Default, TauJudge: defaults.TauJudge.Default,
		DecideK: defaults.DecideK, TemplateDigests: digests,
	})
}

func runBuild(args []string, in io.Reader, out io.Writer) error {
	flags := flag.NewFlagSet(commandBuild, flag.ContinueOnError)
	projectDir := flags.String("project", "", "project root (a copy of the fixture)")
	shallnotBinary := flags.String("shallnot", "", "the shallnot executable the gateway runs")
	models := flags.String("models", "", "comma-separated model identifiers")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *projectDir == "" || *models == "" {
		return errors.New("--project and --models are required")
	}
	var input buildInput
	if err := json.NewDecoder(in).Decode(&input); err != nil {
		return fmt.Errorf("read the mutant inputs: %w", err)
	}
	root, err := filepath.EvalSymlinks(*projectDir)
	if err != nil {
		return fmt.Errorf("resolve the project root: %w", err)
	}
	report, err := (shallnot.Gateway{Process: process.New(), Binary: *shallnotBinary, Root: root}).Load(context.Background())
	if err != nil {
		return fmt.Errorf("load the criteria: %w", err)
	}
	refs := make([]spec.Ref, len(report.Criteria))
	for index, criterion := range report.Criteria {
		refs[index] = criterion.Ref
	}
	built := buildOutput{TemplateDigest: prompt.TemplateDigests()[hostproto.KindDecideClosed]}
	questionToRef := make(map[string]string, len(refs))
	for index, criterion := range report.Criteria {
		built.Refs = append(built.Refs, criterion.Ref.String())
		built.Statements = append(built.Statements, prompt.Scrub(criterion.Statement, refs))
		questionToRef[prompt.QuestionID(index)] = criterion.Ref.String()
	}
	identifiers := strings.Split(*models, listSeparator)
	for _, mutant := range input.Mutants {
		item, err := buildItem(built.Statements, questionToRef, mutant, identifiers)
		if err != nil {
			return err
		}
		built.Items = append(built.Items, item)
	}
	return writeJSON(out, built)
}

func buildItem(statements []string, questionToRef map[string]string, mutant mutantInput, identifiers []string) (itemOutput, error) {
	views, err := editViews(mutant)
	if err != nil {
		return itemOutput{}, fmt.Errorf("mutant %s: %w", mutant.MutantID, err)
	}
	built, err := prompt.BuildScreen(prompt.ScreenInput{Statements: statements, Edits: views, Witnesses: prompt.WitnessViews(mutant.Witnesses)})
	if err != nil {
		return itemOutput{}, fmt.Errorf("mutant %s: build the screen request: %w", mutant.MutantID, err)
	}
	set, err := hostproto.ParseClosedSet(built.Closed)
	if err != nil {
		return itemOutput{}, fmt.Errorf("mutant %s: the closed set is invalid: %w", mutant.MutantID, err)
	}
	if _, err := set.ResponseSchema(); err != nil {
		return itemOutput{}, fmt.Errorf("mutant %s: the response schema does not build: %w", mutant.MutantID, err)
	}
	item := itemOutput{
		MutantID: mutant.MutantID, Closed: string(built.Closed), TemplateDigest: built.Prompt.TemplateDigest,
		QuestionToRef: questionToRef, Bodies: make(map[string]string, len(identifiers)),
	}
	for _, identifier := range identifiers {
		body, err := set.SystemOneRequest(identifier)
		if err != nil {
			return itemOutput{}, fmt.Errorf("mutant %s: render the body for %s: %w", mutant.MutantID, identifier, err)
		}
		item.Bodies[identifier] = string(body)
	}
	return item, nil
}

// editViews orders the edits with EditSet.Canonical, as the module shows a change, and pairs each with the
// normalized definition it replaces.
func editViews(mutant mutantInput) ([]prompt.EditView, error) {
	edits := make([]mutation.Edit, len(mutant.Edits))
	before := make(map[string]string, len(mutant.Edits))
	for index, edit := range mutant.Edits {
		edits[index] = mutation.Edit{File: edit.File, Qualname: edit.Qualname, NewSource: edit.After}
		before[edit.File+"\x00"+edit.Qualname] = edit.Before
	}
	set := mutation.EditSet{Edits: edits}
	if err := set.Validate(); err != nil {
		return nil, err
	}
	var views []prompt.EditView
	for _, edit := range set.Canonical().Edits {
		views = append(views, prompt.EditView{File: edit.File, Qualname: edit.Qualname, Before: before[edit.File+"\x00"+edit.Qualname], After: edit.NewSource})
	}
	return views, nil
}

func runRead(in io.Reader, out io.Writer) error {
	var requests []readRequest
	if err := json.NewDecoder(in).Decode(&requests); err != nil {
		return fmt.Errorf("read the answer documents: %w", err)
	}
	results := make([]readResult, 0, len(requests))
	for _, request := range requests {
		results = append(results, readAnswer(request))
	}
	return writeJSON(out, results)
}

func readAnswer(request readRequest) readResult {
	result := readResult{Key: request.Key}
	answers, err := prompt.ParseScreen(request.Document)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	given, answered := answers[request.Question]
	if !answered {
		result.Error = fmt.Sprintf("the document does not answer %s", request.Question)
		return result
	}
	result.Choice = string(given.Choice)
	result.HasProbability, result.ViolatesReturned = given.HasViolates, given.Violates
	result.ViolationP = power.ViolationProbability([]power.ClosedAnswer{given})
	return result
}

func runCheckThreshold(args []string, in io.Reader, out io.Writer) error {
	flags := flag.NewFlagSet(commandCheckThreshold, flag.ContinueOnError)
	base := flags.Float64("default", -1, "the module's default threshold")
	sources := flags.String("sources", "", "comma-separated sources to resolve besides the entry's own")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *base < 0 {
		return errors.New("--default is required")
	}
	content, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	var override format.ThresholdOverride
	if err := yaml.NewDecoder(bytes.NewReader(content)).Decode(&override); err != nil {
		return fmt.Errorf("read the threshold entry: %w", err)
	}
	applied := override.ApplyTo(format.UniformThreshold(*base))
	if err := applied.Validate(); err != nil {
		return err
	}
	result := thresholdOutput{Default: applied.Default, BySource: map[string]float64{}, Resolved: map[string]float64{}}
	for source, value := range applied.BySource {
		result.BySource[string(source)] = value
		result.Resolved[string(source)] = applied.For(source)
	}
	for _, source := range strings.Split(*sources, listSeparator) {
		if source != "" {
			result.Resolved[source] = applied.For(format.ConfidenceSource(source))
		}
	}
	return writeJSON(out, result)
}

func writeJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
