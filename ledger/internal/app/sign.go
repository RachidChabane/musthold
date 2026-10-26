package app

import (
	"errors"
	"fmt"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/ledger/internal/domain/refusal"
)

// SignService signs the statements of a CI run. Nothing is signed unless every
// statement passes: a signature is the one thing that cannot be taken back.
type SignService struct {
	Source    StatementSource
	Integrity IntegrityCheck
	Schemas   SchemaChecker
	Signer    Signer
	Sink      BundleSink
}

// SignReport counts what was signed.
type SignReport struct {
	Signed int
}

// Run checks every statement, signs them all and hands the bundles to the sink.
// A statement is checked against the digest manifest, the invariants of the
// format, its JSON Schema, the rule that only CI-produced statements are signed,
// and the location its content dictates.
func (s SignService) Run() (SignReport, error) {
	statements, err := s.Source.Statements()
	if err != nil {
		return SignReport{}, fmt.Errorf("read statements: %w", err)
	}
	if err := s.Integrity.Check(statements); err != nil {
		return SignReport{}, err
	}
	if err := s.checkAll(statements); err != nil {
		return SignReport{}, err
	}
	bundles := make([]Bundle, 0, len(statements))
	for _, statement := range statements {
		signed, err := s.Signer.Sign(statement.Bytes)
		if err != nil {
			return SignReport{}, fmt.Errorf("sign %s: %w", statement.Location.StatementPath(), err)
		}
		bundles = append(bundles, Bundle{Location: statement.Location, Bytes: signed})
	}
	if err := s.Sink.WriteBundles(bundles); err != nil {
		return SignReport{}, fmt.Errorf("write bundles: %w", err)
	}
	return SignReport{Signed: len(bundles)}, nil
}

func (s SignService) checkAll(statements []Statement) error {
	var problems []error
	for _, statement := range statements {
		if err := s.check(statement); err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", statement.Location.StatementPath(), err))
		}
	}
	return errors.Join(problems...)
}

func (s SignService) check(statement Statement) error {
	admitted, err := format.Admit(statement.Bytes)
	if err != nil {
		return refusal.Newf("%v", err)
	}
	if producer := admitted.Proof.Common().ProducedBy; producer != format.ProducedByCI {
		return refusal.Newf("produced by %q, only %q statements are signed", producer, format.ProducedByCI)
	}
	if err := admitted.CheckLocation(statement.Location); err != nil {
		return refusal.Newf("%v", err)
	}
	if err := s.Schemas.Validate(statement.Bytes); err != nil {
		return refusal.Newf("%v", err)
	}
	return nil
}
