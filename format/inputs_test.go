package format_test

import (
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/formattest/proofs"
)

func TestInputDescriptors(t *testing.T) {
	t.Run("the constructors build the descriptors of ADR 0008 [verifies SP-400~1]", func(t *testing.T) {
		spec := format.SpecInput("PAR-1", 2, proofs.Digest("statement"))
		if spec.Role != format.RoleSpec || spec.Name != "PAR-1~2" || spec.Digest.SHA256 != proofs.Digest("statement") {
			t.Fatalf("spec input = %+v", spec)
		}
		file := format.FileInput(format.RoleCodeTree, "src/rates.py", proofs.Digest("bytes"))
		if file.Role != format.RoleCodeTree || file.Name != "src/rates.py" || file.Annotations != nil {
			t.Fatalf("file input = %+v", file)
		}
		consumed := []byte("{\"statement\":true}\n")
		proof := format.ProofInput(format.KindDiscriminatingPower, "PAR-1~2", consumed)
		if proof.Role != format.RoleProof || proof.Name != "discriminating-power" || proof.Annotations == nil || proof.Annotations.Criterion != "PAR-1~2" {
			t.Fatalf("proof input = %+v", proof)
		}
		if proof.Digest.SHA256 != format.SHA256Hex(consumed) {
			t.Fatalf("the proof digest %s is not the digest of the exact file bytes", proof.Digest.SHA256)
		}
	})

	t.Run("a statement whose descriptors have the right shapes validates [verifies SP-400~1]", func(t *testing.T) {
		assertCodes(t, proofs.Alignment().Validate())
		assertCodes(t, proofs.DiscriminatingPower().Validate())
	})

	cases := []struct {
		name string
		edit func(*alignment)
	}{
		{"a spec input that is not named after the criterion", func(s *alignment) {
			replaceInput(s, format.RoleSpec, func(i *format.Input) { i.Name = proofs.SpecPath })
		}},
		{"a spec input whose digest is not the statement digest", func(s *alignment) {
			replaceInput(s, format.RoleSpec, func(i *format.Input) { i.Digest.SHA256 = proofs.Digest("other") })
		}},
		{"a file input with an absolute path", func(s *alignment) {
			replaceInput(s, format.RoleTestSource, func(i *format.Input) { i.Name = "/tests/test_parkfee.py" })
		}},
		{"a file input with a parent segment", func(s *alignment) {
			replaceInput(s, format.RoleTestSource, func(i *format.Input) { i.Name = "../tests/test_parkfee.py" })
		}},
		{"a file input with a backslash", func(s *alignment) {
			replaceInput(s, format.RoleTestSource, func(i *format.Input) { i.Name = `tests\test_parkfee.py` })
		}},
		{"a proof input without its criterion", func(s *alignment) { replaceInput(s, format.RoleProof, func(i *format.Input) { i.Annotations = nil }) }},
		{"a proof input whose criterion is not ID~REV", func(s *alignment) {
			replaceInput(s, format.RoleProof, func(i *format.Input) { i.Annotations = &format.Annotations{Criterion: "PARK-2"} })
		}},
		{"a proof input that is not named after a kind", func(s *alignment) { replaceInput(s, format.RoleProof, func(i *format.Input) { i.Name = "PARK-2~1" }) }},
	}
	for _, c := range cases {
		t.Run(c.name+" is reported as input_descriptor [verifies SP-400~1]", func(t *testing.T) {
			assertCodes(t, proofs.Alignment(c.edit).Validate(), format.CodeInputDescriptor)
		})
	}

	t.Run("a completeness proof names its spec file as it always did [verifies SP-400~1]", func(t *testing.T) {
		assertCodes(t, proofs.Completeness().Validate())
	})

	t.Run("a criterion is split at its revision [verifies SP-400~1]", func(t *testing.T) {
		valid := map[string]struct {
			id       string
			revision int
		}{"PAR-1~1": {"PAR-1", 1}, "A~12": {"A", 12}}
		for text, want := range valid {
			id, revision, ok := format.ParseCriterion(text)
			if !ok || id != want.id || revision != want.revision {
				t.Errorf("%q parsed to %q, %d, %v", text, id, revision, ok)
			}
		}
		for _, text := range []string{"", "PAR-1", "~1", "PAR-1~", "PAR-1~0", "PAR-1~01", "PAR-1~x", "PAR~1~2"} {
			if _, _, ok := format.ParseCriterion(text); ok {
				t.Errorf("%q parsed", text)
			}
		}
	})
}

func replaceInput(s *alignment, role format.InputRole, edit func(*format.Input)) {
	for index := range s.Predicate.Inputs {
		if s.Predicate.Inputs[index].Role == role {
			edit(&s.Predicate.Inputs[index])
		}
	}
	s.Predicate.Inputs = proofs.SortInputs(s.Predicate.Inputs)
}

func TestConfidenceSource(t *testing.T) {
	t.Run("the three forms of a confidence source are known [verifies SP-401~1]", func(t *testing.T) {
		for _, source := range []format.ConfidenceSource{
			format.SourceExecution, format.HostVotesSource(3), format.SystemOneSource("nimble:9b@" + proofs.Digest("endpoint")),
		} {
			if !source.IsKnown() {
				t.Errorf("%q is not known", source)
			}
		}
		if format.HostVotesSource(3) != "host-votes(3)" {
			t.Errorf("host votes source = %q", format.HostVotesSource(3))
		}
	})

	t.Run("anything else is not a confidence source [verifies SP-401~1]", func(t *testing.T) {
		for _, source := range []format.ConfidenceSource{"", "system-one", "system-one:", "host-votes()", "host-votes(0)", "host-votes(x)", "gut-feeling", "execution:x"} {
			if source.IsKnown() {
				t.Errorf("%q is known", source)
			}
		}
	})

	t.Run("a decision without a known source is reported, and the mapping is optional [verifies SP-401~1]", func(t *testing.T) {
		unknown := proofs.Alignment(func(s *alignment) { s.Predicate.Extensions.Decision.ConfidenceSource = "" })
		assertCodes(t, unknown.Validate(), format.CodeUnknownValue)
		mapped := proofs.Alignment(func(s *alignment) {
			s.Predicate.Extensions.Decision.ConfidenceMapping = format.MappingModalShare
		})
		assertCodes(t, mapped.Validate())
	})

	t.Run("source and mapping survive an encode and a decode [verifies SP-401~1]", func(t *testing.T) {
		original := proofs.Alignment(func(s *alignment) {
			s.Predicate.Extensions.Decision.ConfidenceMapping = format.MappingSystemOneNoul
		})
		decoded, err := format.DecodeAs[format.Alignment](mustEncode(t, original))
		if err != nil {
			t.Fatal(err)
		}
		decision := decoded.Predicate.Extensions.Decision
		if decision.ConfidenceSource != format.HostVotesSource(2) || decision.ConfidenceMapping != format.MappingSystemOneNoul {
			t.Fatalf("decision = %+v", decision)
		}
	})
}
