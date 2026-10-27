package ecosystem_test

import (
	"errors"
	"io/fs"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	domain "github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

// stubStrategy is a Strategy recognizing a tree by one marker file, a
// configuration or, when byConvention, a test file the runner collects by
// default.
type stubStrategy struct {
	ports.Ecosystem
	name         domain.Name
	marker       string
	byConvention bool
}

func (s stubStrategy) Name() domain.Name { return s.name }

func (s stubStrategy) Detect(root fs.FS) (domain.Detection, bool) {
	if _, err := fs.Stat(root, s.marker); err != nil {
		return domain.Detection{}, false
	}
	return domain.Detection{Name: s.name, Root: ".", Evidence: []string{s.marker}, ByConvention: s.byConvention}, true
}

func TestRegistryRanksConfiguredDetectionsFirst(t *testing.T) {
	registry, err := ecosystem.New(
		stubStrategy{name: domain.Pytest, marker: "tests/test_merge.py", byConvention: true},
		stubStrategy{name: domain.GoTesting, marker: "go.mod"},
		stubStrategy{name: domain.Vitest, marker: "package.json"},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("a strategy detected by its runner's default collection ranks after every configured one [verifies SP-73~2]", func(t *testing.T) {
		detected := registry.Detect(fstest.MapFS{"tests/test_merge.py": {}, "package.json": {}, "go.mod": {}})
		var names []domain.Name
		for _, candidate := range detected {
			names = append(names, candidate.Detection.Name)
		}
		if want := []domain.Name{domain.GoTesting, domain.Vitest, domain.Pytest}; !slices.Equal(names, want) {
			t.Fatalf("detected %v, want %v", names, want)
		}
	})
}

func TestRegistry(t *testing.T) {
	python := stubStrategy{name: domain.Pytest, marker: "pytest.ini"}
	golang := stubStrategy{name: domain.GoTesting, marker: "go.mod"}
	registry, err := ecosystem.New(python, golang)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("strategies are chosen by detection in registration order [verifies SP-73~2]", func(t *testing.T) {
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

	t.Run("a strategy is found by name [verifies SP-73~2]", func(t *testing.T) {
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

	t.Run("a duplicate or unsupported strategy is refused [verifies SP-73~2]", func(t *testing.T) {
		if _, err := ecosystem.New(python, python); !errors.Is(err, ecosystem.ErrDuplicateStrategy) {
			t.Fatalf("err = %v", err)
		}
		if _, err := ecosystem.New(stubStrategy{name: "cobol"}); !errors.Is(err, ecosystem.ErrInvalidStrategy) {
			t.Fatalf("err = %v", err)
		}
	})
}
