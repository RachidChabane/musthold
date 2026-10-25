package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	canary "github.com/RachidChabane/spec-proof/tools/canary/internal/app"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/counterexample"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
)

// Exit code of `git diff --no-index` when the files differ.
const diffExitDifferent = 1

// Names of the work files a counterexample diff is made from.
const (
	diffBefore = "a"
	diffAfter  = "b"
	diffWork   = "counterexample"
)

// Collector writes into the records what a session left behind.
type Collector struct {
	Processes canary.Processes
	Files     canary.Files
	Preparer  Preparer
}

// Collect records the session's change, the gate's verdict on the project it
// left, the mutants the discriminating-power module kept, the diff of each mutant
// a gate block named, the outcomes of the steps and the run's description.
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
	mutants, err := c.copyMutants(l)
	if err != nil {
		return err
	}
	if err := c.counterexamples(ctx, prepared, mutants); err != nil {
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

// copyMutants copies the vault of the discriminating-power module into the
// records and returns the mutants by id.
func (c Collector) copyMutants(l plan.Layout) (map[string]counterexample.Mutant, error) {
	paths, err := c.Files.Glob(filepath.Join(l.Path(plan.CacheDir), "spec-proof", "*", record.MutantsDir, "*"+record.MutantExtension))
	if err != nil {
		return nil, err
	}
	mutants := map[string]counterexample.Mutant{}
	for _, path := range paths {
		content, err := c.Files.Read(path)
		if err != nil {
			return nil, err
		}
		if err := c.Files.Write(filepath.Join(l.Records(), record.MutantsDir, filepath.Base(path)), content, false); err != nil {
			return nil, err
		}
		mutant, err := counterexample.Decode(content)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		mutants[strings.TrimSuffix(filepath.Base(path), record.MutantExtension)] = mutant
	}
	return mutants, nil
}

// counterexamples writes the diff of each mutant a block of the gate named.
func (c Collector) counterexamples(ctx context.Context, prepared *Prepared, mutants map[string]counterexample.Mutant) error {
	l := prepared.Layout
	hooks, err := c.Files.Read(l.Record(record.HooksFile))
	if err != nil {
		return err
	}
	calls, err := evidence.DecodeHooks(hooks)
	if err != nil {
		return err
	}
	var named []string
	for _, call := range calls {
		for _, id := range record.MentionedMutants(call.Stderr) {
			if !slices.Contains(named, id) {
				named = append(named, id)
			}
		}
	}
	for _, short := range named {
		id, found := fullID(mutants, short)
		if !found {
			continue
		}
		diff, err := c.diff(ctx, prepared, short, mutants[id])
		if err != nil {
			return err
		}
		if diff == "" {
			continue
		}
		if err := c.Files.Write(filepath.Join(l.Records(), record.CounterexamplesDir, short+record.DiffExtension), []byte(diff), false); err != nil {
			return err
		}
	}
	return nil
}

func fullID(mutants map[string]counterexample.Mutant, short string) (string, bool) {
	for id := range mutants {
		if strings.HasPrefix(id, short) {
			return id, true
		}
	}
	return "", false
}

// diff is the unified diff, as git writes it, of each file the mutant edits:
// the file as the session left it, or as the equipped commit holds it when the
// edited unit is no longer there, against the same file with the mutant in
// place. A mutant that neither version can take gives no diff.
func (c Collector) diff(ctx context.Context, prepared *Prepared, short string, mutant counterexample.Mutant) (string, error) {
	l := prepared.Layout
	work := l.Path(plan.TmpDir, diffWork, short)
	var parts []string
	for _, file := range mutant.Files() {
		before, after, placed, err := c.versions(ctx, prepared, mutant, file)
		if err != nil {
			return "", err
		}
		if !placed {
			continue
		}
		if err := c.Files.Write(filepath.Join(work, diffBefore, file), []byte(before), false); err != nil {
			return "", err
		}
		if err := c.Files.Write(filepath.Join(work, diffAfter, file), []byte(after), false); err != nil {
			return "", err
		}
		argv := []string{plan.GitBinary, "diff", "--no-index", "--no-color", "--no-ext-diff", "--no-prefix",
			filepath.Join(diffBefore, file), filepath.Join(diffAfter, file)}
		output, err := c.Processes.Run(ctx, canary.CommandSpec{Argv: argv, Dir: work, Env: prepared.Environment, Timeout: stepTimeout})
		if err != nil {
			return "", err
		}
		if output.ExitCode != diffExitDifferent {
			return "", fmt.Errorf("git diff of mutant %s exited %d: %s", short, output.ExitCode, output.Stderr)
		}
		parts = append(parts, output.Stdout)
	}
	return strings.Join(parts, ""), nil
}

// versions finds the file version the mutant applies to and the file with the
// mutant in place.
func (c Collector) versions(ctx context.Context, prepared *Prepared, mutant counterexample.Mutant, file string) (string, string, bool, error) {
	final, err := c.Files.Read(filepath.Join(prepared.Layout.Project(), file))
	if err != nil {
		return "", "", false, err
	}
	candidates := []string{string(final)}
	equipped, err := c.Preparer.Run(ctx, prepared.Layout, prepared.Environment, plan.Step{Argv: []string{plan.GitBinary, "show", "HEAD:" + file}})
	if err != nil {
		return "", "", false, err
	}
	if equipped.ExitCode == 0 {
		candidates = append(candidates, equipped.Stdout)
	}
	for _, candidate := range candidates {
		after, err := mutant.Apply(file, candidate)
		switch {
		case err == nil:
			return candidate, after, true, nil
		case errors.Is(err, counterexample.ErrUnitNotFound), errors.Is(err, counterexample.ErrUnsupported):
			continue
		default:
			return "", "", false, err
		}
	}
	return "", "", false, nil
}
