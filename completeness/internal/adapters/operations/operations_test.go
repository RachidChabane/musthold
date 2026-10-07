package operations_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/operations"
)

const configuration = `
version: 1
proofs:
  require: [alignment]
operations:
  quote:
    python: "parkfee:quote"
    go: "parkfee.Quote"
  invoice_total:
    python: "invoicing.totals:invoice_total"
`

func TestBinding(t *testing.T) {
	t.Run("an operation is bound to its function in every language, the other members of the file being ignored [verifies SP-509~1]", func(t *testing.T) {
		binding, err := operations.Decode([]byte(configuration))
		if err != nil {
			t.Fatal(err)
		}
		if got := binding.Bound("quote"); !slices.Equal(got, []string{"parkfee.Quote", "parkfee:quote"}) {
			t.Fatalf("quote is bound to %v", got)
		}
		if got := binding.Bound("invoice_total"); !slices.Equal(got, []string{"invoicing.totals:invoice_total"}) {
			t.Fatalf("invoice_total is bound to %v", got)
		}
		if got := binding.Bound("unbound"); len(got) != 0 {
			t.Fatalf("an unbound operation is bound to %v", got)
		}
	})

	t.Run("a project without a configuration, or with an empty one, binds nothing [verifies SP-509~1]", func(t *testing.T) {
		binding, err := operations.Load(t.TempDir())
		if err != nil || len(binding.Bound("quote")) != 0 {
			t.Fatalf("binding %v: %v", binding, err)
		}
		empty, err := operations.Decode(nil)
		if err != nil || len(empty.Bound("quote")) != 0 {
			t.Fatalf("empty binding %v: %v", empty, err)
		}
	})

	t.Run("the configuration of a project is read from its state directory [verifies SP-509~1]", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, ".spec-proof"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, ".spec-proof", "config.yaml"), []byte(configuration), 0o644); err != nil {
			t.Fatal(err)
		}
		binding, err := operations.Load(root)
		if err != nil || len(binding.Bound("quote")) != 2 {
			t.Fatalf("binding %v: %v", binding, err)
		}
	})

	t.Run("a configuration that is not YAML is an error [verifies SP-509~1]", func(t *testing.T) {
		if _, err := operations.Decode([]byte("operations: [")); err == nil {
			t.Fatal("decoded a broken configuration")
		}
	})
}
