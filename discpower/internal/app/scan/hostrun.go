package scan

import (
	"context"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/hostrequest"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/operatorgen"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/footprint"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/promise"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/prompt"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/scanscope"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostbridge"
)

// Names in the code a scan shows its host.
const (
	// qualnameSeparator separates the names of a qualified name.
	qualnameSeparator = "."
	// nameJoin joins the names a brief is asked about.
	nameJoin = " "
	// unitKeySeparator joins the file and the qualified name of a unit.
	unitKeySeparator = "::"
)

// ViewSourceCap bounds the code a generation or labels request shows: the
// most bytes of unit source one view holds, whatever the witnesses run. A view
// keeps its units whole, in the order of the code, and leaves out each unit
// that would take it past the bound; the report names every view the bound
// cut and the request says how many units it leaves out.
const ViewSourceCap = 48 << 10

// boundedView keeps the units of a view within ViewSourceCap and counts the
// units it leaves out.
func boundedView(units []viewUnit) ([]viewUnit, int) {
	kept := make([]viewUnit, 0, len(units))
	size := 0
	for _, unit := range units {
		if size+len(unit.source) > ViewSourceCap {
			continue
		}
		size += len(unit.source)
		kept = append(kept, unit)
	}
	return kept, len(units) - len(kept)
}

// hostRun is one invocation of a scan that asks its host: what it is bound
// to, the bridge of its session and what its stages established.
type hostRun struct {
	service *Service
	request Request
	commit  string
	tree    string
	change  change
	sources []promise.Source
	bridge  *hostbridge.Bridge
	asker   *asker
	state   *promiseState
	// built is the reference workspace, built once in the invocation by the
	// first stage that needs it, and builtAt when its copy began.
	built   *reference
	builtAt time.Time
	// code holds the source files and their units, read once.
	code *codeIndex
	// answers are the answers the scan validated, which its statement uses.
	answers []hostbridge.Answer
}

// validate marks an answer validated, execution having found nothing against
// it, and keeps it for its consumption once the statement is written.
func (r *hostRun) validate(answer hostbridge.Answer) error {
	if err := r.bridge.Validate(answer); err != nil {
		return err
	}
	r.answers = append(r.answers, answer)
	return nil
}

// consume records that the statement used the validated answers.
func (r *hostRun) consume() error {
	for _, answer := range r.answers {
		if err := r.bridge.Consume(answer); err != nil {
			return fmt.Errorf("consume the answer to %s: %w", answer.ID, err)
		}
	}
	return nil
}

// release removes the reference workspace, when one was built.
func (r *hostRun) release() {
	if r.built != nil {
		_ = r.service.deps.Workspaces.Release(r.built.workspace)
	}
}

// reference copies and builds the reference once: the stages that run
// witnesses and the final stage share it. Building runs no test.
func (r *hostRun) reference(ctx context.Context) (*reference, error) {
	if r.built != nil {
		return r.built, nil
	}
	r.builtAt = r.service.deps.Clock()
	built, err := r.service.buildReference(ctx, r.request.Root)
	if err != nil {
		return nil, err
	}
	r.built = &built
	return r.built, nil
}

// identity is who a scan's requests are asked by: the module, the plugin and
// the default decision policy, whose scope threshold a scan reads.
func (r *hostRun) identity() (hostrequest.Identity, error) {
	digest, err := policy.Default().Digest()
	if err != nil {
		return hostrequest.Identity{}, fmt.Errorf("digest the decision policy: %w", err)
	}
	return hostrequest.Identity{Module: r.request.Module, Plugin: r.service.deps.Host.Plugin, PolicyDigest: digest}, nil
}

// hostDraft is a request of a scan before it is assembled.
type hostDraft struct {
	kind   hostproto.Kind
	built  prompt.Built
	inputs []hostproto.Input
	round  int
	seed   string
}

// assemble builds a request of the scan the way every request of the module
// is built.
func (r *hostRun) assemble(draft hostDraft) (hostproto.Request, error) {
	identity, err := r.identity()
	if err != nil {
		return hostproto.Request{}, err
	}
	return hostrequest.Assemble(identity, hostrequest.Draft{Kind: draft.kind, Built: draft.built, Inputs: draft.inputs, Round: draft.round, Seed: draft.seed})
}

// codeIndex is the code a scan may show its host: every source file a scan
// may mutate, by path, with its definitions and module assignments.
type codeIndex struct {
	files []mutation.SourceFile
	units []viewUnit
}

// viewUnit is one definition or module assignment of the code, with its text.
type viewUnit struct {
	file     string
	qualname string
	target   mutation.TargetKind
	source   string
	lines    []int
}

// key names the unit by file and qualified name, as a label names a function.
func (u viewUnit) key() string { return unitKey(u.file, u.qualname) }

func unitKey(file, qualname string) string { return file + unitKeySeparator + qualname }

// signature is the first line of the unit, as the labels request shows it.
func (u viewUnit) signature() string {
	first, _, _ := strings.Cut(strings.TrimSpace(u.source), lineJoin)
	return first
}

// codeOf reads the code index once.
func (r *hostRun) codeOf(ctx context.Context) (*codeIndex, error) {
	if r.code != nil {
		return r.code, nil
	}
	files, err := r.service.codeSources(ctx)
	if err != nil {
		return nil, err
	}
	index := &codeIndex{files: files}
	for _, file := range files {
		units, err := r.service.unitsOf(file)
		if err != nil {
			return nil, err
		}
		index.units = append(index.units, units...)
	}
	r.code = index
	return index, nil
}

// codeSources lists the tracked source files a scan may mutate, sorted by path.
func (s *Service) codeSources(ctx context.Context) ([]mutation.SourceFile, error) {
	tracked, err := s.deps.Source.TrackedFiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list the tracked files: %w", err)
	}
	layout := s.scanLayout()
	var files []mutation.SourceFile
	for _, path := range slices.Sorted(slices.Values(tracked)) {
		if path == outputDirectory || strings.HasPrefix(path, outputDirectory+"/") {
			continue
		}
		content, err := readProjectFile(s.deps.Project, path)
		if err != nil || !mutableSource(path, content, layout, s.deps.Engines) {
			continue
		}
		files = append(files, mutation.SourceFile{Path: path, Content: content})
	}
	return files, nil
}

// unitsOf lists the definitions and module assignments of a source file, in
// the order they appear.
func (s *Service) unitsOf(file mutation.SourceFile) ([]viewUnit, error) {
	definitions, err := s.deps.Ecosystem.Locator().Locate(file.Path, file.Content)
	if err != nil {
		return nil, fmt.Errorf("locate the definitions of %q: %w", file.Path, err)
	}
	var units []viewUnit
	add := func(definition ecosystem.Definition, target mutation.TargetKind) error {
		text, err := definition.Span.Slice(file.Content)
		if err != nil {
			return err
		}
		var lines []int
		for _, line := range scanscope.SpanLines(file.Path, file.Content, definition.Span) {
			lines = append(lines, line.Line)
		}
		units = append(units, viewUnit{file: file.Path, qualname: definition.Qualname, target: target, source: string(text), lines: lines})
		return nil
	}
	for _, definition := range definitions {
		if err := add(definition, mutation.TargetDefinition); err != nil {
			return nil, err
		}
	}
	if assignments := s.deps.Ecosystem.Assignments(); assignments != nil {
		found, err := assignments.LocateAssignments(file.Path, file.Content)
		if err != nil {
			return nil, fmt.Errorf("locate the assignments of %q: %w", file.Path, err)
		}
		for _, definition := range found {
			if err := add(definition, mutation.TargetModuleAssignment); err != nil {
				return nil, err
			}
		}
	}
	return units, nil
}

// viewOf is the code a generation request shows: the units the footprint of
// the entries holds, every unit when there is no entry, narrowed in a change
// to the units the change touches; when the footprint holds none of those, the
// touched units themselves.
func (r *hostRun) viewOf(ctx context.Context, entries []footprint.Ref) ([]viewUnit, error) {
	code, err := r.codeOf(ctx)
	if err != nil {
		return nil, err
	}
	units := code.units
	if len(entries) > 0 {
		closed, err := operatorgen.NewGenerator(r.service.deps.Engines).Footprint(code.files, entries)
		if err != nil {
			return nil, fmt.Errorf("close the footprint of the witnesses: %w", err)
		}
		units = slices.DeleteFunc(slices.Clone(units), func(unit viewUnit) bool { return !inFootprint(closed, unit) })
	}
	if r.change.base == "" {
		return units, nil
	}
	touched, err := r.touchedUnits(ctx)
	if err != nil {
		return nil, err
	}
	narrowed := slices.DeleteFunc(slices.Clone(units), func(unit viewUnit) bool { return !touched[unit.key()] })
	if len(narrowed) > 0 {
		return narrowed, nil
	}
	return slices.DeleteFunc(slices.Clone(code.units), func(unit viewUnit) bool { return !touched[unit.key()] }), nil
}

// touchedUnits names the units whose lines the change touches.
func (r *hostRun) touchedUnits(ctx context.Context) (map[string]bool, error) {
	changed, err := r.service.deps.Source.ChangedLines(ctx, r.change.base)
	if err != nil {
		return nil, fmt.Errorf("read the changed lines: %w", err)
	}
	code, err := r.codeOf(ctx)
	if err != nil {
		return nil, err
	}
	touched := map[string]bool{}
	for _, unit := range code.units {
		if slices.ContainsFunc(unit.lines, func(line int) bool { return slices.Contains(changed[unit.file], line) }) {
			touched[unit.key()] = true
		}
	}
	return touched, nil
}

func inFootprint(closed footprint.Footprint, unit viewUnit) bool {
	ref := footprint.Ref{File: unit.file, Qualname: unit.qualname}
	if unit.target == mutation.TargetModuleAssignment {
		return closed.ContainsAssignment(ref)
	}
	return closed.ContainsDefinition(ref)
}

// sliceOf shows the units as the generation prompt does.
func sliceOf(units []viewUnit) []prompt.SliceEntry {
	slice := make([]prompt.SliceEntry, len(units))
	for index, unit := range units {
		slice[index] = prompt.SliceEntry{File: unit.file, Qualname: unit.qualname, Source: unit.source}
	}
	return slice
}

// digestUnits digests the units a request shows.
func digestUnits(units []viewUnit) string {
	parts := make([]string, 0, len(units))
	for _, unit := range units {
		parts = append(parts, unit.key()+unitKeySeparator+unit.source)
	}
	return digestLines(parts)
}

// scanLayout is the ecosystem's layout with the scan's own folder excluded.
func (s *Service) scanLayout() ecosystem.Layout {
	layout := s.deps.Ecosystem.Layout()
	layout.ExcludedDirs = append(slices.Clone(layout.ExcludedDirs), outputDirectory)
	return layout
}

func readProjectFile(project fs.FS, path string) ([]byte, error) { return fs.ReadFile(project, path) }

// mutableSource reports whether a scan may mutate the file: an engine handles
// it and it is a source file the project writes.
func mutableSource(path string, content []byte, layout ecosystem.Layout, engines *operatorgen.Registry) bool {
	if _, err := engines.For(path); err != nil {
		return false
	}
	return scanscope.Mutable(path, content, layout)
}

// spansAny reports whether a span of the content touches one of the lines.
func spansAny(content []byte, span ecosystem.Span, lines []int) bool {
	for _, line := range scanscope.SpanLines("", content, span) {
		if slices.Contains(lines, line.Line) {
			return true
		}
	}
	return false
}

// sourceFilesOf lists files by path.
func sourceFilesOf(files map[string][]byte) []mutation.SourceFile {
	var sources []mutation.SourceFile
	for _, path := range slices.Sorted(maps.Keys(files)) {
		sources = append(sources, mutation.SourceFile{Path: path, Content: files[path]})
	}
	return sources
}

// scanThreshold is the confidence at which a scope vote holds a witness
// fixed: the default policy's.
func scanThreshold() float64 { return policy.Default().TauJudge.Default }
