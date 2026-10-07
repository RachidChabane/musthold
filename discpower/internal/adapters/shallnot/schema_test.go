package shallnot

import (
	"context"
	"errors"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
)

// schemaProcess answers `shallnot schema report` with a canned result.
type schemaProcess struct {
	result  ports.CommandResult
	err     error
	command ports.Command
}

func (p *schemaProcess) Run(_ context.Context, command ports.Command) (ports.CommandResult, error) {
	p.command = command
	return p.result, p.err
}

func schemaWith(pattern string) []byte {
	return []byte(`{"properties":{"schema_version":{"pattern":` + quote(pattern) + `}}}`)
}

func quote(text string) string {
	encoded := []byte{'"'}
	for _, r := range text {
		if r == '\\' || r == '"' {
			encoded = append(encoded, '\\')
		}
		encoded = append(encoded, string(r)...)
	}
	return string(append(encoded, '"'))
}

func TestCheckReportSchema(t *testing.T) {
	check := func(result ports.CommandResult, err error) error {
		process := &schemaProcess{result: result, err: err}
		return Gateway{Process: process, Binary: "shallnot", Root: "/work"}.CheckReportSchema(context.Background())
	}

	t.Run("a schema that accepts 1.2 and refuses 1.1 is enough [verifies SP-245~1]", func(t *testing.T) {
		for _, pattern := range []string{`^1\.([2-9]|[1-9][0-9]+)$`, `^1\.[2-9]$`} {
			if err := check(ports.CommandResult{Stdout: schemaWith(pattern)}, nil); err != nil {
				t.Errorf("%s: %v", pattern, err)
			}
		}
	})

	t.Run("the command asked is the schema of the report [verifies SP-245~1]", func(t *testing.T) {
		process := &schemaProcess{result: ports.CommandResult{Stdout: schemaWith(`^1\.[2-9]$`)}}
		if err := (Gateway{Process: process, Binary: "sn", Root: "/work"}).CheckReportSchema(context.Background()); err != nil {
			t.Fatal(err)
		}
		if got := process.command.Argv; len(got) != 3 || got[0] != "sn" || got[1] != "schema" || got[2] != "report" {
			t.Fatalf("argv = %v", got)
		}
	})

	t.Run("an older schema, one with no pattern and unreadable output are too old [verifies SP-245~1]", func(t *testing.T) {
		for name, result := range map[string]ports.CommandResult{
			"1.0 and later":   {Stdout: schemaWith(`^1\.[0-9]+$`)},
			"only 1.1":        {Stdout: schemaWith(`^1\.1$`)},
			"no pattern":      {Stdout: []byte(`{"properties":{}}`)},
			"a broken regexp": {Stdout: schemaWith(`^1\.(`)},
			"not json":        {Stdout: []byte(`schema`)},
			"a failed run":    {ExitCode: 2},
		} {
			if err := check(result, nil); !errors.Is(err, ErrSchemaTooOld) {
				t.Errorf("%s: error = %v", name, err)
			}
		}
	})

	t.Run("a process that cannot run, and a gateway without one, fail [verifies SP-245~1]", func(t *testing.T) {
		if err := check(ports.CommandResult{}, errors.New("not found")); err == nil {
			t.Error("a missing executable passed")
		}
		if err := (Gateway{}).CheckReportSchema(context.Background()); !errors.Is(err, ErrNoRunner) {
			t.Errorf("error = %v", err)
		}
	})
}
