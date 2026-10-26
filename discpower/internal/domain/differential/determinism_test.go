package differential_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/domain/differential"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
)

const (
	repeatedPools  = 32
	exampleArgs    = 8
	exampleDraws   = 64
	determinedSeed = 11
)

func TestDrawIgnoresTheOrderOfExampleArguments(t *testing.T) {
	t.Run("pools built from the same examples draw the same inputs, whatever the order the arguments are read in [verifies SP-170~2]", func(t *testing.T) {
		args := map[string]spec.Literal{}
		var params []differential.Param
		for index := range exampleArgs {
			name := fmt.Sprintf("p%d", index)
			args[name] = spec.Literal{Type: spec.TypeDecimal, Value: fmt.Sprintf("%d.5", index+1)}
			params = append(params, differential.Param{Name: name, Hint: differential.HintDecimal})
		}
		example := []spec.Call{{Operation: "op", Args: args}}
		signature := differential.Signature{Operation: "op", Params: params}
		want := differential.NewPool(nil, example).Draw(signature, determinedSeed, exampleDraws)
		for range repeatedPools {
			if got := differential.NewPool(nil, example).Draw(signature, determinedSeed, exampleDraws); !reflect.DeepEqual(got, want) {
				t.Fatal("two pools of the same examples drew different inputs")
			}
		}
	})
}
