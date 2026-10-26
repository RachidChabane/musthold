// Package responsecheck checks the output of an executor against the response
// schema of its request. Schemas are compiled with a loader that refuses every
// URL, so a `$ref` is never fetched, and compiled schemas are cached by the
// digest of their bytes.
package responsecheck

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
)

const (
	// schemaResource is the URL the schema under check is registered at.
	schemaResource = "urn:musthold:response-schema"
	violationLine  = "\n- "
	rootLocation   = "(root)"
)

// Errors returned by the validator besides hostproto.ErrSchemaViolation.
var (
	// ErrBadSchema is returned for schema bytes that do not compile.
	ErrBadSchema = errors.New("response schema does not compile")
	// ErrRemoteReference is returned when a schema references a resource it does
	// not embed.
	ErrRemoteReference = errors.New("remote schema reference refused")
)

var _ hostproto.ResponseValidator = (*Validator)(nil)

// Validator implements hostproto.ResponseValidator.
type Validator struct {
	mutex    sync.Mutex
	compiled map[string]*jsonschema.Schema
}

// New returns a validator with an empty cache.
func New() *Validator { return &Validator{compiled: map[string]*jsonschema.Schema{}} }

// Validate accepts the output exactly when it satisfies the schema; otherwise
// the error wraps hostproto.ErrSchemaViolation and lists every violation.
func (v *Validator) Validate(schema json.RawMessage, output []byte) error {
	compiled, err := v.compile(schema)
	if err != nil {
		return err
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(output))
	if err != nil {
		return fmt.Errorf("%w: the output is not JSON: %v", hostproto.ErrSchemaViolation, err)
	}
	var violation *jsonschema.ValidationError
	if err := compiled.Validate(instance); errors.As(err, &violation) {
		return fmt.Errorf("%w:%s", hostproto.ErrSchemaViolation, listViolations(violation))
	} else if err != nil {
		return fmt.Errorf("validate output: %w", err)
	}
	return nil
}

func (v *Validator) compile(schema json.RawMessage) (*jsonschema.Schema, error) {
	key := format.SHA256Hex(schema)
	v.mutex.Lock()
	defer v.mutex.Unlock()
	if cached, found := v.compiled[key]; found {
		return cached, nil
	}
	compiled, err := compileSchema(schema)
	if err != nil {
		return nil, err
	}
	v.compiled[key] = compiled
	return compiled, nil
}

func compileSchema(schema json.RawMessage) (*jsonschema.Schema, error) {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadSchema, err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(refusingLoader{})
	if err := compiler.AddResource(schemaResource, document); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadSchema, err)
	}
	compiled, err := compiler.Compile(schemaResource)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadSchema, err)
	}
	return compiled, nil
}

// refusingLoader fails every load: the schema under check is registered up front.
type refusingLoader struct{}

func (refusingLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("%w: %s", ErrRemoteReference, url)
}

// listViolations renders one line per violated keyword, each with the location of
// the offending member in the output.
func listViolations(violation *jsonschema.ValidationError) string {
	var lines []string
	for _, unit := range violation.BasicOutput().Errors {
		if unit.Error == nil {
			continue
		}
		location := unit.InstanceLocation
		if location == "" {
			location = rootLocation
		}
		lines = append(lines, location+": "+unit.Error.String())
	}
	slices.Sort(lines)
	return violationLine + strings.Join(slices.Compact(lines), violationLine)
}
