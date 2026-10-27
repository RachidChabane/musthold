// Package canary is the `canary` verb of the loop-1 study (PREREG §13.3): three
// sessions that are not runs of the study. C1 observes which hosts the pinned
// CLI contacts through a proxy that allows and logs every host; C2 runs the same
// session with the allowlist enforced and probes the container's isolation; C3
// runs a treatment session on boxoffice in its done state and checks the loop.
// Their output feeds the inputs freeze. The checks are pure; the sessions run
// through the attempts of the run verb.
package canary

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/egress"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/evidence"
)

// ModelDomains are the domains whose hosts may be allowlisted (PREREG §7.6).
func ModelDomains() []string { return []string{"anthropic.com", "claude.ai"} }

// connectMethod is the request a proxy log entry records for a tunnel.
const connectMethod = "CONNECT"

// LogEntries reads a proxy log: one JSON entry per line.
func LogEntries(log []byte) ([]egress.Entry, error) {
	var entries []egress.Entry
	scanner := bufio.NewScanner(bytes.NewReader(log))
	for number := 1; scanner.Scan(); number++ {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var entry egress.Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", record.EgressFile, number, err)
		}
		entries = append(entries, entry)
	}
	return entries, scanner.Err()
}

// host is the host of a CONNECT target, without its port.
func host(target string) string {
	if name, _, err := net.SplitHostPort(target); err == nil {
		return name
	}
	return target
}

// Hosts lists, sorted and once each, the hosts of the CONNECT entries whose
// decision is one of those given.
func Hosts(entries []egress.Entry, decisions ...string) []string {
	var hosts []string
	for _, entry := range entries {
		if entry.Method == connectMethod && slices.Contains(decisions, entry.Decision) {
			hosts = append(hosts, host(entry.Target))
		}
	}
	slices.Sort(hosts)
	return slices.Compact(hosts)
}

// Observed lists the hosts the session contacted: every CONNECT the proxy
// forwarded, or tried to.
func Observed(entries []egress.Entry) []string {
	return Hosts(entries, egress.DecisionAllowed, egress.DecisionFailed)
}

// InDomains reports whether a host is one of the domains or a subdomain of one.
func InDomains(host string, domains []string) bool {
	for _, domain := range domains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

// Allowlist keeps the observed hosts within the model's domains.
func Allowlist(observed []string) []string {
	var kept []string
	for _, host := range observed {
		if InDomains(host, ModelDomains()) {
			kept = append(kept, host)
		}
	}
	return kept
}

// RefusedBeyond lists the hosts the proxy refused that C1 did not observe.
func RefusedBeyond(entries []egress.Entry, observed []string) []string {
	var beyond []string
	for _, host := range Hosts(entries, egress.DecisionDenied) {
		if !slices.Contains(observed, host) {
			beyond = append(beyond, host)
		}
	}
	return beyond
}

// The probes C2 runs in the session's container once the CLI has exited, and
// the exit code of a command the image does not hold.
const (
	probeCurl       = "curl"
	probeRead       = "read"
	exitNotFound    = 127
	probeSeparator  = "="
	probeTimeoutSec = 20
)

// ProbeScript is what C2's container runs: a direct request to github.com, with
// no proxy variable, and a read of a host path outside the container's mounts;
// it prints each probe's exit code.
func ProbeScript(hostPath string) string {
	return fmt.Sprintf("env -u HTTPS_PROXY -u HTTP_PROXY -u https_proxy -u http_proxy curl --silent --show-error --max-time %d --output /dev/null https://github.com; echo %s%s$?\n"+
		"cat %s > /dev/null; echo %s%s$?\n",
		probeTimeoutSec, probeCurl, probeSeparator, shellQuote(hostPath), probeRead, probeSeparator)
}

func shellQuote(word string) string { return "'" + strings.ReplaceAll(word, "'", `'\''`) + "'" }

// ProbeProblems says which probe of C2 did not fail as it must: each must exit
// non-zero, and curl must be in the image.
func ProbeProblems(output []byte) []string {
	codes := map[string]int{}
	for _, line := range strings.Split(string(output), "\n") {
		name, value, found := strings.Cut(strings.TrimSpace(line), probeSeparator)
		if code, err := strconv.Atoi(value); found && err == nil && (name == probeCurl || name == probeRead) {
			codes[name] = code
		}
	}
	var problems []string
	for _, name := range []string{probeCurl, probeRead} {
		code, ran := codes[name]
		switch {
		case !ran:
			problems = append(problems, "the "+name+" probe did not run")
		case code == 0:
			problems = append(problems, "the "+name+" probe succeeded")
		case code == exitNotFound:
			problems = append(problems, "the "+name+" probe's command is not in the image")
		}
	}
	return problems
}

// InitProblems says how a session's init event falls short: every plugin named
// must be listed and the model must be the session model. The event lists
// plugins by name, path, source and version and carries no hook, so the hooks of
// those plugins are checked against the hook log (HookProblems).
func InitProblems(stream evidence.ClaudeStream, plugins []string, model string) []string {
	init, found := stream.Init()
	if !found {
		return []string{"the stream holds no system/init event"}
	}
	var problems []string
	listed := make([]string, 0, len(init.Init.Plugins))
	for _, plugin := range init.Init.Plugins {
		listed = append(listed, plugin.Name)
	}
	for _, name := range plugins {
		if !slices.Contains(listed, name) {
			problems = append(problems, "system/init does not list the plugin "+name)
		}
	}
	if init.Init.Model != model {
		problems = append(problems, fmt.Sprintf("system/init names the model %q, not %q", init.Init.Model, model))
	}
	return problems
}

// GateProblems says why an acceptance gate's report is not a pass.
func GateProblems(report []byte) []string {
	decoded, err := record.DecodeGateReport(report)
	if err != nil || decoded == nil {
		return []string{fmt.Sprintf("the acceptance gate wrote no report it can be read from (%v)", err)}
	}
	var problems []string
	if decoded.ExitCode != 0 {
		problems = append(problems, fmt.Sprintf("the acceptance gate exited %d with the verdict %q", decoded.ExitCode, decoded.Verdict))
	}
	for _, finding := range decoded.Findings {
		if finding.Blocking {
			problems = append(problems, fmt.Sprintf("blocking finding %s on %s", finding.Category, finding.RequirementID))
		}
	}
	return problems
}

// IsolationFloor is the isolation below which an alignment verdict does not count
// (PREREG §2, `min_isolation`).
const IsolationFloor = format.IsolationRestricted

// IsolationProblems reads the statements of a verified set, each a statement or a
// DSSE envelope of one, and names every alignment verdict whose executor ran
// below the isolation floor; it also returns how many alignment verdicts it read.
func IsolationProblems(files map[string][]byte) ([]string, int) {
	var problems []string
	verdicts := 0
	for _, name := range sortedNames(files) {
		statement := statementBytes(files[name])
		var envelope struct {
			PredicateType string `json:"predicateType"`
		}
		if json.Unmarshal(statement, &envelope) != nil || envelope.PredicateType != format.PredicateTypeAlignment {
			continue
		}
		decoded, err := format.DecodeAs[format.Alignment](statement)
		if err != nil {
			problems = append(problems, name+": "+err.Error())
			continue
		}
		verdicts++
		if isolation := decoded.Predicate.Executor.Isolation; !isolation.AtLeast(IsolationFloor) {
			problems = append(problems, fmt.Sprintf("%s: an alignment verdict ran at isolation %q, below %q", name, isolation, IsolationFloor))
		}
	}
	return problems, verdicts
}

// statementBytes is the statement a file holds, out of its envelope when it has one.
func statementBytes(content []byte) []byte {
	if format.ShapeOfBundle(content) != format.ShapeEnvelope {
		return content
	}
	envelope, err := format.DecodeEnvelope(content)
	if err != nil {
		return content
	}
	statement, err := envelope.StatementBytes()
	if err != nil {
		return content
	}
	return statement
}

func sortedNames(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// AuxiliaryModels are the models a session's usage names beyond its own.
func AuxiliaryModels(usage []string, model string) []string {
	var others []string
	for _, used := range usage {
		if used != model && !slices.Contains(others, used) {
			others = append(others, used)
		}
	}
	slices.Sort(others)
	return others
}
