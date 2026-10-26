package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/cli"
)

func TestInitRecognizesJVMProjects(t *testing.T) {
	cases := []struct {
		name      string
		buildFile string
		wantTool  string
	}{
		{"a pom.xml marks a Maven project", "pom.xml", "maven"},
		{"a Groovy build script marks a Gradle project", "build.gradle", "gradle"},
		{"a Kotlin build script marks a Gradle project", "build.gradle.kts", "gradle"},
	}
	for _, c := range cases {
		t.Run(c.name+" [verifies SP-1121~1]", func(t *testing.T) {
			h := newHarness(t, "")
			for _, leftover := range []string{".musthold", "pytest.ini", "conftest.py"} {
				if err := os.RemoveAll(filepath.Join(h.root, leftover)); err != nil {
					t.Fatal(err)
				}
			}
			writeFile(t, filepath.Join(h.root, c.buildFile), "")
			if code, stdout, stderr := h.run(nil, "init", "--hooks", "none"); code != cli.ExitClean {
				t.Fatalf("exit %d: %s%s", code, stdout, stderr)
			}
			config := readFile(t, filepath.Join(h.root, ".musthold", "config.yaml"))
			if !strings.Contains(config, "- "+c.wantTool+"\n") {
				t.Fatalf("the configuration does not name %s:\n%s", c.wantTool, config)
			}
		})
	}
}
