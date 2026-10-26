package ecosystem_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

func TestBuildBaseline(t *testing.T) {
	t.Run("a diagnostic the reference reports is not introduced, one it does not report is [verifies SP-2440~1]", func(t *testing.T) {
		got := ecosystem.NewBuildBaseline([]string{"a", "b"}).Introduced([]string{"b", "c"})
		if len(got) != 1 || got[0] != "c" {
			t.Fatalf("introduced = %v", got)
		}
	})

	t.Run("a diagnostic is introduced each time it exceeds the reference's count [verifies SP-2440~1]", func(t *testing.T) {
		if got := ecosystem.NewBuildBaseline([]string{"a"}).Introduced([]string{"a", "a", "a"}); len(got) != 2 {
			t.Fatalf("introduced = %v", got)
		}
	})

	t.Run("a baseline introduces nothing for the diagnostics it was built from [verifies SP-2440~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			diagnostics := rapid.SliceOf(rapid.StringN(1, 8, 8)).Draw(t, "diagnostics")
			if got := ecosystem.NewBuildBaseline(diagnostics).Introduced(diagnostics); len(got) != 0 {
				t.Fatalf("introduced = %v", got)
			}
		})
	})

	t.Run("the zero baseline introduces every diagnostic [verifies SP-2440~1]", func(t *testing.T) {
		if got := (ecosystem.BuildBaseline{}).Introduced([]string{"a", "b"}); len(got) != 2 {
			t.Fatalf("introduced = %v", got)
		}
	})
}
