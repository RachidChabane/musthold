package testkit_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/spec-proof/alignment/internal/testkit"
	"github.com/RachidChabane/spec-proof/format"
)

var update = flag.Bool("update", false, "rewrite the golden discriminating-power Statements")

// goldenDirectory is where the golden Statements of a fixture project live.
func goldenDirectory(project string) string {
	return filepath.Join(testkit.FixturesDir(), project, "discriminating-power")
}

func TestGoldenStatements(t *testing.T) {
	projects := map[string][]testkit.Power{testkit.PythonParcel: testkit.PythonPowers(), testkit.GoParcel: testkit.GoPowers()}
	for project, powers := range projects {
		for _, power := range powers {
			t.Run(project+" "+power.Ref.String()+" Statement matches its golden file and satisfies the format [verifies SP-329~1]", func(t *testing.T) {
				root := filepath.Join(testkit.FixturesDir(), project)
				statement := power.Statement(t, root)
				if _, err := format.Admit(statement); err != nil {
					t.Fatalf("the Statement is not admitted: %v", err)
				}
				path := filepath.Join(goldenDirectory(project), power.Ref.String()+".json")
				if *update {
					if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, statement, 0o644); err != nil {
						t.Fatal(err)
					}
				}
				golden, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("no golden file (run with -update): %v", err)
				}
				if string(golden) != string(statement) {
					t.Fatalf("%s differs from the Statement the fixture project gives; run with -update", path)
				}
			})
		}
	}
}
