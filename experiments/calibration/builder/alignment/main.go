// Command calibration-build renders the closed requests that the alignment module sends to a System One
// endpoint, for the calibration experiment: one request per test, through the module's own Shallnot gateway,
// maskers, tokenization, tournament and prompt builder, and then through the host protocol's own rendering of
// a closed set as a `/v1/systemone` body.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/masking/golang"
	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/masking/python"
	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/process"
	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/shallnot"
	"github.com/RachidChabane/spec-proof/alignment/internal/app/ports"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/criterion"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/masking"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/prompt"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/tournament"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/verdict"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

const (
	commandBuild         = "build"
	commandDefaults      = "defaults"
	commandRender        = "render"
	commandCompareExport = "compare-export"
	exitUsage            = 2
	exitFailure          = 1
	modelSeparator       = ","
)

// buildOutput is what `build` prints.
type buildOutput struct {
	TemplateDigest string            `json:"template_digest"`
	Criteria       []criterionOutput `json:"criteria"`
	Items          []itemOutput      `json:"items"`
}

type criterionOutput struct {
	Ref             string `json:"ref"`
	StatementDigest string `json:"statement_digest"`
}

type rejection struct {
	Reason string `json:"reason"`
	Detail string `json:"detail"`
}

// itemOutput is one test. The closed set, the digest of its template and one body per model are set unless the
// masker rejected the test.
type itemOutput struct {
	Label          string            `json:"label"`
	Classname      string            `json:"classname"`
	Name           string            `json:"name"`
	File           string            `json:"file"`
	Line           int               `json:"line"`
	Declared       []string          `json:"declared"`
	Rejected       *rejection        `json:"rejected"`
	TokenToRef     map[string]string `json:"token_to_ref,omitempty"`
	Closed         string            `json:"closed,omitempty"`
	TemplateDigest string            `json:"template_digest,omitempty"`
	Bodies         map[string]string `json:"bodies,omitempty"`
}

type defaultsOutput struct {
	TauRoute        float64           `json:"tau_route"`
	ModalMin        float64           `json:"modal_min"`
	AlignK          int               `json:"align_k"`
	DecideK         int               `json:"decide_k"`
	TemplateDigests map[string]string `json:"template_digests"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: calibration-build build|compare-export --project DIR --shallnot PATH [--models a,b] | render --models a,b | defaults")
		os.Exit(exitUsage)
	}
	var err error
	switch os.Args[1] {
	case commandBuild:
		err = runBuild(os.Args[2:], os.Stdout)
	case commandDefaults:
		err = printDefaults(os.Stdout)
	case commandRender:
		err = runRender(os.Args[2:], os.Stdin, os.Stdout)
	case commandCompareExport:
		err = runCompareExport(os.Args[2:], os.Stdin, os.Stdout)
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
		TauRoute: defaults.TauRoute.Default, ModalMin: defaults.ModalMin.Default,
		AlignK: defaults.AlignK, DecideK: defaults.DecideK, TemplateDigests: digests,
	})
}

func runBuild(args []string, out io.Writer) error {
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
	built, err := buildAll(*projectDir, *shallnotBinary, strings.Split(*models, modelSeparator))
	if err != nil {
		return err
	}
	return writeJSON(out, built)
}

// buildAll loads the criteria through the gateway and builds the request of every test.
func buildAll(projectDir, shallnotBinary string, identifiers []string) (buildOutput, error) {
	root, err := filepath.EvalSymlinks(projectDir)
	if err != nil {
		return buildOutput{}, fmt.Errorf("resolve the project root: %w", err)
	}
	report, err := (shallnot.Gateway{Process: process.New(), Binary: shallnotBinary, Root: root}).Load(context.Background())
	if err != nil {
		return buildOutput{}, fmt.Errorf("load the criteria: %w", err)
	}
	built := buildOutput{TemplateDigest: prompt.TemplateDigests()[hostproto.KindDecideClosed]}
	for _, c := range report.Project.Criteria {
		built.Criteria = append(built.Criteria, criterionOutput{Ref: c.Ref.String(), StatementDigest: c.StatementDigest})
	}
	builder := itemBuilder{
		project:     report.Project,
		maskers:     []ports.Masker{python.New(), golang.New()},
		tree:        os.DirFS(root),
		identifiers: identifiers,
	}
	for _, subject := range report.Project.Subjects() {
		item, err := builder.build(subject)
		if err != nil {
			return buildOutput{}, err
		}
		built.Items = append(built.Items, item)
	}
	return built, nil
}

type renderOutput struct {
	Bodies map[string]string `json:"bodies"`
}

// runRender renders a closed set read from standard input, as the relay renders the closed member of an export.
func runRender(args []string, in io.Reader, out io.Writer) error {
	flags := flag.NewFlagSet(commandRender, flag.ContinueOnError)
	models := flags.String("models", "", "comma-separated model identifiers")
	if err := flags.Parse(args); err != nil {
		return err
	}
	closed, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	set, err := hostproto.ParseClosedSet(closed)
	if err != nil {
		return fmt.Errorf("the closed set is invalid: %w", err)
	}
	if _, err := set.ResponseSchema(); err != nil {
		return fmt.Errorf("the response schema does not build: %w", err)
	}
	rendered := renderOutput{Bodies: map[string]string{}}
	for _, identifier := range strings.Split(*models, modelSeparator) {
		body, err := set.SystemOneRequest(identifier)
		if err != nil {
			return fmt.Errorf("render the body for %s: %w", identifier, err)
		}
		rendered.Bodies[identifier] = string(body)
	}
	return writeJSON(out, rendered)
}

type exportedRequest struct {
	Ticket string          `json:"ticket"`
	Nonce  string          `json:"nonce"`
	Closed json.RawMessage `json:"closed"`
}

type compareOutput struct {
	Exported          int      `json:"exported"`
	Rebuilt           int      `json:"rebuilt"`
	Matched           int      `json:"matched"`
	UnmatchedExported []string `json:"unmatched_exported"`
	UnmatchedRebuilt  []string `json:"unmatched_rebuilt"`
}

// runCompareExport rebuilds the closed sets of an `awaiting_host` export from the same inputs and pairs them with
// the exported ones, byte for byte once insignificant whitespace is removed.
func runCompareExport(args []string, in io.Reader, out io.Writer) error {
	flags := flag.NewFlagSet(commandCompareExport, flag.ContinueOnError)
	projectDir := flags.String("project", "", "project root the export was produced from")
	shallnotBinary := flags.String("shallnot", "", "the shallnot executable the gateway runs")
	if err := flags.Parse(args); err != nil {
		return err
	}
	var exported []exportedRequest
	if err := json.NewDecoder(in).Decode(&exported); err != nil {
		return fmt.Errorf("read the export: %w", err)
	}
	built, err := buildAll(*projectDir, *shallnotBinary, nil)
	if err != nil {
		return err
	}
	rebuilt := map[string][]string{}
	result := compareOutput{Exported: len(exported), UnmatchedExported: []string{}, UnmatchedRebuilt: []string{}}
	for _, item := range built.Items {
		if item.Rejected == nil {
			rebuilt[item.Closed] = append(rebuilt[item.Closed], item.Label)
			result.Rebuilt++
		}
	}
	for _, request := range exported {
		var compact bytes.Buffer
		if err := json.Compact(&compact, request.Closed); err != nil {
			return fmt.Errorf("export %s: %w", request.Ticket, err)
		}
		if labels := rebuilt[compact.String()]; len(labels) > 0 {
			rebuilt[compact.String()] = labels[1:]
			result.Matched++
			continue
		}
		result.UnmatchedExported = append(result.UnmatchedExported, request.Ticket)
	}
	for _, labels := range rebuilt {
		result.UnmatchedRebuilt = append(result.UnmatchedRebuilt, labels...)
	}
	return writeJSON(out, result)
}

// itemBuilder builds the request of a test the way the module's prove pass does.
type itemBuilder struct {
	project     criterion.Project
	maskers     []ports.Masker
	tree        fs.FS
	identifiers []string
}

func (b itemBuilder) build(subject criterion.Subject) (itemOutput, error) {
	test := subject.Test
	item := itemOutput{
		Label: test.Label(), Classname: test.Classname, Name: test.Name, File: test.File, Line: test.Line,
		Declared: refNames(subject.Declared),
	}
	input, failure := b.input(test)
	if failure != nil {
		item.Rejected = failure
		return item, nil
	}
	item.TokenToRef = make(map[string]string, len(input.Criteria.Refs))
	for token, ref := range input.Criteria.Refs {
		item.TokenToRef[token] = ref.String()
	}
	groups := tournament.Groups(input.Criteria.Tokens)
	if len(groups) != 1 {
		return item, fmt.Errorf("%s: %d criteria form %d groups, the experiment asks one final round", item.Label, len(input.Criteria.Tokens), len(groups))
	}
	closed, err := prompt.BuildClosed(prompt.ClosedInput{Test: input, Group: groups[0], Final: true})
	if err != nil {
		return item, fmt.Errorf("%s: build the closed request: %w", item.Label, err)
	}
	item.Closed, item.TemplateDigest = string(closed.Closed), closed.Prompt.TemplateDigest
	set, err := hostproto.ParseClosedSet(closed.Closed)
	if err != nil {
		return item, fmt.Errorf("%s: the closed set is invalid: %w", item.Label, err)
	}
	if _, err := set.ResponseSchema(); err != nil {
		return item, fmt.Errorf("%s: the response schema does not build: %w", item.Label, err)
	}
	item.Bodies = make(map[string]string, len(b.identifiers))
	for _, identifier := range b.identifiers {
		body, err := set.SystemOneRequest(identifier)
		if err != nil {
			return item, fmt.Errorf("%s: render the body for %s: %w", item.Label, identifier, err)
		}
		item.Bodies[identifier] = string(body)
	}
	return item, nil
}

// input masks the test and shows it, with every criterion under a token: the pass state's buildInput.
func (b itemBuilder) input(test criterion.BoundTest) (prompt.TestInput, *rejection) {
	masker, found := b.maskerFor(test.File)
	if !found {
		return prompt.TestInput{}, &rejection{Reason: verdict.ReasonUnreadableSource, Detail: ports.ErrUnsupportedFile.Error() + ": " + test.File}
	}
	masked, err := masker.Mask(b.tree, test)
	switch {
	case errors.Is(err, masking.ErrLeak):
		return prompt.TestInput{}, &rejection{Reason: verdict.ReasonMaskedInputLeaks, Detail: err.Error()}
	case err != nil:
		return prompt.TestInput{}, &rejection{Reason: verdict.ReasonUnreadableSource, Detail: err.Error()}
	}
	refs := b.project.Refs()
	for _, shown := range []string{masked.Unit, masked.Signatures} {
		if err := masking.Ensure(shown, test.Name, refs); err != nil {
			return prompt.TestInput{}, &rejection{Reason: verdict.ReasonMaskedInputLeaks, Detail: err.Error()}
		}
	}
	tokenized := masking.Tokenize(b.project.Criteria, test.Label())
	return prompt.TestInput{Criteria: tokenized, Unit: masked.Unit, Signatures: masked.Signatures}, nil
}

func (b itemBuilder) maskerFor(file string) (ports.Masker, bool) {
	for _, masker := range b.maskers {
		if masker.Handles(file) {
			return masker, true
		}
	}
	return nil, false
}

func refNames(refs []criterion.Ref) []string {
	names := make([]string, len(refs))
	for index, ref := range refs {
		names[index] = ref.String()
	}
	return names
}

func writeJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
