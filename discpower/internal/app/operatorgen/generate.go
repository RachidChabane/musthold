package operatorgen

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/footprint"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/operator"
)

// ErrUnsplicable is returned when a generated mutant cannot be spliced into its
// file.
var ErrUnsplicable = errors.New("mutant does not splice")

// Request is one generation run.
type Request struct {
	Files []mutation.SourceFile
	// Entries are the entry functions of the footprint. When empty, every
	// definition and module-level assignment of Files is in scope.
	Entries []footprint.Ref
	Config  operator.Config
}

// Stats counts the stages of a run.
type Stats struct {
	Enumerated int
	Screen     operator.ScreenStats
	Screened   int
	Strata     int
	Sampled    int
	// EnumeratedByFamily counts the screened mutants per family before sampling.
	EnumeratedByFamily map[operator.Family]int
	// SampledByFamily counts the sampled mutants per family.
	SampledByFamily map[operator.Family]int
}

// Result is what a run produces. Sentinels are not part of Operator and do not
// count against K_op.
type Result struct {
	Operator  []operator.Mutant
	Sentinels []operator.Mutant
	Footprint footprint.Footprint
	Stats     Stats
}

// Generator runs the pipeline: footprint closure, enumeration, F0 and F1,
// stratified sampling, sentinels.
type Generator struct {
	registry *Registry
}

// NewGenerator returns a Generator over the registered engines.
func NewGenerator(registry *Registry) *Generator { return &Generator{registry: registry} }

// Generate runs the pipeline over the request.
func (g *Generator) Generate(request Request) (Result, error) {
	files := slices.Clone(request.Files)
	slices.SortFunc(files, func(a, b mutation.SourceFile) int { return strings.Compare(a.Path, b.Path) })

	scope, err := g.scopeOf(files, request.Entries)
	if err != nil {
		return Result{}, err
	}

	var screened []operator.Mutant
	var stats Stats
	for _, file := range files {
		if !scope.hasFile(file.Path) {
			continue
		}
		engine, err := g.registry.For(file.Path)
		if err != nil {
			return Result{}, err
		}
		candidates, err := engine.Candidates(file)
		if err != nil {
			return Result{}, err
		}
		inScope := slices.DeleteFunc(candidates, func(c operator.Candidate) bool { return !scope.allowsEdit(c.Edit) })
		stats.Enumerated += len(inScope)
		kept, screen, err := operator.Screen(inScope, engine)
		if err != nil {
			return Result{}, err
		}
		stats.Screen.Stillborn += screen.Stillborn
		stats.Screen.Trivial += screen.Trivial
		stats.Screen.Duplicate += screen.Duplicate
		screened = append(screened, kept...)
	}

	sampled := operator.Sample(screened, request.Config.KOp, request.Config.Seed)
	stats.Screened = len(screened)
	stats.Sampled = len(sampled)
	stats.Strata = countStrata(screened)
	stats.EnumeratedByFamily = countFamilies(screened)
	stats.SampledByFamily = countFamilies(sampled)

	sentinels, err := g.drawSentinels(files, scope, request.Config.Seed)
	if err != nil {
		return Result{}, err
	}
	if err := g.verifySplices(files, slices.Concat(sampled, sentinels)); err != nil {
		return Result{}, err
	}
	return Result{Operator: sampled, Sentinels: sentinels, Footprint: scope.footprint, Stats: stats}, nil
}

// verifySplices applies every mutant to its file with the production splicer,
// which re-parses the result and checks the post-splice invariant. A mutant that
// does not splice is a defect of its engine and fails the run.
func (g *Generator) verifySplices(files []mutation.SourceFile, mutants []operator.Mutant) error {
	content := make(map[string][]byte, len(files))
	for _, file := range files {
		content[file.Path] = file.Content
	}
	for _, m := range mutants {
		for _, file := range m.EditSet.Files() {
			splicer, err := g.registry.SplicerFor(file)
			if err != nil {
				return err
			}
			if _, err := splicer.SpliceAll(file, content[file], m.EditSet.EditsOf(file)); err != nil {
				return fmt.Errorf("%w: mutant %s: %w", ErrUnsplicable, m.ID, err)
			}
		}
	}
	return nil
}

func (g *Generator) drawSentinels(files []mutation.SourceFile, scope scope, seed operator.Seed) ([]operator.Mutant, error) {
	var drawn []operator.Mutant
	for _, file := range files {
		if !scope.hasFile(file.Path) {
			continue
		}
		engine, err := g.registry.For(file.Path)
		if err != nil {
			return nil, err
		}
		candidates, err := engine.SentinelCandidates(file, seed)
		if err != nil {
			return nil, err
		}
		allowed := slices.DeleteFunc(candidates, func(c operator.SentinelCandidate) bool { return !scope.allowsEdit(c.Edit) })
		mutants, err := operator.DrawSentinels(file.Path, allowed, seed)
		if err != nil {
			return nil, err
		}
		drawn = append(drawn, mutants...)
	}
	return drawn, nil
}

func countStrata(mutants []operator.Mutant) int {
	keys := map[string]bool{}
	for _, m := range mutants {
		keys[operator.StratumKey(m)] = true
	}
	return len(keys)
}

func countFamilies(mutants []operator.Mutant) map[operator.Family]int {
	counts := map[operator.Family]int{}
	for _, m := range mutants {
		counts[m.Family]++
	}
	return counts
}

// scope restricts generation to a footprint; an unrestricted scope allows all.
type scope struct {
	restricted bool
	footprint  footprint.Footprint
	files      []string
	all        map[string]bool
}

func (g *Generator) scopeOf(files []mutation.SourceFile, entries []footprint.Ref) (scope, error) {
	all := map[string]bool{}
	for _, file := range files {
		all[file.Path] = true
	}
	if len(entries) == 0 {
		return scope{all: all}, nil
	}
	var graphs []footprint.FileGraph
	for _, file := range files {
		engine, err := g.registry.For(file.Path)
		if err != nil {
			return scope{}, err
		}
		graph, err := engine.CallGraph(file)
		if err != nil {
			return scope{}, err
		}
		graphs = append(graphs, graph)
	}
	closed := footprint.Close(graphs, entries)
	return scope{restricted: true, footprint: closed, files: closed.Files()}, nil
}

func (s scope) hasFile(path string) bool {
	if !s.restricted {
		return s.all[path]
	}
	return slices.Contains(s.files, path)
}

func (s scope) allowsEdit(edit mutation.Edit) bool {
	if !s.restricted {
		return true
	}
	ref := footprint.Ref{File: edit.File, Qualname: edit.Qualname}
	switch edit.Target {
	case mutation.TargetDefinition:
		return s.footprint.ContainsDefinition(ref)
	case mutation.TargetModuleAssignment:
		return s.footprint.ContainsAssignment(ref)
	default:
		return true
	}
}
