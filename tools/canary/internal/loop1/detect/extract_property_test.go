package detect

import (
	"fmt"
	"testing"

	"pgregory.net/rapid"
)

// Moving the testpaths cannot flag a file that stays inside them, and no
// testpaths flags nothing.
func TestWithinTestpathsProperties(t *testing.T) {
	segment := rapid.StringMatching(`[a-z]{1,6}`)
	rapid.Check(t, func(t *rapid.T) {
		directory := segment.Draw(t, "directory")
		name := segment.Draw(t, "name")
		file := fmt.Sprintf("%s/%s.py", directory, name)
		if !withinTestpaths(file, nil) {
			t.Fatal("a file is outside empty testpaths")
		}
		if !withinTestpaths(file, []string{directory}) {
			t.Fatalf("%s is outside its own directory", file)
		}
		if !withinTestpaths(file, []string{"."}) {
			t.Fatalf("%s is outside the root", file)
		}
		if withinTestpaths(file, []string{directory + "x"}) {
			t.Fatalf("%s is inside a sibling prefix", file)
		}
	})
}
