package format_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/formattest/proofs"
)

const (
	verifiesSP1984 = " [verifies SP-1984~1]"

	predicateMember  = "predicate"
	extensionsMember = "extensions"
	mutantsMember    = "mutants"

	fixtureSite       = "src/parkfee.py::fee"
	fixtureCollateral = "PARK-3~1"
	writtenWitnesses  = 6
)

// failer is what the helpers need of a test, so that a property can use them too.
type failer interface {
	Helper()
	Fatal(args ...any)
	Fatalf(format string, args ...any)
}

// schemaLoader refuses every load: all resources are registered up front.
type schemaLoader struct{}

func (schemaLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("no remote schema: %s", url)
}

type schemaPair struct {
	statement, predicate *jsonschema.Schema
}

func compiledPowerSchemas(t failer) schemaPair {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(schemaLoader{})
	for _, schema := range format.Schemas() {
		document, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema.Bytes))
		if err != nil {
			t.Fatalf("parse %s: %v", schema.ID, err)
		}
		if err := compiler.AddResource(schema.ID, document); err != nil {
			t.Fatalf("add %s: %v", schema.ID, err)
		}
	}
	compile := func(id string) *jsonschema.Schema {
		compiled, err := compiler.Compile(id)
		if err != nil {
			t.Fatalf("compile %s: %v", id, err)
		}
		return compiled
	}
	return schemaPair{statement: compile(format.StatementSchemaID), predicate: compile(format.PredicateTypeDiscriminatingPower)}
}

func (p schemaPair) accepts(t failer, document []byte) bool {
	t.Helper()
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(document))
	if err != nil {
		t.Fatal(err)
	}
	members := instance.(map[string]any)
	return p.statement.Validate(instance) == nil && p.predicate.Validate(members[predicateMember]) == nil
}

func witnessedStatement() power {
	return proofs.DiscriminatingPower(func(s *power) {
		s.Predicate.Extensions.Witnesses = &format.WitnessSummary{
			Universe: proofs.Digest("universe"), Written: writtenWitnesses, ReadingConflicts: 1, Flaky: 1,
			Unrunnable: 1, RefactorDropped: 1, EscalateOnly: 1, BlockBearing: 2,
		}
		index := proofs.RegularMutantIndex(s.Predicate.Mutants)
		mutant := &s.Predicate.Mutants[index]
		s.Predicate.Extensions.Faults = []format.FaultClass{{
			Key:       proofs.Digest("fault"),
			Sites:     []string{fixtureSite},
			Witnesses: []string{proofs.Digest("witness")},
			Mutants:   []string{mutant.Digest},
		}}
		if mutant.Extensions == nil {
			mutant.Extensions = &format.MutantExtensions{}
		}
		mutant.Extensions.Class = "owner_attributed"
		mutant.Extensions.Route = "e0_identity"
		mutant.Extensions.Witnesses = []string{proofs.Digest("witness")}
		mutant.Extensions.CollateralWith = []string{fixtureCollateral}
	})
}

func encodedWitnessed(t failer) []byte {
	t.Helper()
	encoded, err := witnessedStatement().Encode()
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

// editedJSON decodes a statement document, applies an edit to it and encodes it again.
func editedJSON(t failer, document []byte, edit func(statement map[string]any)) []byte {
	t.Helper()
	var statement map[string]any
	if err := json.Unmarshal(document, &statement); err != nil {
		t.Fatal(err)
	}
	edit(statement)
	edited, err := json.Marshal(statement)
	if err != nil {
		t.Fatal(err)
	}
	return edited
}

func extensionsOf(statement map[string]any) map[string]any {
	return statement[predicateMember].(map[string]any)[extensionsMember].(map[string]any)
}

func regularMutantExtensions(statement map[string]any) map[string]any {
	for _, entry := range statement[predicateMember].(map[string]any)[mutantsMember].([]any) {
		if extensions, present := entry.(map[string]any)[extensionsMember].(map[string]any); present && extensions["class"] != nil {
			return extensions
		}
	}
	panic("no mutant carries a class")
}

func TestWitnessExtensions(t *testing.T) {
	schemas := compiledPowerSchemas(t)

	t.Run("a statement carrying the witness summary, the fault classes and the mutant routes validates against the embedded schemas"+verifiesSP1984, func(t *testing.T) {
		if !schemas.accepts(t, encodedWitnessed(t)) {
			t.Error("the schemas refuse the statement")
		}
		if err := witnessedStatement().Validate(); err != nil {
			t.Errorf("the Go validator refuses the statement: %v", err)
		}
	})
	t.Run("the extensions round-trip through the Go types"+verifiesSP1984, func(t *testing.T) {
		original := witnessedStatement()
		encoded := encodedWitnessed(t)
		decoded, err := format.DecodeAs[format.DiscriminatingPower](encoded)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(decoded, original) {
			t.Errorf("the round trip changed the statement:\n%+v\n%+v", decoded.Predicate.Extensions, original.Predicate.Extensions)
		}
		reencoded, err := decoded.Encode()
		if err != nil || !bytes.Equal(reencoded, encoded) {
			t.Errorf("re-encoding differs (err %v)", err)
		}
		index := proofs.RegularMutantIndex(decoded.Predicate.Mutants)
		extensions := decoded.Predicate.Mutants[index].Extensions
		if extensions.Class != "owner_attributed" || extensions.Route != "e0_identity" ||
			len(extensions.Witnesses) != 1 || !reflect.DeepEqual(extensions.CollateralWith, []string{fixtureCollateral}) {
			t.Errorf("mutant extensions = %+v", extensions)
		}
		if summary := decoded.Predicate.Extensions.Witnesses; summary == nil || summary.Written != writtenWitnesses || summary.BlockBearing != 2 {
			t.Errorf("summary = %+v", summary)
		}
	})
	t.Run("the document names the members witnesses, faults, class, route and collateralWith"+verifiesSP1984, func(t *testing.T) {
		var statement map[string]any
		if err := json.Unmarshal(encodedWitnessed(t), &statement); err != nil {
			t.Fatal(err)
		}
		for _, member := range []string{"witnesses", "faults"} {
			if _, present := extensionsOf(statement)[member]; !present {
				t.Errorf("the predicate extensions lack %q", member)
			}
		}
		for _, member := range []string{"class", "route", "witnesses", "collateralWith"} {
			if _, present := regularMutantExtensions(statement)[member]; !present {
				t.Errorf("the mutant extensions lack %q", member)
			}
		}
	})
	t.Run("a statement without them stays valid and writes none of their members"+verifiesSP1984, func(t *testing.T) {
		plain, err := proofs.DiscriminatingPower().Encode()
		if err != nil {
			t.Fatal(err)
		}
		if !schemas.accepts(t, plain) {
			t.Error("the schemas refuse a statement without the witness members")
		}
		var statement map[string]any
		if err := json.Unmarshal(plain, &statement); err != nil {
			t.Fatal(err)
		}
		for _, member := range []string{"witnesses", "faults"} {
			if _, present := extensionsOf(statement)[member]; present {
				t.Errorf("a plain statement writes %q", member)
			}
		}
	})
	t.Run("a reader that ignores the members reads the same statement"+verifiesSP1984, func(t *testing.T) {
		var reader struct {
			Predicate struct {
				Tests             []string `json:"tests"`
				CorrectCodePasses struct {
					Value bool `json:"value"`
				} `json:"correctCodePasses"`
				Mutants []struct {
					Digest string `json:"digest"`
				} `json:"mutants"`
			} `json:"predicate"`
		}
		if err := json.Unmarshal(encodedWitnessed(t), &reader); err != nil {
			t.Fatal(err)
		}
		original := witnessedStatement().Predicate
		if len(reader.Predicate.Tests) != len(original.Tests) || len(reader.Predicate.Mutants) != len(original.Mutants) || !reader.Predicate.CorrectCodePasses.Value {
			t.Errorf("reader = %+v", reader)
		}
	})
	t.Run("the members are optional in every combination"+verifiesSP1984, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			withSummary, withFaults, withRoute := rapid.Bool().Draw(t, "summary"), rapid.Bool().Draw(t, "faults"), rapid.Bool().Draw(t, "route")
			document := editedJSON(t, encodedWitnessed(t), func(statement map[string]any) {
				if !withSummary {
					delete(extensionsOf(statement), "witnesses")
				}
				if !withFaults {
					delete(extensionsOf(statement), "faults")
				}
				if !withRoute {
					extensions := regularMutantExtensions(statement)
					delete(extensions, "route")
					delete(extensions, "witnesses")
					delete(extensions, "collateralWith")
				}
			})
			if !schemas.accepts(t, document) {
				t.Fatalf("refused with summary %v, faults %v, route %v", withSummary, withFaults, withRoute)
			}
		})
	})

	for _, c := range []struct {
		name string
		edit func(statement map[string]any)
	}{
		{"a summary without universe", func(s map[string]any) { delete(extensionsOf(s)["witnesses"].(map[string]any), "universe") }},
		{"a summary without written", func(s map[string]any) { delete(extensionsOf(s)["witnesses"].(map[string]any), "written") }},
		{"a universe that is no digest", func(s map[string]any) { extensionsOf(s)["witnesses"].(map[string]any)["universe"] = "abc" }},
		{"a negative count", func(s map[string]any) { extensionsOf(s)["witnesses"].(map[string]any)["blockBearing"] = -1 }},
		{"a fractional count", func(s map[string]any) { extensionsOf(s)["witnesses"].(map[string]any)["flaky"] = 1.5 }},
		{"a fault without key", func(s map[string]any) { delete(extensionsOf(s)["faults"].([]any)[0].(map[string]any), "key") }},
		{"a fault key that is no digest", func(s map[string]any) { extensionsOf(s)["faults"].([]any)[0].(map[string]any)["key"] = "k" }},
		{"a fault without sites", func(s map[string]any) { delete(extensionsOf(s)["faults"].([]any)[0].(map[string]any), "sites") }},
		{"a fault without mutants", func(s map[string]any) { extensionsOf(s)["faults"].([]any)[0].(map[string]any)["mutants"] = []any{} }},
		{"a fault witness that is no digest", func(s map[string]any) {
			extensionsOf(s)["faults"].([]any)[0].(map[string]any)["witnesses"] = []any{"w"}
		}},
		{"a mutant class outside the set", func(s map[string]any) { regularMutantExtensions(s)["class"] = "mystery" }},
		{"a mutant route outside the set", func(s map[string]any) { regularMutantExtensions(s)["route"] = "luck" }},
		{"a mutant witness that is no digest", func(s map[string]any) { regularMutantExtensions(s)["witnesses"] = []any{"w"} }},
		{"a repeated mutant witness", func(s map[string]any) {
			regularMutantExtensions(s)["witnesses"] = []any{proofs.Digest("witness"), proofs.Digest("witness")}
		}},
		{"an empty collateral criterion", func(s map[string]any) { regularMutantExtensions(s)["collateralWith"] = []any{""} }},
		{"a repeated collateral criterion", func(s map[string]any) {
			regularMutantExtensions(s)["collateralWith"] = []any{fixtureCollateral, fixtureCollateral}
		}},
	} {
		t.Run(c.name+" is refused by the schema"+verifiesSP1984, func(t *testing.T) {
			if schemas.accepts(t, editedJSON(t, encodedWitnessed(t), c.edit)) {
				t.Error("the schemas accept the statement")
			}
		})
	}

	t.Run("every class and route of the schema is carried by the Go types"+verifiesSP1984, func(t *testing.T) {
		classes := []string{"sentinel", "owner_attributed", "valid_e", "collateral", "excluded", "unsettled"}
		routes := []string{"e0_identity", "f3_equivalent", "other_criterion"}
		for _, class := range classes {
			for _, route := range routes {
				document := editedJSON(t, encodedWitnessed(t), func(s map[string]any) {
					regularMutantExtensions(s)["class"] = class
					regularMutantExtensions(s)["route"] = route
				})
				if !schemas.accepts(t, document) {
					t.Errorf("the schemas refuse class %s and route %s", class, route)
				}
				decoded, err := format.DecodeAs[format.DiscriminatingPower](document)
				if err != nil {
					t.Fatalf("class %s, route %s: %v", class, route, err)
				}
				extensions := decoded.Predicate.Mutants[proofs.RegularMutantIndex(decoded.Predicate.Mutants)].Extensions
				if extensions.Class != class || extensions.Route != route {
					t.Errorf("read %+v for class %s and route %s", extensions, class, route)
				}
			}
		}
	})
}
