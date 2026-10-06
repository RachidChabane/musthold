package format

import (
	_ "embed" // the schemas are embedded as byte slices
	"fmt"
)

// Identifiers of the schemas that are not predicate schemas.
const (
	// CommonSchemaID identifies the definitions shared by the predicate schemas.
	CommonSchemaID = "https://rachid-chabane.com/attestations/common/v1"
	// StatementSchemaID identifies the schema of the Statement v1 envelope with
	// the subject rule of each predicate.
	StatementSchemaID = "https://rachid-chabane.com/attestations/statement/v1"
)

//go:embed schemas/common.json
var commonSchema []byte

//go:embed schemas/statement.json
var statementSchema []byte

//go:embed schemas/alignment.json
var alignmentSchema []byte

//go:embed schemas/discriminating-power.json
var discriminatingPowerSchema []byte

//go:embed schemas/completeness.json
var completenessSchema []byte

// Schema is one embedded JSON Schema 2020-12 document and the URI it is
// identified by (its `$id`).
type Schema struct {
	ID    string
	Bytes []byte
}

// Schemas returns every embedded schema: the common definitions, the envelope
// schema and one schema per predicate type.
func Schemas() []Schema {
	return []Schema{
		{ID: CommonSchemaID, Bytes: commonSchema},
		{ID: StatementSchemaID, Bytes: statementSchema},
		{ID: PredicateTypeAlignment, Bytes: alignmentSchema},
		{ID: PredicateTypeDiscriminatingPower, Bytes: discriminatingPowerSchema},
		{ID: PredicateTypeCompleteness, Bytes: completenessSchema},
	}
}

// PredicateSchema returns the schema of a predicate type, whose `$id` is the
// predicate type URI.
func PredicateSchema(predicateType string) (Schema, error) {
	if _, known := KindOfPredicateType(predicateType); known {
		for _, schema := range Schemas() {
			if schema.ID == predicateType {
				return schema, nil
			}
		}
	}
	return Schema{}, fmt.Errorf("%w: %q", ErrUnknownPredicateType, predicateType)
}
