package format_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/formattest/proofs"
)

type scanStatement = format.ScanStatement

func encodeScan(t *testing.T, statement scanStatement) []byte {
	t.Helper()
	encoded, err := statement.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return encoded
}

func provenMutant(s *scanStatement) *format.ScanMutant {
	index := slices.IndexFunc(s.Predicate.Mutants, func(m format.ScanMutant) bool { return m.Proven() })
	return &s.Predicate.Mutants[index]
}

func scanTestOf(s *scanStatement, class format.ScanClass) *format.ScanTest {
	index := slices.IndexFunc(s.Predicate.Tests, func(t format.ScanTest) bool { return t.Class == class })
	return &s.Predicate.Tests[index]
}

func TestScanEnvelope(t *testing.T) {
	t.Run("the fixture scan validates [verifies SP-2000~1]", func(t *testing.T) {
		if err := proofs.Scan().Validate(); err != nil {
			t.Fatal(err)
		}
	})

	cases := []struct {
		name   string
		mutate func(*scanStatement)
		want   format.ViolationCode
	}{
		{"a wrong statement type", func(s *scanStatement) { s.Type = "https://in-toto.io/Statement/v0.1" }, format.CodeStatementType},
		{"another predicate type", func(s *scanStatement) { s.PredicateType = format.PredicateTypeDiscriminatingPower }, format.CodePredicateType},
		{"two subjects", func(s *scanStatement) { s.Subject = append(s.Subject, s.Subject[0]) }, format.CodeSubject},
		{"a subject that is not a commit", func(s *scanStatement) { s.Subject[0].Name = "main" }, format.CodeSubject},
		{"a tree digest that is not SHA-256", func(s *scanStatement) { s.Subject[0].Digest.SHA256 = "abc" }, format.CodeBadDigest},
		{"no module version", func(s *scanStatement) { s.Predicate.Module.Version = "" }, format.CodeMissingField},
		{"an unknown producer", func(s *scanStatement) { s.Predicate.ProducedBy = "laptop" }, format.CodeUnknownValue},
		{"an unknown mode", func(s *scanStatement) { s.Predicate.Mode = "nightly" }, format.CodeUnknownValue},
		{"a change scan without base", func(s *scanStatement) { s.Predicate.Base = "" }, format.CodeMissingField},
		{"a repository scan with a base", func(s *scanStatement) { s.Predicate.Mode = format.ScanModeRepository }, format.CodeUnknownValue},
		{"no runner", func(s *scanStatement) { s.Predicate.Runner = "" }, format.CodeMissingField},
		{"a seed with a leading zero", func(s *scanStatement) { s.Predicate.Seed = "07" }, format.CodeUnknownValue},
		{"a seed beyond 64 bits", func(s *scanStatement) { s.Predicate.Seed = "18446744073709551616" }, format.CodeUnknownValue},
	}
	for _, c := range cases {
		t.Run(c.name+" is reported [verifies SP-2000~1]", func(t *testing.T) {
			assertCodes(t, proofs.Scan(c.mutate).Validate(), c.want)
		})
	}

	t.Run("a SHA-256 commit id names the subject as well as a SHA-1 one [verifies SP-2000~1]", func(t *testing.T) {
		statement := proofs.Scan(func(s *scanStatement) { s.Subject[0].Name = proofs.Digest("commit") })
		if err := statement.Validate(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestScanRefusesARequirement(t *testing.T) {
	t.Run("a requirement member in the predicate is reported [verifies SP-2001~1]", func(t *testing.T) {
		statement := proofs.Scan(func(s *scanStatement) {
			s.Predicate.Requirement = json.RawMessage(`{"id":"PARK-2","revision":1}`)
		})
		assertCodes(t, statement.Validate(), format.CodeRequirement)
	})

	t.Run("a decoded requirement member survives decoding and is refused on admission [verifies SP-2001~1]", func(t *testing.T) {
		var generic map[string]any
		if err := json.Unmarshal(encodeScan(t, proofs.Scan()), &generic); err != nil {
			t.Fatal(err)
		}
		generic["predicate"].(map[string]any)["requirement"] = map[string]any{"id": proofs.RequirementID, "revision": proofs.RequirementRevision}
		document, err := format.Canonicalize(generic)
		if err != nil {
			t.Fatal(err)
		}
		_, err = format.AdmitScan(document)
		assertCodes(t, err, format.CodeRequirement)
	})
}

func TestScanIsNeverAGateProof(t *testing.T) {
	document := encodeScan(t, proofs.Scan())

	t.Run("decoding a scan statement as a proof refuses its predicate type [verifies SP-2002~1]", func(t *testing.T) {
		if _, err := format.Decode(document); !errors.Is(err, format.ErrUnknownPredicateType) {
			t.Fatalf("decode error = %v, want ErrUnknownPredicateType", err)
		}
	})

	t.Run("admitting a scan statement as a proof refuses it, as the ledger and the verifier do [verifies SP-2002~1]", func(t *testing.T) {
		if _, err := format.Admit(document); !errors.Is(err, format.ErrUnknownPredicateType) {
			t.Fatalf("admit error = %v, want ErrUnknownPredicateType", err)
		}
	})

	t.Run("the scan predicate type is no proof kind and has no predicate schema [verifies SP-2002~1]", func(t *testing.T) {
		if kind, known := format.KindOfPredicateType(format.PredicateTypeScan); known {
			t.Fatalf("the scan predicate type resolves to the proof kind %q", kind)
		}
		if _, err := format.PredicateSchema(format.PredicateTypeScan); !errors.Is(err, format.ErrUnknownPredicateType) {
			t.Fatalf("predicate schema error = %v, want ErrUnknownPredicateType", err)
		}
		for _, schema := range format.Schemas() {
			if schema.ID == format.PredicateTypeScan {
				t.Fatal("the scan schema is listed among the schemas of proofs")
			}
		}
	})

	t.Run("a scan relabelled as a discriminating-power proof is not admitted [verifies SP-2002~1]", func(t *testing.T) {
		var generic map[string]any
		if err := json.Unmarshal(document, &generic); err != nil {
			t.Fatal(err)
		}
		generic["predicateType"] = format.PredicateTypeDiscriminatingPower
		relabelled, err := format.Canonicalize(generic)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := format.Admit(relabelled); err == nil {
			t.Fatal("a relabelled scan was admitted as a discriminating-power proof")
		}
	})

	t.Run("decoding a proof as a scan refuses it [verifies SP-2002~1]", func(t *testing.T) {
		proof, err := proofs.DiscriminatingPower().Encode()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := format.DecodeScan(proof); !errors.Is(err, format.ErrNotScan) {
			t.Fatalf("decode error = %v, want ErrNotScan", err)
		}
	})
}

func TestScanMutants(t *testing.T) {
	t.Run("an id its edits do not give is reported [verifies SP-2003~1]", func(t *testing.T) {
		statement := proofs.Scan(func(s *scanStatement) { provenMutant(s).Edits[0].Qualname = "split_lines" })
		assertCodes(t, statement.Validate(), format.CodeMutantID)
	})

	cases := []struct {
		name   string
		mutate func(*format.ScanMutant)
		want   format.ViolationCode
	}{
		{"no edit", func(m *format.ScanMutant) { m.Edits = nil }, format.CodeMissingField},
		{"an absolute edited file", func(m *format.ScanMutant) { m.Edits[0].File = "/src/parse.py" }, format.CodeUnknownValue},
		{"a definition target spelled out", func(m *format.ScanMutant) { m.Edits[0].Target = "definition" }, format.CodeUnknownValue},
		{"no mutated line", func(m *format.ScanMutant) { m.Lines = nil }, format.CodeMissingField},
		{"a line before the first", func(m *format.ScanMutant) { m.Lines[0].Line = 0 }, format.CodeUnknownValue},
		{"an unknown origin", func(m *format.ScanMutant) { m.Origin = "fuzzer" }, format.CodeUnknownValue},
		{"unsorted lines", func(m *format.ScanMutant) {
			m.Lines = append(m.Lines, format.ScanLine{File: m.Lines[0].File, Line: m.Lines[0].Line - 1})
		}, format.CodeUnsorted},
	}
	for _, c := range cases {
		t.Run(c.name+" is reported [verifies SP-2003~1]", func(t *testing.T) {
			statement := proofs.Scan(func(s *scanStatement) {
				mutant := provenMutant(s)
				c.mutate(mutant)
				if digest, err := format.ScanEditsDigest(mutant.Edits); err == nil && len(mutant.Edits) > 0 && c.want != format.CodeUnknownValue {
					mutant.ID = digest
				}
			})
			codes := violationCodes(t, statement.Validate())
			if !slices.Contains(codes, c.want) {
				t.Fatalf("codes = %v, want %s among them", codes, c.want)
			}
		})
	}

	t.Run("the mutant id does not depend on the order the edits are given in [verifies SP-2003~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			edits := rapid.SliceOfNDistinct(rapid.Custom(func(t *rapid.T) format.ScanEdit {
				return format.ScanEdit{
					File:      rapid.StringMatching(`[a-z]{1,4}/[a-z]{1,4}\.py`).Draw(t, "file"),
					Qualname:  rapid.StringMatching(`[a-z_]{1,6}`).Draw(t, "qualname"),
					Target:    rapid.SampledFrom([]string{"", format.ScanTargetModuleAssignment, format.ScanTargetModuleAddition}).Draw(t, "target"),
					NewSource: rapid.String().Draw(t, "source"),
				}
			}), 1, 4, func(e format.ScanEdit) string { return e.File + e.Target + e.Qualname }).Draw(t, "edits")
			shuffled := rapid.Permutation(edits).Draw(t, "shuffled")
			first, err := format.ScanEditsDigest(edits)
			if err != nil {
				t.Fatal(err)
			}
			second, err := format.ScanEditsDigest(shuffled)
			if err != nil {
				t.Fatal(err)
			}
			if first != second {
				t.Fatalf("digest %s and %s for the same edits", first, second)
			}
		})
	})

	t.Run("a sentinel with a route is reported [verifies SP-2004~1]", func(t *testing.T) {
		statement := proofs.Scan(func(s *scanStatement) {
			for index := range s.Predicate.Mutants {
				if s.Predicate.Mutants[index].Sentinel {
					s.Predicate.Mutants[index].Routes = []format.ScanRoute{{Kind: format.ScanRouteKilledByTest, Test: proofs.ScanCaughtTest}}
				}
			}
		})
		assertCodes(t, statement.Validate(), format.CodeSentinelNotEquivalent)
	})

	t.Run("a model sentinel is reported [verifies SP-2004~1]", func(t *testing.T) {
		statement := proofs.Scan(func(s *scanStatement) {
			for index := range s.Predicate.Mutants {
				if s.Predicate.Mutants[index].Sentinel {
					s.Predicate.Mutants[index].Origin = format.ScanOriginModel
				}
			}
		})
		assertCodes(t, statement.Validate(), format.CodeSentinelNotEquivalent)
	})
}

func TestScanRoutes(t *testing.T) {
	witness := proofs.ScanWitnessID(proofs.ScanWitnessCode)
	question := proofs.ScanWitnessID(proofs.ScanQuestionCode)
	cases := []struct {
		name  string
		route format.ScanRoute
		want  []format.ViolationCode
	}{
		{"a killing test", format.ScanRoute{Kind: format.ScanRouteKilledByTest, Test: proofs.ScanCaughtTest}, nil},
		{"a killing test with an input", format.ScanRoute{Kind: format.ScanRouteKilledByTest, Test: proofs.ScanCaughtTest, Input: `"a"`}, []format.ViolationCode{format.CodeScanRoute}},
		{"a killed route without its test", format.ScanRoute{Kind: format.ScanRouteKilledByTest}, []format.ViolationCode{format.CodeScanRoute}},
		{"a difference", format.ScanRoute{Kind: format.ScanRouteDifferential, Operation: "split_fields", Input: `"a,"`, Channel: format.ScanChannelException, Reference: `["a", ""]`, Mutant: "IndexError"}, nil},
		{"a difference with equal results", format.ScanRoute{Kind: format.ScanRouteDifferential, Operation: "split_fields", Input: `"a,"`, Channel: format.ScanChannelReturnValue, Reference: "1", Mutant: "1"}, []format.ViolationCode{format.CodeScanRoute}},
		{"a difference on the standard output", format.ScanRoute{Kind: format.ScanRouteDifferential, Operation: "split_fields", Input: `"a,"`, Channel: "stdout", Reference: "1", Mutant: "2"}, []format.ViolationCode{format.CodeUnknownValue}},
		{"a difference naming a witness", format.ScanRoute{Kind: format.ScanRouteDifferential, Operation: "split_fields", Input: `"a,"`, Channel: format.ScanChannelReturnValue, Reference: "1", Mutant: "2", Witness: witness}, []format.ViolationCode{format.CodeScanRoute}},
		{"a difference without its operation", format.ScanRoute{Kind: format.ScanRouteDifferential, Input: `"a,"`, Channel: format.ScanChannelReturnValue, Reference: "1", Mutant: "2"}, []format.ViolationCode{format.CodeScanRoute}},
		{"a killing test with an operation", format.ScanRoute{Kind: format.ScanRouteKilledByTest, Test: proofs.ScanCaughtTest, Operation: "split_fields"}, []format.ViolationCode{format.CodeScanRoute}},
		{"an admitted witness", format.ScanRoute{Kind: format.ScanRouteWitness, Witness: witness}, nil},
		{"a witness the reference fails", format.ScanRoute{Kind: format.ScanRouteWitness, Witness: question}, []format.ViolationCode{format.CodeScanReference}},
		{"a witness outside the scan", format.ScanRoute{Kind: format.ScanRouteWitness, Witness: proofs.Digest("elsewhere")}, []format.ViolationCode{format.CodeScanReference}},
		{"an unknown kind", format.ScanRoute{Kind: "judged", Test: proofs.ScanCaughtTest}, []format.ViolationCode{format.CodeUnknownValue}},
	}
	for _, c := range cases {
		t.Run(c.name+" is checked against its kind [verifies SP-2005~1]", func(t *testing.T) {
			statement := proofs.Scan(func(s *scanStatement) { provenMutant(s).Routes = []format.ScanRoute{c.route} })
			assertCodes(t, statement.Validate(), c.want...)
		})
	}

	t.Run("two differences on one input with other results are distinct routes [verifies SP-2005~1]", func(t *testing.T) {
		first := format.ScanRoute{Kind: format.ScanRouteDifferential, Operation: "split_fields", Input: `"a,"`, Channel: format.ScanChannelReturnValue, Reference: "1", Mutant: "2"}
		second := first
		second.Mutant = "3"
		if format.CompareScanRoutes(first, second) >= 0 || format.CompareScanRoutes(first, first) != 0 {
			t.Fatal("the route order is not total")
		}
	})

	t.Run("routes in descending order are reported [verifies SP-2005~1]", func(t *testing.T) {
		statement := proofs.Scan(func(s *scanStatement) {
			provenMutant(s).Routes = []format.ScanRoute{
				{Kind: format.ScanRouteWitness, Witness: witness},
				{Kind: format.ScanRouteKilledByTest, Test: proofs.ScanCaughtTest},
			}
		})
		assertCodes(t, statement.Validate(), format.CodeUnsorted)
	})
}

func TestScanTests(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*scanStatement)
		want   format.ViolationCode
	}{
		{"a verifies-nothing test with a killed change", func(s *scanStatement) {
			scanTestOf(s, format.ScanVerifiesNothing).Changes[0].Outcome = format.ScanKilled
		}, format.CodeScanClass},
		{"a verifies-nothing test with an inconclusive change", func(s *scanStatement) {
			scanTestOf(s, format.ScanVerifiesNothing).Changes[0].Outcome = format.ScanInconclusive
		}, format.CodeScanClass},
		{"a verifies-nothing test without change", func(s *scanStatement) {
			scanTestOf(s, format.ScanVerifiesNothing).Changes = nil
		}, format.CodeScanClass},
		{"a caught test without a killed change", func(s *scanStatement) {
			test := scanTestOf(s, format.ScanCaught)
			for index := range test.Changes {
				test.Changes[index].Outcome = format.ScanSurvived
			}
		}, format.CodeScanClass},
		{"an unproven test without reason", func(s *scanStatement) {
			scanTestOf(s, format.ScanUnproven).Reason = ""
		}, format.CodeUnknownValue},
		{"a caught test with a reason", func(s *scanStatement) {
			scanTestOf(s, format.ScanCaught).Reason = format.ScanTooFewChanges
		}, format.CodeScanClass},
		{"a change that is unproven", func(s *scanStatement) {
			provenMutant(s).Routes = nil
		}, format.CodeScanReference},
		{"a change outside the scan", func(s *scanStatement) {
			scanTestOf(s, format.ScanUnproven).Changes[0].Mutant = proofs.Digest("elsewhere")
		}, format.CodeScanReference},
		{"a label that is no promise", func(s *scanStatement) {
			scanTestOf(s, format.ScanVerifiesNothing).Promises = []string{"P9"}
		}, format.CodeScanReference},
		{"a listed test that is not sampled", func(s *scanStatement) {
			scanTestOf(s, format.ScanUnproven).Class = format.ScanNotSampled
			s.Predicate.Counts.Unproven--
			s.Predicate.Counts.NotSampled++
		}, format.CodeUnknownValue},
	}
	for _, c := range cases {
		t.Run(c.name+" is reported [verifies SP-2006~1]", func(t *testing.T) {
			codes := violationCodes(t, proofs.Scan(c.mutate).Validate())
			if !slices.Contains(codes, c.want) {
				t.Fatalf("codes = %v, want %s among them", codes, c.want)
			}
		})
	}

	t.Run("a sentinel cited as a change is reported [verifies SP-2006~1]", func(t *testing.T) {
		statement := proofs.Scan(func(s *scanStatement) {
			sentinel := s.Predicate.Mutants[slices.IndexFunc(s.Predicate.Mutants, func(m format.ScanMutant) bool { return m.Sentinel })]
			test := scanTestOf(s, format.ScanUnproven)
			test.Changes = []format.ScanChange{{Mutant: sentinel.ID, Outcome: format.ScanSurvived}}
		})
		assertCodes(t, statement.Validate(), format.CodeScanReference)
	})
}

func TestScanCounts(t *testing.T) {
	t.Run("classes that do not add up to the suite are reported [verifies SP-2007~1]", func(t *testing.T) {
		assertCodes(t, proofs.Scan(func(s *scanStatement) { s.Predicate.Counts.Tests++ }).Validate(), format.CodeScanCounts)
	})

	t.Run("a verdict count that is not the number of listed tests is reported [verifies SP-2007~1]", func(t *testing.T) {
		statement := proofs.Scan(func(s *scanStatement) {
			s.Predicate.Counts.VerifiesNothing++
			s.Predicate.Counts.NotSampled--
		})
		assertCodes(t, statement.Validate(), format.CodeScanCounts)
	})

	t.Run("a negative count is reported [verifies SP-2007~1]", func(t *testing.T) {
		statement := proofs.Scan(func(s *scanStatement) {
			s.Predicate.Counts.NotGreen = -1
			s.Predicate.Counts.NotSampled += 4
		})
		assertCodes(t, statement.Validate(), format.CodeScanCounts)
	})

	t.Run("adding a test of a class counts it in that class and in the suite [verifies SP-2007~1]", func(t *testing.T) {
		var counts format.ScanCounts
		for _, class := range format.ScanClasses() {
			counts = counts.Add(class)
			if counts.Of(class) != 1 {
				t.Fatalf("%s counted %d times", class, counts.Of(class))
			}
		}
		if counts.Tests != len(format.ScanClasses()) || counts.Judged() != len(format.ScanVerdicts()) {
			t.Fatalf("counts = %+v", counts)
		}
	})

	t.Run("the judged denominator is the three verdicts and never the suite [verifies SP-2007~1]", func(t *testing.T) {
		counts := format.ScanCounts{Tests: 412, VerifiesNothing: 7, Caught: 21, Unproven: 12, NotSampled: 369, NotGreen: 3}
		if counts.Judged() != 40 {
			t.Fatalf("judged = %d, want 40", counts.Judged())
		}
		sum := 0
		for _, class := range format.ScanClasses() {
			sum += counts.Of(class)
		}
		if sum != counts.Tests {
			t.Fatalf("the classes add up to %d, not %d", sum, counts.Tests)
		}
		if !slices.Equal(format.ScanVerdicts(), format.ScanClasses()[:len(format.ScanVerdicts())]) {
			t.Fatalf("the verdicts %v do not lead the classes %v", format.ScanVerdicts(), format.ScanClasses())
		}
	})
}

func TestScanPromises(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*scanStatement)
		want   format.ViolationCode
	}{
		{"promises of a scan that checked none", func(s *scanStatement) { s.Predicate.PromisesChecked = false }, format.CodeScanReference},
		{"a block-bearing witness that is not admitted", func(s *scanStatement) {
			for index := range s.Predicate.Witnesses {
				if s.Predicate.Witnesses[index].BlockBearing {
					s.Predicate.Witnesses[index].Admitted = false
				}
			}
		}, format.CodeScanClass},
		{"a witness of an unknown promise", func(s *scanStatement) { s.Predicate.Witnesses[0].Promise = "P9" }, format.CodeScanReference},
		{"a broken promise on an escalate-only witness", func(s *scanStatement) {
			for index := range s.Predicate.Witnesses {
				s.Predicate.Witnesses[index].BlockBearing = false
			}
		}, format.CodeScanReference},
		{"a broken promise whose mutant does not fail the witness", func(s *scanStatement) {
			s.Predicate.BrokenPromises[0].Mutant = provenMutant(s).ID
		}, format.CodeScanReference},
		{"a question on an admitted witness", func(s *scanStatement) {
			s.Predicate.Questions[0].Witness = proofs.ScanWitnessID(proofs.ScanWitnessCode)
		}, format.CodeScanReference},
		{"a witness id that is not a digest", func(s *scanStatement) { s.Predicate.Witnesses[0].ID = "w1" }, format.CodeBadDigest},
		{"a quote without source digest", func(s *scanStatement) { s.Predicate.Promises[0].Source.Digest.SHA256 = "" }, format.CodeBadDigest},
	}
	for _, c := range cases {
		t.Run(c.name+" is reported [verifies SP-2008~1]", func(t *testing.T) {
			codes := violationCodes(t, proofs.Scan(c.mutate).Validate())
			if !slices.Contains(codes, c.want) {
				t.Fatalf("codes = %v, want %s among them", codes, c.want)
			}
		})
	}

	t.Run("a scan without host holds no promise and validates [verifies SP-2008~1]", func(t *testing.T) {
		statement := proofs.Scan(func(s *scanStatement) {
			s.Predicate.PromisesChecked = false
			s.Predicate.Promises, s.Predicate.Witnesses, s.Predicate.BrokenPromises, s.Predicate.Questions = nil, nil, nil, nil
			s.Predicate.Mutants = slices.DeleteFunc(s.Predicate.Mutants, func(m format.ScanMutant) bool { return m.Origin == format.ScanOriginModel })
			for index := range s.Predicate.Tests {
				test := &s.Predicate.Tests[index]
				test.Promises = nil
				test.Changes = slices.DeleteFunc(test.Changes, func(c format.ScanChange) bool {
					return !slices.ContainsFunc(s.Predicate.Mutants, func(m format.ScanMutant) bool { return m.ID == c.Mutant })
				})
			}
		})
		if err := statement.Validate(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestCheckQuote(t *testing.T) {
	promise := proofs.Scan().Predicate.Promises[0]
	readme := []byte(proofs.ScanReadmeText())

	t.Run("a quote found byte for byte in its unchanged source passes [verifies SP-2009~1]", func(t *testing.T) {
		if err := format.CheckQuote(readme, promise); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("a quote that differs by one byte is absent [verifies SP-2009~1]", func(t *testing.T) {
		altered := promise
		altered.Quote = string(bytes.ToUpper([]byte(promise.Quote[:1]))) + promise.Quote[1:] + " "
		if err := format.CheckQuote(readme, altered); !errors.Is(err, format.ErrQuoteAbsent) {
			t.Fatalf("error = %v, want ErrQuoteAbsent", err)
		}
	})

	t.Run("a source that changed since the quote was taken is refused [verifies SP-2009~1]", func(t *testing.T) {
		if err := format.CheckQuote(append(slices.Clone(readme), '\n'), promise); !errors.Is(err, format.ErrQuoteSourceChanged) {
			t.Fatalf("error = %v, want ErrQuoteSourceChanged", err)
		}
	})

	t.Run("an empty quote is absent [verifies SP-2009~1]", func(t *testing.T) {
		empty := promise
		empty.Quote = ""
		if err := format.CheckQuote(readme, empty); !errors.Is(err, format.ErrQuoteAbsent) {
			t.Fatalf("error = %v, want ErrQuoteAbsent", err)
		}
	})
}

func TestScanEncoding(t *testing.T) {
	t.Run("encoding gives canonical bytes that decode to the same statement [verifies SP-2010~1]", func(t *testing.T) {
		statement := proofs.Scan()
		encoded := encodeScan(t, statement)
		canonical, err := format.CanonicalizeJSON(encoded)
		if err != nil || !bytes.Equal(canonical, encoded) {
			t.Fatalf("encoded bytes are not canonical (%v)", err)
		}
		admitted, err := format.AdmitScan(encoded)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(admitted, statement) {
			t.Fatal("the admitted statement differs from the encoded one")
		}
	})

	t.Run("an invalid statement is not encoded [verifies SP-2010~1]", func(t *testing.T) {
		_, err := proofs.Scan(func(s *scanStatement) { s.Predicate.Runner = "" }).Encode()
		assertCodes(t, err, format.CodeMissingField)
	})

	t.Run("bytes that are not their canonical form are not admitted [verifies SP-2010~1]", func(t *testing.T) {
		indented, err := json.MarshalIndent(proofs.Scan(), "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := format.AdmitScan(indented); !errors.Is(err, format.ErrNotCanonical) {
			t.Fatalf("error = %v, want ErrNotCanonical", err)
		}
	})

	t.Run("bytes that are not JSON are not decoded [verifies SP-2010~1]", func(t *testing.T) {
		if _, err := format.DecodeScan([]byte(`{"predicateType":`)); err == nil {
			t.Fatal("decoded a truncated document")
		}
	})
}
