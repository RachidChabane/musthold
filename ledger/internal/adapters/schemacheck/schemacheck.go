// Package schemacheck validates statements against the JSON Schemas embedded in
// the format package. The schemas are compiled from embedded bytes with a loader
// that refuses every URL, so no `$id` or `$ref` is ever fetched.
package schemacheck

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/RachidChabane/spec-proof/format"
)

// Errors returned by the checker.
var (
	// ErrInvalid wraps every schema violation of a statement.
	ErrInvalid = errors.New("statement violates its schema")
	// ErrMalformed is returned for a document that is not a JSON statement.
	ErrMalformed = errors.New("malformed statement")
	// ErrRemoteReference is returned when a schema references a resource that is
	// not embedded.
	ErrRemoteReference = errors.New("remote schema reference refused")
)

const (
	predicateTypeMember = "predicateType"
	predicateMember     = "predicate"
)

// Checker validates statements against the compiled schemas.
type Checker struct {
	statement  *jsonschema.Schema
	predicates map[string]*jsonschema.Schema
}

// New compiles every embedded schema.
func New() (*Checker, error) {
	return newChecker(format.Schemas())
}

func newChecker(schemas []format.Schema) (*Checker, error) {
	compiled, err := compileAll(schemas)
	if err != nil {
		return nil, err
	}
	checker := &Checker{
		statement:  compiled[format.StatementSchemaID],
		predicates: map[string]*jsonschema.Schema{},
	}
	for _, kind := range format.Kinds() {
		checker.predicates[kind.PredicateType()] = compiled[kind.PredicateType()]
	}
	return checker, nil
}

func compileAll(schemas []format.Schema) (map[string]*jsonschema.Schema, error) {
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(refusingLoader{})
	for _, schema := range schemas {
		document, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema.Bytes))
		if err != nil {
			return nil, fmt.Errorf("parse schema %s: %w", schema.ID, err)
		}
		if err := compiler.AddResource(schema.ID, document); err != nil {
			return nil, fmt.Errorf("add schema %s: %w", schema.ID, err)
		}
	}
	compiled := make(map[string]*jsonschema.Schema, len(schemas))
	for _, schema := range schemas {
		compiledSchema, err := compiler.Compile(schema.ID)
		if err != nil {
			return nil, fmt.Errorf("compile schema %s: %w", schema.ID, err)
		}
		compiled[schema.ID] = compiledSchema
	}
	return compiled, nil
}

// refusingLoader fails every load: all resources are registered up front.
type refusingLoader struct{}

func (refusingLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("%w: %s", ErrRemoteReference, url)
}

// Validate checks a statement against the envelope schema and the schema of its
// predicate type. A document that is not JSON, or whose predicate type is outside
// the format, is ErrMalformed or format.ErrUnknownPredicateType; a schema
// violation is ErrInvalid.
func (c *Checker) Validate(statement []byte) error {
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(statement))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	members, isObject := instance.(map[string]any)
	if !isObject {
		return fmt.Errorf("%w: a statement is a JSON object", ErrMalformed)
	}
	predicateType, _ := members[predicateTypeMember].(string)
	predicateSchema, known := c.predicates[predicateType]
	if !known {
		return fmt.Errorf("%w: %q", format.ErrUnknownPredicateType, predicateType)
	}
	if err := c.statement.Validate(instance); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := predicateSchema.Validate(members[predicateMember]); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return nil
}

// ValidateValue serializes a statement value and validates the result.
func (c *Checker) ValidateValue(statement any) error {
	encoded, err := json.Marshal(statement)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	return c.Validate(encoded)
}
