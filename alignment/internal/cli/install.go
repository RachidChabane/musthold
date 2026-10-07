package cli

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/hostinstall"
	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/shallnot"
)

// Commands and flags of the install commands.
const (
	commandInit    = "init"
	commandEnable  = "enable"
	commandDisable = "disable"

	flagDir   = "dir"
	flagHooks = "hooks"
	flagCheck = "check"

	hooksAuto = "auto"
)

// errShallnotTooOld is the refusal of a project whose Shallnot cannot gate proofs.
var errShallnotTooOld = errors.New("shallnot must speak report schema 1.2 or later; run `doctor`")

func (e Env) shallnotBinary() string {
	if binary := e.lookup(envShallnot); binary != "" {
		return binary
	}
	return shallnot.DefaultBinary
}

// requireShallnot refuses to touch a project whose Shallnot does not speak the
// report schema the module reads.
func (e Env) requireShallnot() error {
	if check := e.schemaCheck(e.shallnotBinary()); !check.passed {
		return refusal{fmt.Errorf("%w: %s", errShallnotTooOld, check.detail)}
	}
	return nil
}

func (e Env) init(args []string) outcome {
	set := e.newFlags(commandInit)
	dir := set.String(flagDir, "", "project directory (default: the working directory)")
	hooks := set.String(flagHooks, hooksAuto, "harness files to install: a comma-separated list of claude, codex, copilot; \"auto\" installs claude and every harness whose configuration the project holds; \"none\" installs none")
	check := set.Bool(flagCheck, false, "change nothing; exit 1 if init would change a file")
	rest, err := parseInterleaved(set, args)
	switch {
	case err != nil:
		return outcome{err: err}
	case len(rest) > 0:
		return outcome{err: errUnexpectedParams}
	}
	root, err := e.rootOf(*dir)
	if err != nil {
		return outcome{err: err}
	}
	if err := e.requireShallnot(); err != nil {
		return outcome{err: err}
	}
	harnesses, err := hostinstall.SelectHarnesses(root, *hooks)
	if err != nil {
		return outcome{err: refusal{err}}
	}
	plan, err := hostinstall.BuildInit(hostinstall.InitOptions{Root: root, Harnesses: harnesses, Seed: hostinstall.Seed()})
	if err != nil {
		return outcome{err: refusal{err}}
	}
	return e.applyPlan(plan, *check)
}

func (e Env) toggle(command string, build func(root string) (hostinstall.Plan, error), args []string) outcome {
	set := e.newFlags(command)
	dir := set.String(flagDir, "", "project directory (default: the working directory)")
	check := set.Bool(flagCheck, false, "change nothing; exit 1 if the command would change a file")
	rest, err := parseInterleaved(set, args)
	switch {
	case err != nil:
		return outcome{err: err}
	case len(rest) > 0:
		return outcome{err: errUnexpectedParams}
	}
	root, err := e.rootOf(*dir)
	if err != nil {
		return outcome{err: err}
	}
	if err := e.requireShallnot(); err != nil {
		return outcome{err: err}
	}
	plan, err := build(root)
	if err != nil {
		return outcome{err: refusal{err}}
	}
	return e.applyPlan(plan, *check)
}

func (e Env) enable(args []string) outcome {
	return e.toggle(commandEnable, hostinstall.BuildEnable, args)
}

func (e Env) disable(args []string) outcome {
	return e.toggle(commandDisable, hostinstall.BuildDisable, args)
}

// applyPlan prints what the plan does and, unless it only checks, does it.
func (e Env) applyPlan(plan hostinstall.Plan, checkOnly bool) outcome {
	for _, action := range plan.Actions {
		fmt.Fprintf(e.Stdout, "%-9s %s\n", action.Change, filepath.ToSlash(action.Path))
	}
	if checkOnly {
		if plan.Pending() {
			return outcome{code: ExitBlocked}
		}
		return outcome{code: ExitClean}
	}
	if err := plan.Apply(); err != nil {
		return outcome{err: err}
	}
	for _, step := range plan.NextSteps {
		fmt.Fprintf(e.Stdout, "next: %s\n", step)
	}
	return outcome{code: ExitClean}
}
