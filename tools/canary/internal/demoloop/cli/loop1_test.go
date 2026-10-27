package cli_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/verbs"
)

// scriptVerb finds the subcommand a front-door script runs through _lib.
var scriptVerb = regexp.MustCompile(`(?m)^\s*loop1_(?:light|heavy)\s+([a-z-]+)\b`)

// undefinedFlag is what the flag package prints for a flag a verb does not take.
const undefinedFlag = "flag provided but not defined"

func TestLoop1Verbs(t *testing.T) {
	repo, err := filepath.Abs(repoFromHere)
	if err != nil {
		t.Fatal(err)
	}
	scripts, err := filepath.Glob(filepath.Join(repo, filepath.FromSlash(study.StudyDir), "script", "*"))
	if err != nil {
		t.Fatal(err)
	}
	var named []string
	for _, script := range scripts {
		content, err := os.ReadFile(script)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range scriptVerb.FindAllStringSubmatch(string(content), -1) {
			if match[1] != filepath.Base(script) {
				t.Errorf("%s runs the verb %s", filepath.Base(script), match[1])
			}
			named = append(named, match[1])
		}
	}
	slices.Sort(named)

	t.Run("every front-door script runs a verb of the runner by its own name, and every verb has its script [verifies SP-2642~1]", func(t *testing.T) {
		if !slices.Equal(named, verbs.Names()) {
			t.Fatalf("the scripts run %v, the runner has %v", named, verbs.Names())
		}
	})

	t.Run("every verb takes the repository, the configuration and the work directory the front door passes [verifies SP-2642~1]", func(t *testing.T) {
		for _, verb := range verbs.Names() {
			_, _, stderr := run(t, nil, verb, "--repo", repo, "--config", "loop-1.json", "--work", t.TempDir(), "--help")
			if strings.Contains(stderr, undefinedFlag) {
				t.Errorf("%s: %s", verb, stderr)
			}
			for _, flag := range []string{"-repo", "-config", "-work"} {
				if !strings.Contains(stderr, flag) {
					t.Errorf("%s does not list %s:\n%s", verb, flag, stderr)
				}
			}
		}
	})
}
