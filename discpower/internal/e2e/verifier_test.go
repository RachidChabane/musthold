package e2e_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// These helpers publish what the module wrote as a verified set with local trust,
// listing the given criteria as pending, without running the verifier binary: the
// tests that drive one module alone need a set Shallnot reads, not the verifier. They
// are test support and ship nowhere.
const (
	statementsDirectory = ".spec-proof/cache/discriminating-power"
	verifiedDirectory   = ".spec-proof/verified"
	manifestFile        = "manifest.json"
	standInVerifier     = "end-to-end stand-in for spec-proof-verify"
	moduleKind          = "discriminating-power"
	reasonAwaitingHost  = "awaiting_host"
)

type manifestEntry struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

type pendingEntry struct {
	Module    string `json:"module"`
	Criterion string `json:"criterion"`
	Reason    string `json:"reason"`
}

type manifest struct {
	VerifierVersion string          `json:"verifierVersion"`
	Trust           string          `json:"trust"`
	Statements      []manifestEntry `json:"statements"`
	Pending         []pendingEntry  `json:"pending"`
}

// writeVerifiedSet publishes the Statements the module wrote in the cache directory
// as a verified set with local trust, listing the given criteria as pending.
func (p *project) writeVerifiedSet(pending []string) {
	p.t.Helper()
	root := filepath.Join(p.root, filepath.FromSlash(verifiedDirectory))
	if err := os.RemoveAll(root); err != nil {
		p.t.Fatal(err)
	}
	if err := os.MkdirAll(root, directoryMode); err != nil {
		p.t.Fatal(err)
	}
	set := manifest{VerifierVersion: standInVerifier, Trust: "local", Statements: []manifestEntry{}, Pending: []pendingEntry{}}
	source := filepath.Join(p.root, filepath.FromSlash(statementsDirectory))
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		target := filepath.Join(root, "statements", relative)
		if err := os.MkdirAll(filepath.Dir(target), directoryMode); err != nil {
			return err
		}
		if err := os.WriteFile(target, content, fileMode); err != nil {
			return err
		}
		digest := sha256.Sum256(content)
		set.Statements = append(set.Statements, manifestEntry{Path: filepath.ToSlash(filepath.Join("statements", relative)), Digest: hex.EncodeToString(digest[:])})
		return nil
	})
	if err != nil {
		p.t.Fatal(err)
	}
	slices.SortFunc(set.Statements, func(a, b manifestEntry) int { return strings.Compare(a.Path, b.Path) })
	for _, criterion := range slices.Sorted(slices.Values(pending)) {
		set.Pending = append(set.Pending, pendingEntry{Module: moduleKind, Criterion: criterion, Reason: reasonAwaitingHost})
	}
	encoded, err := json.MarshalIndent(set, "", "  ")
	if err != nil {
		p.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, manifestFile), append(encoded, '\n'), fileMode); err != nil {
		p.t.Fatal(err)
	}
}

var criterionLine = regexp.MustCompile(`^(\S+~\d+) (.*)$`)

// verdicts reads the `<criterion> <status> <result>` lines `prove` prints.
func verdicts(output string) map[string]string {
	found := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		if match := criterionLine.FindStringSubmatch(line); match != nil {
			found[match[1]] = strings.TrimSpace(match[2])
		}
	}
	return found
}

// awaiting lists the criteria a `prove` output leaves waiting for the host.
func awaiting(output string) []string {
	var criteria []string
	for criterion, verdict := range verdicts(output) {
		if strings.HasPrefix(verdict, reasonAwaitingHost) {
			criteria = append(criteria, criterion)
		}
	}
	slices.Sort(criteria)
	return criteria
}

// journalEvents reads the journal of the project's host store.
func (p *project) journalEvents() []hostproto.Event {
	p.t.Helper()
	matches, err := filepath.Glob(filepath.Join(p.state, "spec-proof", "discpower", "*", "host", "journal.jsonl"))
	if err != nil || len(matches) != 1 {
		p.t.Fatalf("the host journal: %v, %v", matches, err)
	}
	content, err := os.ReadFile(matches[0])
	if err != nil {
		p.t.Fatal(err)
	}
	var events []hostproto.Event
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		var record hostproto.Record
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			p.t.Fatalf("journal line %q: %v", line, err)
		}
		events = append(events, record.Event)
	}
	return events
}

func (p *project) journalKinds() []hostproto.EventKind {
	var kinds []hostproto.EventKind
	for _, event := range p.journalEvents() {
		kinds = append(kinds, event.Kind)
	}
	return kinds
}

// firstJournaledTicket is a ticket the journal names.
func (p *project) firstJournaledTicket() string {
	for _, event := range p.journalEvents() {
		if event.Ticket != "" {
			return event.Ticket
		}
	}
	p.t.Fatal("the journal names no ticket")
	return ""
}
