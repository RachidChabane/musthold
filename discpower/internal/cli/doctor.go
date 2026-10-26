package cli

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/process"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/shallnot"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
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

// diagnose checks what a run depends on: the criteria source, the ecosystem, the
// host store and its journal.
func (e Env) diagnose(rootFlag string) []check {
	binary := e.lookup(envShallnot)
	if binary == "" {
		binary = shallnot.DefaultBinary
	}
	checks := []check{lookPath("shallnot", binary), e.schemaCheck(binary)}
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
	diagnosis := hostproto.DiagnoseClassification(settings.ClassifyModeFor(format.KindDiscriminatingPower), admission, format.KindDiscriminatingPower, records)
	return check{name: "classify", passed: diagnosis.Passed, detail: diagnosis.Detail}
}

// schemaCheck verifies that Shallnot speaks the report schema the module reads.
func (e Env) schemaCheck(binary string) check {
	gateway := shallnot.Gateway{Process: process.New(), Binary: binary, Root: e.WorkDir}
	if err := gateway.CheckReportSchema(context.Background()); err != nil {
		return check{name: "shallnot-schema", detail: err.Error()}
	}
	return check{name: "shallnot-schema", passed: true, detail: "report schema 1.2 or later"}
}

func lookPath(name, binary string) check {
	path, err := exec.LookPath(binary)
	if err != nil {
		return check{name: name, detail: err.Error()}
	}
	return check{name: name, passed: true, detail: path}
}
