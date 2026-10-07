package app_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/ledger/internal/app"
	"github.com/RachidChabane/spec-proof/ledger/internal/domain/refusal"
	"github.com/RachidChabane/spec-proof/ledger/internal/testkit"
)

const verifiesSign = " [verifies SP-151~1]"

type fakeSource struct{ statements []app.Statement }

func (f fakeSource) Statements() ([]app.Statement, error) { return f.statements, nil }

type failingSource struct{}

func (failingSource) Statements() ([]app.Statement, error) { return nil, errors.New("unreadable") }

type recordingSigner struct {
	signed [][]byte
	fail   bool
}

func (r *recordingSigner) Sign(statement []byte) ([]byte, error) {
	if r.fail {
		return nil, errors.New("no key")
	}
	r.signed = append(r.signed, statement)
	return append([]byte("signed:"), statement...), nil
}

type recordingSink struct{ bundles []app.Bundle }

func (r *recordingSink) WriteBundles(bundles []app.Bundle) error {
	r.bundles = append(r.bundles, bundles...)
	return nil
}

type fixedSchemas struct{ err error }

func (f fixedSchemas) Validate([]byte) error { return f.err }

type fixedIntegrity struct{ err error }

func (f fixedIntegrity) Check([]app.Statement) error { return f.err }

func validStatements(t *testing.T) []app.Statement {
	t.Helper()
	var statements []app.Statement
	for _, statement := range testkit.ValidStatements(t) {
		statements = append(statements, app.Statement{Location: statement.Location, Bytes: statement.Bytes})
	}
	return statements
}

func newService(statements []app.Statement) (app.SignService, *recordingSigner, *recordingSink) {
	signer, sink := &recordingSigner{}, &recordingSink{}
	return app.SignService{
		Source: fakeSource{statements}, Integrity: app.NoIntegrity{}, Schemas: fixedSchemas{}, Signer: signer, Sink: sink,
	}, signer, sink
}

func TestSignService(t *testing.T) {
	t.Run("every valid CI statement is signed and written at its location"+verifiesSign, func(t *testing.T) {
		statements := validStatements(t)
		service, signer, sink := newService(statements)
		report, err := service.Run()
		if err != nil || report.Signed != len(statements) {
			t.Fatalf("report %+v: %v", report, err)
		}
		for index, bundle := range sink.bundles {
			if bundle.Location != statements[index].Location || !bytes.Equal(signer.signed[index], statements[index].Bytes) {
				t.Fatalf("bundle %d: %+v", index, bundle.Location)
			}
		}
	})

	t.Run("no statement is a clean run that signs nothing"+verifiesSign, func(t *testing.T) {
		service, _, sink := newService(nil)
		report, err := service.Run()
		if err != nil || report.Signed != 0 || len(sink.bundles) != 0 {
			t.Fatalf("report %+v: %v", report, err)
		}
	})

	refused := map[string]func(statements []app.Statement) []app.Statement{
		"a statement that is not canonical": func(statements []app.Statement) []app.Statement {
			statements[0].Bytes = append(bytes.Clone(statements[0].Bytes), '\n')
			return statements
		},
		"a statement produced locally": func(statements []app.Statement) []app.Statement {
			statements[0].Bytes = bytes.Replace(statements[0].Bytes, []byte(`"producedBy":"ci"`), []byte(`"producedBy":"local"`), 1)
			return statements
		},
		"a statement at another cache key": func(statements []app.Statement) []app.Statement {
			statements[0].Location.CacheKey = format.SHA256Hex([]byte("elsewhere"))
			return statements
		},
		"a statement at another subject key": func(statements []app.Statement) []app.Statement {
			statements[0].Location.SubjectKey = "OTHER-1~1"
			return statements
		},
		"a statement that is not JSON": func(statements []app.Statement) []app.Statement {
			statements[0].Bytes = []byte("{")
			return statements
		},
	}
	for name, corrupt := range refused {
		t.Run(name+" refuses the run and signs nothing"+verifiesSign, func(t *testing.T) {
			service, signer, sink := newService(corrupt(validStatements(t)))
			if _, err := service.Run(); !errors.Is(err, refusal.ErrRefused) {
				t.Fatalf("error %v", err)
			}
			if len(signer.signed) != 0 || len(sink.bundles) != 0 {
				t.Fatalf("signed %d, wrote %d", len(signer.signed), len(sink.bundles))
			}
		})
	}

	t.Run("a statement that breaks its JSON Schema refuses the run"+verifiesSign, func(t *testing.T) {
		service, signer, _ := newService(validStatements(t))
		service.Schemas = fixedSchemas{err: errors.New("schema violation")}
		if _, err := service.Run(); !errors.Is(err, refusal.ErrRefused) || len(signer.signed) != 0 {
			t.Fatalf("error %v, signed %d", err, len(signer.signed))
		}
	})

	t.Run("a digest manifest that disagrees refuses the run"+verifiesSign, func(t *testing.T) {
		service, signer, _ := newService(validStatements(t))
		service.Integrity = fixedIntegrity{err: refusal.Newf("altered")}
		if _, err := service.Run(); !errors.Is(err, refusal.ErrRefused) || len(signer.signed) != 0 {
			t.Fatalf("error %v, signed %d", err, len(signer.signed))
		}
	})

	t.Run("every refused statement is reported, not only the first"+verifiesSign, func(t *testing.T) {
		statements := validStatements(t)
		statements[0].Bytes, statements[1].Bytes = []byte("{"), []byte("[")
		service, _, _ := newService(statements)
		_, err := service.Run()
		if err == nil || bytes.Count([]byte(err.Error()), []byte("refused:")) < 2 {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a signing failure aborts before anything is written"+verifiesSign, func(t *testing.T) {
		service, signer, sink := newService(validStatements(t))
		signer.fail = true
		if _, err := service.Run(); err == nil || errors.Is(err, refusal.ErrRefused) || len(sink.bundles) != 0 {
			t.Fatalf("error %v, wrote %d", err, len(sink.bundles))
		}
	})

	t.Run("an unreadable source is a failure"+verifiesSign, func(t *testing.T) {
		service, _, _ := newService(nil)
		service.Source = failingSource{}
		if _, err := service.Run(); err == nil || errors.Is(err, refusal.ErrRefused) {
			t.Fatalf("error %v", err)
		}
	})
}
