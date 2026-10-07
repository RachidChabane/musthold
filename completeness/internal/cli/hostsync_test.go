package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/bundlefs"
	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/hoststore"
	"github.com/RachidChabane/spec-proof/completeness/internal/cli"
	"github.com/RachidChabane/spec-proof/completeness/internal/testkit"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostbundle"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	honestAnswer       = hostprototest.ClosedHonestAnswer
	hostModule         = "completeness"
	editedSource       = "src/invoicing/totals.py"
	pushedFormat       = "pushed refs/spec-proof/host/completeness/"
	issuedArtifactName = "spec-proof-host-completeness"
	issuedCriterion    = "INV-1~1"
	issuedPurpose      = "decision"
)

// hostFixture is a harness whose host store holds the tickets a test put there.
type hostFixture struct {
	*harness
	store *hoststore.Store
}

func newHostFixture(t *testing.T) *hostFixture {
	t.Helper()
	h := newHarness(t, testkit.PythonLedger)
	return &hostFixture{harness: h, store: h.store()}
}

// newHostFixtureWithoutRepository is a host fixture whose project is under no
// version control.
func newHostFixtureWithoutRepository(t *testing.T) *hostFixture {
	t.Helper()
	h := newHarness(t, testkit.PythonLedger)
	h.root = testkit.MaterializeWithoutRepository(t, testkit.PythonLedger)
	return &hostFixture{harness: h, store: h.store()}
}

// emit stores the request and the first emission of one vote of it.
func (f *hostFixture) emit(request hostproto.Request, vote int) hostproto.Ticket {
	f.t.Helper()
	if _, err := f.store.PutRequest(request); err != nil {
		f.t.Fatal(err)
	}
	digest, err := request.SchemaDigest()
	if err != nil {
		f.t.Fatal(err)
	}
	ticket := hostprototest.Ticket(request, vote, hostproto.FirstEmission)
	if err := f.store.PutEntry(hostproto.NewEntry(ticket, request.Role, digest, hostproto.DefaultMaxEmissions)); err != nil {
		f.t.Fatal(err)
	}
	return ticket
}

// runWithEnv runs the command line with some environment variables replaced.
func (f *hostFixture) runWithEnv(overrides map[string]string, args ...string) (int, string, string) {
	f.t.Helper()
	env, stdout, stderr := f.env(nil)
	inner := env.LookupEnv
	env.LookupEnv = func(name string) (string, bool) {
		if value, found := overrides[name]; found {
			return value, true
		}
		return inner(name)
	}
	code := cli.Run(env, args)
	return code, stdout.String(), stderr.String()
}

// git runs the git binary for a test's own set-up and checks, with the user's
// own configuration kept out of it.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false"}, args...)...)
	command.Dir = dir
	command.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@localhost",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@localhost",
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

// commitProject commits everything the project of a harness holds, its change
// included, and returns the commit.
func commitProject(t *testing.T, root string) string {
	t.Helper()
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "project")
	return git(t, root, "rev-parse", "HEAD")
}

// answerFromCLI fetches and answers a ticket with `request show` and `request submit`.
func answerFromCLI(t *testing.T, f *hostFixture, ticket hostproto.Ticket) {
	t.Helper()
	if code, _, stderr := f.run(nil, hostproto.ShowArgs(ticket.TicketID)...); code != cli.ExitClean {
		t.Fatalf("show: %d %s", code, stderr)
	}
	if code, _, stderr := f.run([]byte(honestAnswer), hostproto.SubmitArgs(ticket.TicketID, ticket.Nonce)...); code != cli.ExitClean {
		t.Fatalf("submit: %d %s", code, stderr)
	}
}

func TestHostPush(t *testing.T) {
	t.Run("push publishes the answered tickets under the ref of the head commit [verifies SP-788~1]", func(t *testing.T) {
		f := newHostFixture(t)
		head := commitProject(t, f.root)
		ticket := f.emit(deciderRequest(), 0)
		answerFromCLI(t, f, ticket)
		code, stdout, stderr := f.run(nil, "host", "push")
		if code != cli.ExitClean || !strings.Contains(stdout, pushedFormat+head+" 1 responses") {
			t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
		listing := git(t, f.root, "ls-tree", "-r", "--name-only", hostbundle.RefName(hostModule, head))
		if !strings.Contains(listing, "manifest.json") || !strings.Contains(listing, "responses/"+string(ticket.RequestID)+".0.json") {
			t.Fatalf("tree = %q", listing)
		}
		if status := git(t, f.root, "status", "--porcelain"); strings.Contains(status, "manifest") {
			t.Fatalf("the working tree changed: %q", status)
		}
	})

	t.Run("push warns when the working tree differs from the commit it names [verifies SP-788~1]", func(t *testing.T) {
		f := newHostFixture(t)
		commitProject(t, f.root)
		ticket := f.emit(deciderRequest(), 0)
		answerFromCLI(t, f, ticket)
		writeFile(t, filepath.Join(f.root, filepath.FromSlash(editedSource)), "# edited\n")
		code, _, stderr := f.run(nil, "host", "push")
		if code != cli.ExitClean || !strings.Contains(stderr, "working tree differs") {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
	})

	t.Run("push refuses to publish nothing, and fails outside a repository [verifies SP-788~1]", func(t *testing.T) {
		bare := newHostFixtureWithoutRepository(t)
		if code, _, _ := bare.run(nil, "host", "push"); code != cli.ExitToolFailure {
			t.Errorf("outside a repository exit = %d", code)
		}
		f := newHostFixture(t)
		commitProject(t, f.root)
		if code, _, stderr := f.run(nil, "host", "push"); code != cli.ExitBlocked || !strings.Contains(stderr, "no answered request") {
			t.Errorf("nothing to push: exit %d, %s", code, stderr)
		}
	})

	t.Run("push --remote also publishes the ref to the remote [verifies SP-788~1]", func(t *testing.T) {
		f := newHostFixture(t)
		head := commitProject(t, f.root)
		remote := t.TempDir()
		git(t, remote, "init", "-q", "--bare")
		git(t, f.root, "remote", "add", "origin", remote)
		ticket := f.emit(deciderRequest(), 0)
		answerFromCLI(t, f, ticket)
		if code, _, stderr := f.run(nil, "host", "push", "--remote", "origin"); code != cli.ExitClean {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		if got := git(t, remote, "rev-parse", hostbundle.RefName(hostModule, head)); got == "" {
			t.Fatal("the remote has no ref")
		}
	})

	t.Run("host without a verb, with an unknown one or with a stray argument is refused [verifies SP-788~1]", func(t *testing.T) {
		f := newHostFixture(t)
		if code, _, _ := f.run(nil, "host"); code != cli.ExitBlocked {
			t.Errorf("no verb exit = %d", code)
		}
		if code, _, _ := f.run(nil, "host", "sync"); code != cli.ExitBlocked {
			t.Errorf("unknown verb exit = %d", code)
		}
		if code, _, _ := f.run(nil, "host", "push", "extra"); code != cli.ExitToolFailure {
			t.Errorf("stray argument exit = %d", code)
		}
	})
}

// issuedFiles writes the bundle a CI run uploads for one request.
func issuedFiles(t *testing.T, dir string) hostproto.Request {
	t.Helper()
	request := deciderRequest()
	issued := hostbundle.IssuedBundle{
		Entries:  []hostbundle.IssuedEntry{{RequestID: hostprototest.MustID(request), Votes: 2, Criteria: []string{issuedCriterion}, Purpose: issuedPurpose}},
		Requests: []hostproto.Request{request},
	}
	files, err := issued.Files()
	if err != nil {
		t.Fatal(err)
	}
	if err := bundlefs.Write(dir, files); err != nil {
		t.Fatal(err)
	}
	return request
}

// stubGH puts a gh on the PATH that copies a prepared bundle into the --dir it is
// given, or fails.
func stubGH(t *testing.T, bundleDir string, fail bool) {
	t.Helper()
	bin := t.TempDir()
	body := "#!/bin/sh\nwhile [ \"$#\" -gt 0 ]; do if [ \"$1\" = \"--dir\" ]; then dir=\"$2\"; fi; shift; done\ncp -R \"" + bundleDir + "/.\" \"$dir\"\n"
	if fail {
		body = "#!/bin/sh\necho 'HTTP 404' >&2\nexit 1\n"
	}
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestHostPull(t *testing.T) {
	t.Run("pull downloads the run's requests with gh and makes tickets of them [verifies SP-789~1]", func(t *testing.T) {
		f := newHostFixture(t)
		bundle := t.TempDir()
		request := issuedFiles(t, bundle)
		stubGH(t, bundle, false)
		code, stdout, stderr := f.run(nil, "host", "pull", "--ci-run", "4242")
		if code != cli.ExitClean || !strings.Contains(stdout, "imported 1 requests") {
			t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
		_, listed, _ := f.run(nil, hostproto.ListArgs()...)
		if !strings.Contains(listed, string(hostprototest.MustID(request))) || strings.Count(listed, `"vote"`) != 2 {
			t.Fatalf("listing = %s", listed)
		}
		if code, _, _ := f.run(nil, "host", "pull", "--ci-run", "4242"); code != cli.ExitClean {
			t.Fatalf("a second pull exit = %d", code)
		}
		if _, again, _ := f.run(nil, hostproto.ListArgs()...); strings.Count(again, `"vote"`) != 2 {
			t.Fatalf("a second pull added tickets: %s", again)
		}
	})

	t.Run("pull without a run, with a failing gh or with a bundle that is not one is refused [verifies SP-789~1]", func(t *testing.T) {
		f := newHostFixture(t)
		if code, _, stderr := f.run(nil, "host", "pull"); code != cli.ExitBlocked || !strings.Contains(stderr, "--ci-run") {
			t.Errorf("no run: exit %d, %s", code, stderr)
		}
		stubGH(t, t.TempDir(), true)
		if code, _, stderr := f.run(nil, "host", "pull", "--ci-run", "1"); code != cli.ExitToolFailure || !strings.Contains(stderr, "HTTP 404") {
			t.Errorf("failing gh: exit %d, %s", code, stderr)
		}
		empty := t.TempDir()
		stubGH(t, empty, false)
		if code, _, _ := f.run(nil, "host", "pull", "--ci-run", "1"); code != cli.ExitBlocked {
			t.Errorf("an empty artifact: exit %d", code)
		}
	})

	t.Run("pull asks gh for the artifact of the module, named spec-proof-host-completeness [verifies SP-789~1]", func(t *testing.T) {
		f := newHostFixture(t)
		bin := t.TempDir()
		argsFile := filepath.Join(bin, "args")
		script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argsFile + "\necho 'HTTP 404' >&2\nexit 1\n"
		if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		f.run(nil, "host", "pull", "--ci-run", "7")
		got := readFile(t, argsFile)
		if !strings.Contains(got, issuedArtifactName) || !strings.Contains(got, hostbundle.IssuedArtifact(hostModule)) || !strings.Contains(got, "7") {
			t.Fatalf("gh was called with %q", got)
		}
	})
}
