package cli

import (
	"fmt"
	"os/exec"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/shallnot"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// Check results of `doctor`.
const (
	checkOK   = "ok"
	checkFail = "fail"
)

// check is one line of the doctor's report.
type check struct {
	name   string
	passed bool
	detail string
}

func (e Env) doctor(args []string) outcome {
	set := e.newFlags(commandDoctor)
	root := set.String(flagRoot, "", "project root (default: the working directory)")
	rest, err := parseInterleaved(set, args)
	switch {
	case err != nil:
		return outcome{err: err}
	case len(rest) > 0:
		return outcome{err: errUnexpectedParams}
	}
	checks := e.diagnose(*root)
	failed := false
	for _, c := range checks {
		status := checkOK
		if !c.passed {
			status, failed = checkFail, true
		}
		fmt.Fprintf(e.Stdout, "%s %s %s\n", status, c.name, c.detail)
	}
	if failed {
		return outcome{code: ExitToolFailure}
	}
	return outcome{code: ExitClean}
}

// diagnose checks what a run depends on: the criteria source, the ecosystem, the
// host store and its journal.
func (e Env) diagnose(rootFlag string) []check {
	binary := e.lookup(envShallnot)
	if binary == "" {
		binary = shallnot.DefaultBinary
	}
	checks := []check{lookPath("shallnot", binary)}
	project, err := e.openProject(rootFlag)
	if err != nil {
		return append(checks, check{name: "project", detail: err.Error()})
	}
	defer project.close()
	checks = append(checks, check{name: "ecosystem", passed: true, detail: string(project.name)})
	records, err := project.store.Records()
	if err != nil {
		return append(checks, check{name: "host-store", detail: err.Error()})
	}
	if err := hostproto.VerifyChain(records); err != nil {
		return append(checks, check{name: "journal", detail: err.Error()})
	}
	return append(checks, check{name: "journal", passed: true, detail: fmt.Sprintf("%d records, chain verified", len(records))})
}

func lookPath(name, binary string) check {
	path, err := exec.LookPath(binary)
	if err != nil {
		return check{name: name, detail: err.Error()}
	}
	return check{name: name, passed: true, detail: path}
}
