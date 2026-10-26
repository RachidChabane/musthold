package testname_test

import (
	"regexp"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/alignment/internal/platform/testname"
)

func TestCompact(t *testing.T) {
	t.Run("the citations, the separators of a reporter and the runs of blanks are taken out of a name [verifies SP-1216~1]", func(t *testing.T) {
		cases := map[string]string{
			tagged("line totals multiply [cites PAR-1~1]"):                    "line totals multiply",
			tagged("discounts > limits [cites PAR-2~1] > clamp a percentage"): "discounts limits clamp a percentage",
			tagged("  a   b  [ cites  PAR-3~2 , PAR-4~1 ]"):                   "a b",
			"plain":                           "plain",
			"has a [bracket] but no citation": "has a [bracket] but no citation",
			tagged("one [cites PAR-1~1] two [cites PAR-2~1] three"): "one two three",
			strings.ToUpper(tagged("upper [cites PAR-1~1]")):        "UPPER",
		}
		for name, want := range cases {
			if got := testname.Compact(name); got != want {
				t.Errorf("%q: got %q, want %q", name, got, want)
			}
		}
	})

	t.Run("a name without a citation or a separator keeps its words [verifies SP-1216~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			words := rapid.SliceOfN(rapid.StringMatching(`[a-z]{1,8}`), 1, 6).Draw(t, "words")
			name := ""
			for index, word := range words {
				if index > 0 {
					name += " "
				}
				name += word
			}
			if got := testname.Compact(name); got != name {
				t.Fatalf("got %q, want %q", got, name)
			}
		})
	})
}

func TestPatternOf(t *testing.T) {
	placeholders := regexp.MustCompile(`%[sd]`)

	t.Run("a title matches the names it expands to and no others [verifies SP-1216~1]", func(t *testing.T) {
		pattern := regexp.MustCompile("^" + testname.PatternOf("take %s percent off %d", placeholders) + "$")
		for _, name := range []string{"take 10 percent off 200", "take x y percent off 7", "take  10   percent off 200"} {
			if !pattern.MatchString(name) {
				t.Errorf("%q does not match", name)
			}
		}
		for _, name := range []string{"take 10 percent", "give 10 percent off 200", "take 10 percentage off 200"} {
			if pattern.MatchString(name) {
				t.Errorf("%q matches", name)
			}
		}
	})

	t.Run("the characters of a title that a pattern reads specially match themselves [verifies SP-1216~1]", func(t *testing.T) {
		pattern := regexp.MustCompile("^" + testname.PatternOf("costs (a+b) [x]? $5", placeholders) + "$")
		if !pattern.MatchString("costs (a+b) [x]? $5") || pattern.MatchString("costs aab") {
			t.Fatal("the title was read as a pattern")
		}
	})
}
