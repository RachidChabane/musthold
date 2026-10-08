package adapters_test

import (
	"regexp"
	"testing"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/adapters"
)

var hexToken = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TestRandomTokens(t *testing.T) {
	t.Run("a token is 32 hexadecimal characters and two tokens differ [verifies SP-1522~1]", func(t *testing.T) {
		first, err := adapters.RandomTokens{}.New()
		if err != nil {
			t.Fatal(err)
		}
		second, err := adapters.RandomTokens{}.New()
		if err != nil {
			t.Fatal(err)
		}
		if !hexToken.MatchString(first) || !hexToken.MatchString(second) || first == second {
			t.Errorf("tokens %q and %q", first, second)
		}
	})
}
