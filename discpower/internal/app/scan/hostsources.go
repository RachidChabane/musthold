package scan

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/domain/promise"
	"github.com/RachidChabane/musthold/discpower/internal/domain/prompt"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
)

// Text of the sources of a change.
const (
	// titleBodySeparator joins the title and the body of a pull request or an
	// issue into the bytes its source holds, both as the forge returned them.
	titleBodySeparator = "\n\n"
	// promisesSeedTag and the scan's seed seed the extraction request.
	promisesSeedTag = "scan-promises"
	// seedJoin joins the parts of a request's seed.
	seedJoin = "|"
	// firstRound is the round of every generation-kind request of a scan but
	// the generation tickets, which count their own.
	firstRound = 1
	// contradictionUnusablePromises is the ground on which an extraction answer
	// is rejected: it names a source the request did not show or holds an
	// empty quote.
	contradictionUnusablePromises = "unusable_promises"
	// lineJoin joins the lines a request digests.
	lineJoin = "\n"
)

// quotableSource is one document a promise may be quoted from, with its exact
// bytes.
type quotableSource struct {
	name    string
	kind    promise.SourceKind
	content []byte
}

// quotableSources are the closed set of a scan's sources: the README of the
// head commit, and for `scan <PR>` the title and body of the pull request and
// of each issue it closes, byte for byte. A change's diff is never one.
func (s *Service) quotableSources(ctx context.Context, change change) ([]quotableSource, error) {
	var sources []quotableSource
	readme, err := s.deps.Host.Head.ReadAtHead(ctx, format.ScanSourceReadme)
	switch {
	case err == nil:
		sources = append(sources, quotableSource{name: format.ScanSourceReadme, kind: promise.SourceReadme, content: readme})
	case !errors.Is(err, ErrNotAtHead):
		return nil, fmt.Errorf("read the README of the head commit: %w", err)
	}
	if change.pull == nil {
		return sources, nil
	}
	pull := change.pull
	sources = append(sources, quotableSource{name: format.ScanSourceDescription, kind: promise.SourceDescription, content: titleAndBody(pull.Title, pull.Body)})
	issues := slices.Clone(pull.ClosingIssues)
	slices.SortFunc(issues, func(a, b Issue) int { return a.Number - b.Number })
	for _, issue := range issues {
		sources = append(sources, quotableSource{name: format.ScanSourceIssue(issue.Number), kind: promise.SourceIssue, content: titleAndBody(issue.Title, issue.Body)})
	}
	return sources, nil
}

func titleAndBody(title, body string) []byte { return []byte(title + titleBodySeparator + body) }

// saveSources keeps the sources under the scan's folder and reads them as the
// quote check sees them, with the digests of the saved bytes.
func (s *Service) saveSources(commit string, quotable []quotableSource) ([]promise.Source, error) {
	if len(quotable) == 0 {
		return nil, nil
	}
	named := make([]Source, len(quotable))
	for index, source := range quotable {
		named[index] = Source{Name: source.name, Content: source.content}
	}
	saved, err := s.deps.Host.Sources.Save(commit, named)
	if err != nil {
		return nil, fmt.Errorf("save the scan's sources: %w", err)
	}
	sources := make([]promise.Source, len(saved))
	for index, one := range saved {
		sources[index] = promise.Source{Name: one.Name, Kind: quotable[index].kind, Bytes: quotable[index].content, Digest: one.Digest}
	}
	return sources, nil
}

// extract asks the one extraction of the scan and reads the promises it
// gives. A scan with no quotable source asks nothing and has no promise.
func (r *hostRun) extract(ctx context.Context) (bool, error) {
	if len(r.sources) == 0 {
		r.state.checked, r.state.noSources = true, true
		return true, nil
	}
	request, err := r.extractionRequest(ctx)
	if err != nil {
		return false, err
	}
	ask := newHostAsk(request, promise.Extraction())
	answers, err := r.asker.stage([]hostAsk{ask}, nil)
	if err != nil {
		return false, err
	}
	answer := answers[0]
	switch {
	case pending(answer):
		return false, nil
	case !answered(answer):
		r.state.extractionFailed = true
		return true, nil
	}
	parsed, err := prompt.ParsePromises(answer.Votes[0].Output, r.sourceNames())
	if err != nil {
		return r.contradictVotes(ask, answer.Votes, contradictionUnusablePromises)
	}
	if err := r.validate(*answer); err != nil {
		return false, err
	}
	for _, one := range parsed {
		r.state.extracted = append(r.state.extracted, promise.Extracted{Quote: one.Quote, Source: one.Source})
	}
	r.state.checked = true
	return true, nil
}

func (r *hostRun) sourceNames() []string {
	names := make([]string, len(r.sources))
	for index, source := range r.sources {
		names[index] = source.Name
	}
	return names
}

// extractionRequest shows the sources, each under its name with its exact
// text, and for a change the paths of its changed files and the public
// signatures of its changed functions, never a hunk or a body.
func (r *hostRun) extractionRequest(ctx context.Context) (hostproto.Request, error) {
	input := prompt.PromisesInput{}
	var inputs []hostproto.Input
	for _, source := range r.sources {
		input.Sources = append(input.Sources, prompt.PromiseSource{Name: source.Name, Text: string(source.Bytes)})
		inputs = append(inputs, hostproto.Input{Role: format.RoleSpec, Digest: source.Digest})
	}
	if r.change.base != "" {
		touched, err := r.service.touched(ctx, r.change.base)
		if err != nil {
			return hostproto.Request{}, err
		}
		signatures, err := r.service.signaturesOf(touched)
		if err != nil {
			return hostproto.Request{}, err
		}
		input.ChangedPaths, input.Signatures = touched.paths, signatures
		inputs = append(inputs,
			hostproto.Input{Role: format.RoleCodeTree, Digest: digestLines(input.ChangedPaths)},
			hostproto.Input{Role: format.RoleCodeTree, Digest: digestLines(input.Signatures)},
		)
	}
	return r.assemble(hostDraft{kind: hostproto.KindGeneratePromises, built: prompt.BuildPromises(input), inputs: inputs,
		round: firstRound, seed: digestLines([]string{promisesSeedTag, strconv.FormatUint(r.request.Seed, seedBase)})})
}

// touchedChange is what a change touches: the paths of its files and the names
// of the definitions whose lines it changes, in the files a scan may mutate.
type touchedChange struct {
	paths []string
	names []string
	files map[string][]byte
}

// touched reads what the change since the base touches.
func (s *Service) touched(ctx context.Context, base string) (touchedChange, error) {
	changed, err := s.deps.Source.ChangedLines(ctx, base)
	if err != nil {
		return touchedChange{}, fmt.Errorf("read the changed lines: %w", err)
	}
	touched := touchedChange{paths: slices.Sorted(maps.Keys(changed)), files: map[string][]byte{}}
	layout := s.scanLayout()
	for _, file := range touched.paths {
		content, err := readProjectFile(s.deps.Project, file)
		if err != nil || !mutableSource(file, content, layout, s.deps.Engines) {
			continue
		}
		touched.files[file] = content
		definitions, err := s.deps.Ecosystem.Locator().Locate(file, content)
		if err != nil {
			continue
		}
		for _, definition := range definitions {
			if spansAny(content, definition.Span, changed[file]) {
				touched.names = append(touched.names, strings.Split(definition.Qualname, qualnameSeparator)...)
			}
		}
	}
	slices.Sort(touched.names)
	touched.names = slices.Compact(touched.names)
	return touched, nil
}

// signaturesOf are the public signatures of the changed functions, as the
// ecosystem's witness host writes them; none without a witness host.
func (s *Service) signaturesOf(touched touchedChange) ([]string, error) {
	if s.deps.WitnessHost == nil || len(touched.names) == 0 {
		return nil, nil
	}
	brief, err := s.deps.WitnessHost.Brief(s.deps.Project, sourceFilesOf(touched.files), strings.Join(touched.names, nameJoin))
	if err != nil {
		return nil, fmt.Errorf("read the signatures of the changed functions: %w", err)
	}
	return brief.Interface, nil
}

// digestLines digests lines as one text.
func digestLines(lines []string) string {
	return format.SHA256Hex([]byte(strings.Join(lines, lineJoin)))
}
