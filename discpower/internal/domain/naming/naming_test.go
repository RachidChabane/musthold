package naming

import (
	"testing"

	"pgregory.net/rapid"
)

func TestSnakeCase(t *testing.T) {
	t.Run("words of a camel case or pascal case identifier are joined by underscores [verifies SP-1120~1]", func(t *testing.T) {
		cases := map[string]string{
			"unitPrice":    "unit_price",
			"UnitPrice":    "unit_price",
			"parseURLPath": "parse_url_path",
			"total":        "total",
			"line2Total":   "line2_total",
			"unit_price":   "unit_price",
			"":             "",
		}
		for name, want := range cases {
			if got := SnakeCase(name); got != want {
				t.Errorf("SnakeCase(%q) = %q, want %q", name, got, want)
			}
		}
	})

	t.Run("spelling an identifier twice changes nothing [verifies SP-1120~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			name := rapid.StringMatching(`[A-Za-z][A-Za-z0-9]{0,12}`).Draw(t, "name")
			once := SnakeCase(name)
			if twice := SnakeCase(once); twice != once {
				t.Fatalf("SnakeCase(SnakeCase(%q)) = %q, want %q", name, twice, once)
			}
		})
	})
}
