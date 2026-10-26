package app

import (
	"context"
	"fmt"
	"strings"

	canary "github.com/RachidChabane/musthold/tools/canary/internal/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
)

// Collector writes into the records what a session left behind.
type Collector struct {
	Processes canary.Processes
	Files     canary.Files
	Preparer  Preparer
}

// Collect records the session's change, the gate's verdict on the project it
// left, the outcomes of the steps and the run's description.
func (c Collector) Collect(ctx context.Context, prepared *Prepared, meta record.Meta) error {
	l := prepared.Layout
	change, err := c.change(ctx, prepared)
	if err != nil {
		return err
	}
	if err := c.Files.Write(l.Record(record.ChangeFile), []byte(change), false); err != nil {
		return err
	}
	gate := plan.Step{Argv: plan.GateArgv(l, l.Record(record.FinalGateFile)), Purpose: "run the gate on the project the session left"}
	outcome, err := c.Preparer.Run(ctx, l, prepared.Environment, gate)
	prepared.Steps = append(prepared.Steps, outcome)
	if err != nil {
		return err
	}
	if err := WriteSteps(c.Files, l.Record(record.StepsFile), prepared.Steps); err != nil {
		return err
	}
	encoded, err := encodeJSON(meta)
	if err != nil {
		return err
	}
	return c.Files.Write(l.Record(record.MetaFile), encoded, false)
}

// change is the session's change against the equipped commit, new files included.
func (c Collector) change(ctx context.Context, prepared *Prepared) (string, error) {
	var diff string
	for _, argv := range plan.ChangeArgv() {
		outcome, err := c.Preparer.Run(ctx, prepared.Layout, prepared.Environment, plan.Step{Argv: argv, Purpose: "record the session's change"})
		if err != nil {
			return "", err
		}
		if outcome.ExitCode != 0 {
			return "", fmt.Errorf("%w: %s exited %d: %s", ErrPrepare, strings.Join(argv, " "), outcome.ExitCode, outcome.Stderr)
		}
		diff = outcome.Stdout
		outcome.Stdout = ""
		prepared.Steps = append(prepared.Steps, outcome)
	}
	return diff, nil
}
