package scan

import (
	"cmp"
	"context"
	"io/fs"
	"slices"
	"sync"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/differential"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/scanscope"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
	"github.com/RachidChabane/spec-proof/format"
)

// differentialSearch is the full-resolution differential search of a scan
// without a spec: it calls the function a mutant edits, bound by the ecosystem
// when its parameters are plain typed values, on inputs drawn from the
// literals of the code and of the suite, twice on the reference and twice on
// the mutant, and reports the first input on which the two differ.
type differentialSearch struct {
	prober ports.Prober
	suite  ports.ScanSuite
	ref    reference
	pool   differential.Pool
	seed   uint64
	mutex  sync.Mutex
	// draws memoizes, by target, the inputs and what the reference did on them;
	// nil for a target the search cannot call.
	draws map[string]*referenceDraws
}

// referenceDraws are the inputs of one target and the reference's two runs.
type referenceDraws struct {
	calls  []spec.Call
	probes []ports.ProbeCall
	// observed is the reference's first run and agrees tells, per input,
	// whether its second run observed the same.
	observed []spec.Observation
	agrees   []bool
	// order lists the inputs smallest first.
	order []int
}

var _ EvidenceSource = (*differentialSearch)(nil)

// newDifferentialSearch seeds the pool with the literals of the drawn sources
// and of every test file of the suite.
func newDifferentialSearch(prober ports.Prober, suite ports.ScanSuite, ref reference, project fs.FS, drawn drawn, seed uint64) *differentialSearch {
	var texts []string
	for _, path := range sortedKeys(drawn.sources) {
		texts = append(texts, string(drawn.sources[path]))
	}
	var testFiles []string
	for _, scanCase := range ref.cases {
		if !slices.Contains(testFiles, scanCase.File) {
			testFiles = append(testFiles, scanCase.File)
		}
	}
	slices.Sort(testFiles)
	for _, file := range testFiles {
		if content, err := fs.ReadFile(project, file); err == nil {
			texts = append(texts, string(content))
		}
	}
	return &differentialSearch{
		prober: prober, suite: suite, ref: ref, pool: differential.NewPool(texts, nil),
		seed: uint64(scanscope.DrawSeed(seed)), draws: map[string]*referenceDraws{},
	}
}

// Gather searches each function the mutant edits that the ecosystem binds,
// and records how far the search went. A target the probe cannot call, on the
// reference or on the mutant, yields no evidence and is no error.
func (d *differentialSearch) Gather(ctx context.Context, candidate Candidate, mutant ecosystem.Workspace, _ map[ecosystem.TestID][]vacuity.Run) (Evidence, error) {
	evidence := Evidence{Search: SearchUnbound}
	for _, edit := range candidate.Edits.Canonical().Edits {
		if edit.Kind() != mutation.TargetDefinition {
			continue
		}
		target, bound := d.suite.Operation(d.ref.workspace, edit.File, edit.Qualname)
		if !bound {
			continue
		}
		evidence.Search = max(evidence.Search, SearchUncallable)
		draws := d.prepare(ctx, target)
		if draws == nil {
			continue
		}
		first, firstErr := d.prober.Invoke(ctx, mutant, d.ref.toolchain, draws.probes)
		second, secondErr := d.prober.Invoke(ctx, mutant, d.ref.toolchain, draws.probes)
		if firstErr != nil || secondErr != nil || len(first) != len(draws.calls) || len(second) != len(draws.calls) {
			continue
		}
		evidence.Search = max(evidence.Search, SearchNoDifference)
		if difference, found := firstDifference(target, draws, first, second); found {
			evidence.Differences = append(evidence.Differences, difference)
			evidence.Search = SearchDifference
		}
	}
	return evidence, nil
}

// prepare draws the inputs of a target and runs them twice on the reference;
// it is nil when the target has an untyped parameter, no parameter, or cannot
// be called.
func (d *differentialSearch) prepare(ctx context.Context, target string) *referenceDraws {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	if draws, done := d.draws[target]; done {
		return draws
	}
	draws := d.drawFor(ctx, target)
	d.draws[target] = draws
	return draws
}

func (d *differentialSearch) drawFor(ctx context.Context, target string) *referenceDraws {
	signatures, err := d.prober.Signatures(ctx, d.ref.workspace, d.ref.toolchain, []string{target})
	if err != nil || len(signatures) != 1 || len(signatures[0].Params) == 0 {
		return nil
	}
	if slices.ContainsFunc(signatures[0].Params, func(param differential.Param) bool { return param.Hint == differential.HintUnknown }) {
		return nil
	}
	calls := d.pool.Draw(differential.Signature{Operation: target, Params: signatures[0].Params}, d.seed, differential.DefaultDraws)
	if len(calls) == 0 {
		return nil
	}
	draws := &referenceDraws{calls: calls}
	for _, call := range calls {
		draws.probes = append(draws.probes, ports.ProbeCall{Target: target, Args: call.Args})
	}
	first, firstErr := d.prober.Invoke(ctx, d.ref.workspace, d.ref.toolchain, draws.probes)
	second, secondErr := d.prober.Invoke(ctx, d.ref.workspace, d.ref.toolchain, draws.probes)
	if firstErr != nil || secondErr != nil || len(first) != len(calls) || len(second) != len(calls) {
		return nil
	}
	draws.observed = first
	for index := range calls {
		draws.agrees = append(draws.agrees, first[index].Same(second[index]))
	}
	draws.order = smallestFirst(calls)
	return draws
}

// firstDifference is the smallest input on which the reference agreed with
// itself, the mutant agreed with itself, and the two differ.
func firstDifference(target string, draws *referenceDraws, first, second []spec.Observation) (vacuity.Difference, bool) {
	for _, index := range draws.order {
		reference, mutant := draws.observed[index], first[index]
		if !draws.agrees[index] || !mutant.Same(second[index]) || reference.Same(mutant) {
			continue
		}
		input, inputErr := format.Canonicalize(draws.calls[index].Args)
		referenceText, referenceErr := format.Canonicalize(reference)
		mutantText, mutantErr := format.Canonicalize(mutant)
		if inputErr != nil || referenceErr != nil || mutantErr != nil {
			continue
		}
		return vacuity.Difference{
			Operation: target, Input: string(input), Channel: channelOf(reference, mutant),
			Reference: string(referenceText), Mutant: string(mutantText),
			Reproduced: true, ReferenceAgrees: true,
		}, true
	}
	return vacuity.Difference{}, false
}

// channelOf is what a difference was observed on: an exception when either
// side raised, the return value otherwise.
func channelOf(reference, mutant spec.Observation) format.ScanChannel {
	if reference.Kind == spec.Raises || mutant.Kind == spec.Raises {
		return format.ScanChannelException
	}
	return format.ScanChannelReturnValue
}

// smallestFirst orders the inputs by the length of their canonical form, then
// by the form itself.
func smallestFirst(calls []spec.Call) []int {
	keys := make([]string, len(calls))
	order := make([]int, len(calls))
	for index, call := range calls {
		encoded, _ := format.Canonicalize(call.Args)
		keys[index] = string(encoded)
		order[index] = index
	}
	slices.SortStableFunc(order, func(a, b int) int {
		return cmp.Or(cmp.Compare(len(keys[a]), len(keys[b])), cmp.Compare(keys[a], keys[b]))
	})
	return order
}

// sortedKeys lists the keys of a map of sources, sorted.
func sortedKeys(sources map[string][]byte) []string {
	keys := make([]string, 0, len(sources))
	for key := range sources {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
