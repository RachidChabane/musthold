package conclusion

import (
	"os"
	"strings"
	"testing"
)

const prereg = "../../../../../../experiments/loop-1/PREREG.md"

func TestText(t *testing.T) {
	t.Run("the templates and the limits paragraph are the protocol's text [verifies SP-2604~1]", func(t *testing.T) {
		data, err := os.ReadFile(prereg)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for _, template := range Templates() {
			if !strings.Contains(text, `: "`+template+`"`) {
				t.Errorf("not in PREREG.md: %s", template)
			}
		}
		if !strings.Contains(text, "**Limits paragraph**, verbatim in the report:\n\n"+Limits+"\n") {
			t.Error("the limits paragraph is not PREREG.md's")
		}
	})
	t.Run("a filled template matches its pattern [verifies SP-2604~1]", func(t *testing.T) {
		filled := Fill(H2, map[string]string{"c": "4", "b": "6", "lo": "0.22", "hi": "0.96", "decision": "INCONCLUSIVE"})
		want := "After a finding block, the agent changed its tests or code and the gate then passed in 4/6 treatment runs (exact 95 % interval [0.22, 0.96]); INCONCLUSIVE."
		if filled != want {
			t.Fatalf("got %q", filled)
		}
		if !Pattern(H2).MatchString(filled) || Pattern(H1Contradicted).MatchString(filled) {
			t.Fatal("the pattern does not single out its template")
		}
		if Fill("<x> and <y>", map[string]string{"x": "1"}) != "1 and <y>" {
			t.Fatal("a placeholder without a value stays")
		}
	})
}
