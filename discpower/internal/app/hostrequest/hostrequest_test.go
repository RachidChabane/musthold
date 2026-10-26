package hostrequest_test

import (
	"cmp"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/app/hostrequest"
	"github.com/RachidChabane/musthold/discpower/internal/domain/prompt"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
)

const (
	verifiesAssembly = " [verifies SP-2262~1]"
	// pinnedID is the id of the pinned draft: the requests users answered are
	// stored under such ids, so assembling them must never change.
	pinnedID = "908fc35686cd4568bf63b5574bbf7d021e7ad4770862d084e15297a6e392701b"
)

func identity() hostrequest.Identity {
	return hostrequest.Identity{Module: format.Module{Name: "musthold-dp", Version: "0.1.0"}, Plugin: "musthold-dp", PolicyDigest: hostrequest.DigestText("policy")}
}

func pinnedDraft() hostrequest.Draft {
	return hostrequest.Draft{
		Kind:  hostproto.KindGenerateWitness,
		Built: prompt.BuildWitness(prompt.WitnessInput{Spec: "R1: zero keeps the total", Target: "R1", Statement: "zero keeps the total", Language: "Python 3, pytest"}),
		Inputs: []hostproto.Input{
			{Role: format.RoleSpec, Digest: hostrequest.DigestText("zero keeps the total")},
			{Role: format.RoleCodeTree, Digest: hostrequest.DigestText("def discount(total, percent)")},
			{Role: format.RoleSpec, Digest: hostrequest.DigestText("zero keeps the total")},
		},
		Round: 1, Seed: hostrequest.DigestText("seed"),
	}
}

func TestAssemble(t *testing.T) {
	request, err := hostrequest.Assemble(identity(), pinnedDraft())
	if err != nil {
		t.Fatal(err)
	}
	t.Run("a request carries the template and the policy among its sorted, distinct inputs, and validates"+verifiesAssembly, func(t *testing.T) {
		if err := request.Validate(); err != nil {
			t.Fatal(err)
		}
		roles := make([]format.InputRole, len(request.Inputs))
		for index, input := range request.Inputs {
			roles[index] = input.Role
		}
		if !slices.Contains(roles, format.RolePromptTemplate) || !slices.Contains(roles, format.RoleDecisionPolicy) || len(request.Inputs) != 4 {
			t.Fatalf("inputs %+v", request.Inputs)
		}
		if !slices.IsSortedFunc(request.Inputs, func(a, b hostproto.Input) int {
			return cmp.Or(strings.Compare(string(a.Role), string(b.Role)), strings.Compare(a.Digest, b.Digest))
		}) {
			t.Fatalf("inputs not sorted %+v", request.Inputs)
		}
	})
	t.Run("a generation kind carries its round and seed, and a closed decision its shape"+verifiesAssembly, func(t *testing.T) {
		if request.Round == nil || *request.Round != 1 || request.Seed == "" || request.Shape != hostproto.ShapeOpen {
			t.Fatalf("request %+v", request)
		}
		judged := pinnedDraft()
		judged.Kind = hostproto.KindJudgeScope
		scope, err := hostrequest.Assemble(identity(), judged)
		if err != nil || scope.Round != nil || scope.Seed != "" {
			t.Fatalf("scope %+v, %v", scope, err)
		}
	})
	t.Run("the same draft always gets the pinned request id"+verifiesAssembly, func(t *testing.T) {
		id, err := request.ID()
		if err != nil {
			t.Fatal(err)
		}
		if string(id) != pinnedID {
			t.Fatalf("id %s", id)
		}
	})
}
