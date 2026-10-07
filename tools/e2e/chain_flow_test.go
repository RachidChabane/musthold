package e2e_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest/claudecode"
)

// Exit codes of the modules and of the gate the flows read.
const (
	exitClean      = 0
	exitBlocked    = 1
	exitToolFailed = 2
	exitAwaiting   = 3
	maxWaves       = 12

	editedLine    = "\n# reviewed\n"
	rangeCodeFile = "parcel/rates.py"
	outsideFile   = "parcel/__init__.py"
	extraTestFile = "tests/test_extra.py"
)

// equip installs Shallnot and the modules the way a user does, then gives the
// project the policy of the run: local trust, the code tree, and both proofs required.
func (p *parcelProject) equip() {
	p.t.Helper()
	if r := p.sn("", "init", "--dir", p.root, "--hooks", "claude"); r.code != exitClean {
		p.t.Fatalf("shallnot init: %s", r.output())
	}
	for _, module := range []struct {
		name string
		run  func(string, ...string) result
	}{{"discriminating-power", p.dp}, {"alignment", p.alignment}} {
		if r := module.run("", "init", "--hooks", "claude"); r.code != exitClean {
			p.t.Fatalf("%s init: %s", module.name, r.output())
		}
		if r := module.run("", "enable"); r.code != exitClean {
			p.t.Fatalf("%s enable: %s", module.name, r.output())
		}
	}
	const required = "proofs:\n  require:\n    - discriminating-power\n    - alignment\n"
	config := p.read("shallnot.yaml")
	if !strings.Contains(config, required) {
		p.t.Fatalf("enable did not require both proofs:\n%s", config)
	}
	p.write("shallnot.yaml", strings.Replace(config, required, "proofs:\n  trust: local\n  code:\n    - parcel/**\n  require:\n    - discriminating-power\n    - alignment\n", 1))
}

// edit makes the change an agent would make: it touches a file with a comment.
func (p *parcelProject) edit(relative string) {
	p.t.Helper()
	p.write(relative, p.read(relative)+editedLine)
}

// serveExecutors lets the simulated Claude Code answer every ticket of the
// discriminating-power module wave after wave, and returns the last `prove`.
func (p *parcelProject) proveDiscriminatingPower(host *claudecode.Host, answers hostprototest.AnswerSource) result {
	p.t.Helper()
	last := p.dp("", "prove", "--changed")
	for wave := 0; last.code == exitAwaiting && wave < maxWaves; wave++ {
		host.Serve(answers)
		last = p.dp("", "prove", "--changed", "--resume")
	}
	return last
}

// verifyLocal publishes what the modules wrote as a verified set the way the
// verifier does for a developer: unsigned statements accepted locally. A module that
// wrote nothing leaves an empty cache.
func (p *parcelProject) verifyLocal() {
	p.t.Helper()
	if err := os.MkdirAll(filepath.Join(p.root, filepath.FromSlash(cachePath)), directoryMode); err != nil {
		p.t.Fatal(err)
	}
	if r := p.verify("--accept-local", cachePath, "--out", verifiedPath); r.code != exitClean {
		p.t.Fatalf("spec-proof-verify: %s", r.output())
	}
}

// provedLine is one `<criterion> <test> <status> <verdict> [reason]` line of the
// alignment `prove` output.
var provedLine = regexp.MustCompile(`^(\S+~\d+) (\S+) (\S+) (\S+)(?: (\S+))?$`)

// alignmentVerdicts reads the verdict of each test the alignment `prove` printed,
// keyed by `criterion test`.
func alignmentVerdicts(output string) map[string]string {
	found := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		if match := provedLine.FindStringSubmatch(strings.TrimSpace(line)); match != nil {
			found[match[1]+" "+match[2]] = match[4]
		}
	}
	return found
}

// proveAlignment lets the simulated Claude Code answer every ticket of the
// alignment module wave after wave, through the module's real hooks and executor
// server, and returns the last `prove`.
func (p *parcelProject) proveAlignment(answers hostprototest.AnswerSource) result {
	p.t.Helper()
	host := claudeAlignmentHostOf(p)
	last := p.alignment("", "prove", "--all")
	for wave := 0; last.code == exitAwaiting && wave < maxWaves; wave++ {
		host.Serve(answers)
		last = p.alignment("", "prove", "--all", "--resume")
	}
	return last
}

// cliRunner runs a module binary as a command-line host would.
type cliRunner func(stdin string, args ...string) result

func (r cliRunner) Run(args []string, stdin []byte) ([]byte, error) {
	out := r(string(stdin), args...)
	if out.code != exitClean {
		return []byte(out.stdout), fmt.Errorf("%s", strings.TrimSpace(out.stderr))
	}
	return []byte(out.stdout), nil
}

// playForger lets the forger persona answer every pending ticket of a module from
// its command line, without any executor.
func playForger(p *parcelProject, run cliRunner) int {
	p.t.Helper()
	results, err := hostprototest.PlayCLI(run, hostprototest.PersonaForger, hostprototest.SchemaAnswers{}, func(time.Duration) {})
	if err != nil {
		p.t.Fatalf("the forger could not be played: %v", err)
	}
	return len(results)
}

// finding is one finding of the report of `shallnot check`.
type finding struct {
	Category    string `json:"category"`
	Blocking    bool   `json:"blocking"`
	Requirement string `json:"requirement_id"`
	Message     string `json:"message"`
}

// consumedProof is one statement file of the verified set with the state the gate
// gave it.
type consumedProof struct {
	Path          string `json:"path"`
	Digest        string `json:"digest"`
	State         string `json:"state"`
	Reason        string `json:"reason"`
	PredicateType string `json:"predicateType"`
	Subject       string `json:"subject"`
}

// gateReport is the part of the report of `shallnot check` the tests read.
type gateReport struct {
	code     int
	output   string
	findings []finding
	consumed []consumedProof
}

func (p *parcelProject) check() gateReport {
	p.t.Helper()
	r := p.sn("", "check", "--json-out", reportFile)
	var report struct {
		Verdict    string    `json:"verdict"`
		Findings   []finding `json:"findings"`
		Extensions struct {
			Proofs struct {
				Consumed []consumedProof `json:"consumed"`
			} `json:"proofs"`
		} `json:"extensions"`
	}
	if err := json.Unmarshal([]byte(p.read(reportFile)), &report); err != nil {
		p.t.Fatalf("the report of shallnot check: %v\n%s", err, r.output())
	}
	if (report.Verdict == "pass") != (r.code == exitClean) {
		p.t.Fatalf("verdict %q with exit %d", report.Verdict, r.code)
	}
	return gateReport{code: r.code, output: r.output(), findings: report.Findings, consumed: report.Extensions.Proofs.Consumed}
}

// The verdicts of the table the tests assert, and what a criterion reads as when its
// proofs are not usable.
const (
	verdictPass      = "PASS"
	verdictBlock     = "BLOCK"
	verdictEscalate  = "ESCALATE"
	verdictStale     = "STALE_PROOF"
	verdictMalformed = "MALFORMED_PROOF"
)

// Finding categories by what they decide for a criterion.
var (
	blockingCategories   = []string{"surviving_valid_mutant", "correct_code_fails", "example_failed"}
	escalatingCategories = []string{"misaligned_test", "escalated_mutant", "no_valid_mutant", "no_killed_valid_mutant", "mutation_sensitive_test", "awaiting_host", "awaiting_proof", "awaiting_host_judgment", "host_unresponsive"}
)

// verdict is the verdict the gate gives one criterion: a proof the gate cannot use
// shows as such, then a block, then an escalation, and a criterion with no finding
// passes.
func (g gateReport) verdict(requirement string) string {
	categories := map[string]bool{}
	for _, f := range g.findings {
		if f.Requirement == requirement {
			categories[f.Category] = true
		}
	}
	switch {
	case categories["stale_proof"]:
		return verdictStale
	case categories["malformed_proof"]:
		return verdictMalformed
	}
	for _, category := range blockingCategories {
		if categories[category] {
			return verdictBlock
		}
	}
	for _, category := range escalatingCategories {
		if categories[category] {
			return verdictEscalate
		}
	}
	return verdictPass
}

// table is the verdict of each criterion of the parcel project.
func (g gateReport) table() map[string]string {
	return map[string]string{"PAR-1": g.verdict("PAR-1"), "PAR-2": g.verdict("PAR-2"), "PAR-3": g.verdict("PAR-3")}
}

// proofState is the state the gate gave the statement file of a kind about a
// criterion, "absent" when the verified set holds none. Several files of one
// criterion read as the state they share, or "mixed".
func (g gateReport) proofState(kind, criterion string) string {
	states := map[string]bool{}
	for _, proof := range g.consumed {
		if strings.HasPrefix(proof.Path, verifiedPath+"/"+kind+"/") && proof.Subject == criterion {
			states[proof.State] = true
		}
	}
	switch len(states) {
	case 0:
		return "absent"
	case 1:
		for state := range states {
			return state
		}
	}
	return "mixed"
}
