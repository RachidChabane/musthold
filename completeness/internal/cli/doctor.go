package cli

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/hostinstall"
	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/process"
	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/shallnot"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// Check results of `doctor`, and the executable the change source depends on.
const (
	checkOK   = "ok"
	checkFail = "fail"
	vcsBinary = "git"
)

// check is one line of the doctor's report.
type check struct {
	name   string
	passed bool
	detail string
}

func (e Env) doctor(args []string) outcome {
	set := e.newFlags(commandDoctor, "")
	root := set.String(flagRoot, "", usageRoot)
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

// diagnose checks what a run depends on: the criteria source, the configuration,
// the host store and its journal.
func (e Env) diagnose(rootFlag string) []check {
	binary := e.lookup(envShallnot)
	if binary == "" {
		binary = shallnot.DefaultBinary
	}
	checks := []check{lookPath("shallnot", binary), e.schemaCheck(binary), lookPath(vcsBinary, vcsBinary)}
	project, err := e.openProject(rootFlag)
	if err != nil {
		return append(checks, check{name: "project", detail: err.Error()})
	}
	checks = append(checks, check{name: "policy", passed: true, detail: fmt.Sprintf("tau_adv=%v decide_k=%d", project.policy.TauAdv, project.policy.DecideK)})
	checks = append(checks, hostFilesCheck(project.root))
	records, err := project.store.Records()
	if err != nil {
		return append(checks, check{name: "host-store", detail: err.Error()})
	}
	if err := hostproto.VerifyChain(records); err != nil {
		return append(checks, check{name: "journal", detail: err.Error()})
	}
	checks = append(checks, check{name: "journal", passed: true, detail: fmt.Sprintf("%d records, chain verified", len(records))})
	return append(checks, e.classifyCheck(project.root, records))
}

// classifyCheck reports the classification switch, the admission entries of the
// module and any version of the host that differs from the one an entry was
// measured on.
func (e Env) classifyCheck(root string, records []hostproto.Record) check {
	settings, err := hostSettingsOf(root)
	if err != nil {
		return check{name: "classify", detail: err.Error()}
	}
	admission, err := e.classifyAdmission()
	if err != nil {
		return check{name: "classify", detail: err.Error()}
	}
	diagnosis := hostproto.DiagnoseClassification(settings.ClassifyModeFor(format.KindCompleteness), admission, format.KindCompleteness, records)
	return check{name: "classify", passed: diagnosis.Passed, detail: diagnosis.Detail}
}

func lookPath(name, binary string) check {
	path, err := exec.LookPath(binary)
	if err != nil {
		return check{name: name, detail: err.Error()}
	}
	return check{name: name, passed: true, detail: path}
}

// schemaCheck verifies that Shallnot speaks the report schema the module reads.
func (e Env) schemaCheck(binary string) check {
	gateway := shallnot.Gateway{Process: process.New(), Binary: binary, Root: e.WorkDir}
	if err := gateway.CheckReportSchema(context.Background()); err != nil {
		return check{name: "shallnot-schema", detail: err.Error()}
	}
	return check{name: "shallnot-schema", passed: true, detail: "report schema 1.2 or later"}
}

// hostFilesCheck names the harnesses whose files of the module the project holds.
// Without them no executor holds a capability, so the tickets stay pending: the
// check reports it and never fails the diagnosis, since a project may be proved
// from the command line alone.
func hostFilesCheck(root string) check {
	installed := hostinstall.Installed(root)
	if len(installed) == 0 {
		return check{name: "host-files", passed: true, detail: "none installed; run `init` to install them"}
	}
	return check{name: "host-files", passed: true, detail: strings.Join(installed, ", ")}
}
