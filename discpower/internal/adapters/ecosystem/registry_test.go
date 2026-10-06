package ecosystem_test

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	domain "github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
)

// stubStrategy is a Strategy recognizing a tree by one marker file.
type stubStrategy struct {
	ports.Ecosystem
	name   domain.Name
	marker string
}

func (s stubStrategy) Name() domain.Name { return s.name }

func (s stubStrategy) Detect(root fs.FS) (domain.Detection, bool) {
	if _, err := fs.Stat(root, s.marker); err != nil {
		return domain.Detection{}, false
	}
	return domain.Detection{Name: s.name, Root: ".", Evidence: []string{s.marker}}, true
}

func TestRegistry(t *testing.T) {
	python := stubStrategy{name: domain.Pytest, marker: "pytest.ini"}
	golang := stubStrategy{name: domain.GoTesting, marker: "go.mod"}
	registry, err := ecosystem.New(python, golang)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("strategies are chosen by detection in registration order [verifies SP-73~1]", func(t *testing.T) {
		detected := registry.Detect(fstest.MapFS{"go.mod": {}, "pytest.ini": {}})
		if len(detected) != 2 || detected[0].Detection.Name != domain.Pytest || detected[1].Strategy.Name() != domain.GoTesting {
			t.Fatalf("detected = %+v", detected)
		}
		if only := registry.Detect(fstest.MapFS{"go.mod": {}}); len(only) != 1 || only[0].Detection.Evidence[0] != "go.mod" {
			t.Fatalf("detected = %+v", only)
		}
		if none := registry.Detect(fstest.MapFS{}); len(none) != 0 {
			t.Fatalf("detected = %+v", none)
		}
	})

	t.Run("a strategy is found by name [verifies SP-73~1]", func(t *testing.T) {
		got, err := registry.For(domain.GoTesting)
		if err != nil || got.Name() != domain.GoTesting {
			t.Fatalf("%v %v", got, err)
		}
		if _, err := registry.For(domain.Jest); !errors.Is(err, ecosystem.ErrUnknownEcosystem) {
			t.Fatalf("err = %v", err)
		}
		if names := registry.Names(); len(names) != 2 || names[0] != domain.Pytest {
			t.Fatalf("names = %v", names)
		}
	})

	t.Run("a duplicate or unsupported strategy is refused [verifies SP-73~1]", func(t *testing.T) {
		if _, err := ecosystem.New(python, python); !errors.Is(err, ecosystem.ErrDuplicateStrategy) {
			t.Fatalf("err = %v", err)
		}
		if _, err := ecosystem.New(stubStrategy{name: "cobol"}); !errors.Is(err, ecosystem.ErrInvalidStrategy) {
			t.Fatalf("err = %v", err)
		}
	})
}
