package proofsource_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/proofsource"
	"github.com/RachidChabane/spec-proof/alignment/internal/app/ports"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/criterion"
	"github.com/RachidChabane/spec-proof/format"
)

const goldenPower = "../../../../format/testdata/golden/discriminating-power-valid.json"

var ref = criterion.Ref{ID: "PAR-1", Revision: 1}

func write(t *testing.T, root, relative string, content []byte) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func golden(t *testing.T, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func TestCandidates(t *testing.T) {
	power := golden(t, goldenPower)
	alignment := golden(t, "../../../../format/testdata/golden/alignment-valid.json")

	t.Run("the verified set is listed before the local cache [verifies SP-328~1]", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, ".spec-proof/cache/discriminating-power/PAR-1~1/b.json", power)
		write(t, root, ".spec-proof/verified/discpower/PAR-1~1/a.json", power)
		files, err := proofsource.Source{Root: root}.Candidates(format.KindDiscriminatingPower, ref)
		if err != nil || len(files) != 2 {
			t.Fatalf("files %d: %v", len(files), err)
		}
		if files[0].Origin != ports.OriginVerified || files[1].Origin != ports.OriginLocalCache {
			t.Fatalf("origins %s %s", files[0].Origin, files[1].Origin)
		}
	})

	t.Run("only the statements of the kind asked for are listed, whatever the module directory is called [verifies SP-328~1]", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, ".spec-proof/cache/some-module/PAR-1~1/p.json", power)
		write(t, root, ".spec-proof/cache/some-module/PAR-1~1/a.json", alignment)
		write(t, root, ".spec-proof/cache/some-module/PAR-1~1/notes.txt", []byte("x"))
		write(t, root, ".spec-proof/cache/some-module/PAR-1~1/broken.json", []byte("{"))
		files, err := proofsource.Source{Root: root}.Candidates(format.KindDiscriminatingPower, ref)
		if err != nil || len(files) != 1 || string(files[0].Bytes) != string(power) {
			t.Fatalf("files %d: %v", len(files), err)
		}
	})

	t.Run("a statement of another criterion is never listed [verifies SP-328~1]", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, ".spec-proof/cache/discriminating-power/PAR-2~1/p.json", power)
		files, err := proofsource.Source{Root: root}.Candidates(format.KindDiscriminatingPower, ref)
		if err != nil || len(files) != 0 {
			t.Fatalf("files %d: %v", len(files), err)
		}
	})

	t.Run("a project with no state lists nothing [verifies SP-328~1]", func(t *testing.T) {
		files, err := proofsource.Source{Root: t.TempDir()}.Candidates(format.KindDiscriminatingPower, ref)
		if err != nil || len(files) != 0 {
			t.Fatalf("files %d: %v", len(files), err)
		}
	})

	t.Run("the verified set is read by the escaped subject key of the criterion [verifies SP-328~1]", func(t *testing.T) {
		root := t.TempDir()
		odd := criterion.Ref{ID: "A/B", Revision: 2}
		write(t, root, ".spec-proof/verified/discpower/"+format.SubjectKey(odd.String())+"/p.json", power)
		files, err := proofsource.Source{Root: root}.Candidates(format.KindDiscriminatingPower, odd)
		if err != nil || len(files) != 1 {
			t.Fatalf("files %d: %v", len(files), err)
		}
	})
}
