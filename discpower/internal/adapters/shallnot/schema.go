package shallnot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
)

// The report schema command and the versions the module tells apart.
const (
	schemaCommand   = "schema"
	schemaReportArg = "report"
	// previousVersion is the newest report version the module cannot work with.
	previousVersion = "1.1"
)

// ErrSchemaTooOld is returned when the report schema Shallnot prints does not
// accept version 1.2, or does not declare which versions it accepts.
var ErrSchemaTooOld = errors.New("shallnot's report schema is older than " + minimumVersionLabel)

// CheckReportSchema runs `shallnot schema report` and checks that the schema
// accepts report version 1.2 and refuses 1.1: the pattern the schema gives its
// `schema_version` member is compiled and tried on both versions.
func (g Gateway) CheckReportSchema(ctx context.Context) error {
	if g.Process == nil {
		return ErrNoRunner
	}
	binary := g.Binary
	if binary == "" {
		binary = DefaultBinary
	}
	result, err := g.Process.Run(ctx, ports.Command{Argv: []string{binary, schemaCommand, schemaReportArg}, Dir: g.Root, Timeout: g.Timeout})
	switch {
	case err != nil:
		return fmt.Errorf("run %s: %w", binary, err)
	case result.ExitCode != 0:
		return fmt.Errorf("%w: %s %s %s exited with status %d", ErrSchemaTooOld, binary, schemaCommand, schemaReportArg, result.ExitCode)
	}
	var schema struct {
		Properties struct {
			SchemaVersion struct {
				Pattern string `json:"pattern"`
			} `json:"schema_version"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(result.Stdout, &schema); err != nil {
		return fmt.Errorf("%w: the schema is not JSON: %v", ErrSchemaTooOld, err)
	}
	pattern, err := regexp.Compile(schema.Properties.SchemaVersion.Pattern)
	if schema.Properties.SchemaVersion.Pattern == "" || err != nil {
		return fmt.Errorf("%w: no usable schema_version pattern", ErrSchemaTooOld)
	}
	if pattern.MatchString(previousVersion) || !pattern.MatchString(minimumVersionLabel) {
		return fmt.Errorf("%w: pattern %s", ErrSchemaTooOld, schema.Properties.SchemaVersion.Pattern)
	}
	return nil
}
