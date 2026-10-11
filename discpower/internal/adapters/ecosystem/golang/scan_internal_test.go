package golang

import (
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
)

func TestProfileLines(t *testing.T) {
	profile := strings.Join([]string{
		"mode: set",
		"example.com/tally/tally.go:18.38,19.18 1 1",
		"example.com/tally/tally.go:19.18,21.3 1 1",
		"example.com/tally/tally.go:25.2,25.42 1 0",
		"example.com/tally/internal/rate/rate.go:3.20,5.2 1 1",
		"example.com/other/x.go:1.1,2.2 1 1",
		"example.com/tally/broken.go:x 1 1",
	}, "\n")
	t.Run("the blocks a test ran give their lines, relative to the module, and a block it did not run gives none [verifies SP-2101~1]", func(t *testing.T) {
		got := profileLines([]byte(profile), "example.com/tally")
		want := []vacuity.Line{
			{File: "internal/rate/rate.go", Line: 3}, {File: "internal/rate/rate.go", Line: 4}, {File: "internal/rate/rate.go", Line: 5},
			{File: "tally.go", Line: 18}, {File: "tally.go", Line: 19}, {File: "tally.go", Line: 20}, {File: "tally.go", Line: 21},
		}
		if !slices.Equal(got, want) {
			t.Fatalf("lines = %v", got)
		}
	})
}

func TestLeafResults(t *testing.T) {
	output := strings.Join([]string{
		`{"Action":"run","Package":"example.com/tally","Test":"TestShipping"}`,
		`{"Action":"pass","Package":"example.com/tally","Test":"TestShipping/waived"}`,
		`{"Action":"fail","Package":"example.com/tally","Test":"TestShipping/charged"}`,
		`{"Action":"fail","Package":"example.com/tally","Test":"TestShipping"}`,
		`{"Action":"pass","Package":"example.com/tally","Test":"TestShipping/waived"}`,
		`{"Action":"skip","Package":"example.com/tally","Test":"TestLater"}`,
		`{"Action":"output","Package":"example.com/tally","Output":"ok\n"}`,
		`{"Action":"pass","Package":"example.com/tally"}`,
	}, "\n")
	t.Run("each leaf test has its results in order; a parent and package events are not tests [verifies SP-2102~1]", func(t *testing.T) {
		got := leafResults([]byte(output))
		want := map[resultKey][]vacuity.Run{
			{pkg: "example.com/tally", test: "TestShipping/waived"}:  {vacuity.RunPassed, vacuity.RunPassed},
			{pkg: "example.com/tally", test: "TestShipping/charged"}: {vacuity.RunFailed},
			{pkg: "example.com/tally", test: "TestLater"}:            {vacuity.RunErrored},
		}
		if len(got) != len(want) {
			t.Fatalf("results = %v", got)
		}
		for key, runs := range want {
			if !slices.Equal(got[key], runs) {
				t.Fatalf("%v: %v", key, got[key])
			}
		}
	})
}
