package scan

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/differential"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
	"github.com/RachidChabane/musthold/format"
)

func TestRunTimeout(t *testing.T) {
	cases := []struct {
		reference time.Duration
		want      time.Duration
	}{
		{0, MinRunTimeout},
		{3 * time.Second, MinRunTimeout},
		{3 * time.Minute, 3 * time.Minute},
	}
	for _, c := range cases {
		t.Run("three reference runs of "+c.reference.String()+" bound a mutant run by "+c.want.String()+" [verifies SP-2111~3]", func(t *testing.T) {
			if got := runTimeout(c.reference); got != c.want {
				t.Fatalf("timeout = %s", got)
			}
		})
	}
}

func TestCoverageTimeoutBound(t *testing.T) {
	cases := []struct {
		reference, requested, want time.Duration
	}{
		{0, 0, MinCoverageTimeout},
		{30 * time.Second, 0, MinCoverageTimeout},
		{3 * time.Minute, 0, 10 * time.Minute},
		{30 * time.Minute, 0, MaxCoverageTimeout},
		{30 * time.Minute, time.Hour, time.Hour},
		{3 * time.Minute, time.Second, time.Second},
	}
	for _, c := range cases {
		t.Run("three reference runs of "+c.reference.String()+" and a requested "+c.requested.String()+" bound a coverage run by "+c.want.String()+" [verifies SP-2127~1]", func(t *testing.T) {
			if got := coverageTimeout(c.reference, c.requested); got != c.want {
				t.Fatalf("bound = %s", got)
			}
		})
	}
}

func TestStartOrder(t *testing.T) {
	t.Run("the start order holds the sentinels first, then the operator mutants in the order the allocation drew them [verifies SP-2110~3]", func(t *testing.T) {
		candidates := []Candidate{{ID: "a"}, {ID: "b", Sentinel: true}, {ID: "c"}, {ID: "d", Sentinel: true}, {ID: "e"}}
		if got := startOrder(candidates, []string{"e", "a", "c", "b", "d"}); !slices.Equal(got, []int{1, 3, 4, 0, 2}) {
			t.Fatalf("order %v", got)
		}
	})
}

func integer(value string) spec.Literal { return spec.Literal{Type: spec.TypeInteger, Value: value} }

func returns(value string) spec.Observation {
	return spec.Observation{Kind: spec.Returns, Type: spec.TypeInteger, Value: value}
}

func TestFirstDifference(t *testing.T) {
	calls := []spec.Call{
		{Operation: "m:f", Args: map[string]spec.Literal{"n": integer("1000")}},
		{Operation: "m:f", Args: map[string]spec.Literal{"n": integer("7")}},
		{Operation: "m:f", Args: map[string]spec.Literal{"n": integer("12")}},
	}
	draws := &referenceDraws{calls: calls, observed: []spec.Observation{returns("1"), returns("2"), returns("3")}, agrees: []bool{true, false, true}, order: smallestFirst(calls)}
	t.Run("the smallest input on which both sides agree with themselves and differ is reported [verifies SP-2112~1]", func(t *testing.T) {
		mutant := []spec.Observation{returns("9"), returns("9"), returns("9")}
		difference, found := firstDifference("m:f", draws, mutant, mutant)
		if !found || difference.Input != `{"n":{"type":"integer","value":"12"}}` || difference.Channel != format.ScanChannelReturnValue ||
			!difference.Reproduced || !difference.ReferenceAgrees || difference.Operation != "m:f" {
			t.Fatalf("difference %+v, %v", difference, found)
		}
	})
	t.Run("an input on which the mutant disagrees with itself is no difference [verifies SP-2112~1]", func(t *testing.T) {
		first := []spec.Observation{returns("1"), returns("2"), returns("9")}
		second := []spec.Observation{returns("1"), returns("2"), returns("8")}
		if difference, found := firstDifference("m:f", draws, first, second); found {
			t.Fatalf("difference %+v", difference)
		}
	})
	t.Run("a raise on either side is an exception difference [verifies SP-2112~1]", func(t *testing.T) {
		raised := spec.Observation{Kind: spec.Raises, Value: "ZeroDivisionError"}
		if channel := channelOf(returns("1"), raised); channel != format.ScanChannelException {
			t.Fatalf("channel %s", channel)
		}
	})
}

// typedProber reports a signature with an untyped parameter.
type typedProber struct{ invoked bool }

func (p *typedProber) Signatures(context.Context, ecosystem.Workspace, ecosystem.Toolchain, []string) ([]ports.ProbeSignature, error) {
	return []ports.ProbeSignature{{Target: "m:f", Params: []differential.Param{{Name: "n", Hint: differential.HintInteger}, {Name: "rows", Hint: differential.HintUnknown}}}}, nil
}

func (p *typedProber) Invoke(context.Context, ecosystem.Workspace, ecosystem.Toolchain, []ports.ProbeCall) ([]spec.Observation, error) {
	p.invoked = true
	return nil, nil
}

func TestUntypedParameter(t *testing.T) {
	t.Run("a function with a parameter of unknown type is never called [verifies SP-2112~1]", func(t *testing.T) {
		prober := &typedProber{}
		search := &differentialSearch{prober: prober, draws: map[string]*referenceDraws{}}
		if draws := search.prepare(context.Background(), "m:f"); draws != nil || prober.invoked {
			t.Fatalf("draws %+v, invoked %v", draws, prober.invoked)
		}
	})
}
