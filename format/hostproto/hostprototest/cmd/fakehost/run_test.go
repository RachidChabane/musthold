package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/format/formattest/parcel"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
)

type scriptedRunner struct {
	listing hostproto.Listing
	runs    [][]string
	stdin   [][]byte
}

func (s *scriptedRunner) Run(args []string, stdin []byte) ([]byte, error) {
	s.runs = append(s.runs, args)
	s.stdin = append(s.stdin, stdin)
	if args[1] == hostproto.SubcommandList {
		return json.Marshal(s.listing)
	}
	return nil, nil
}

func newScripted() (*scriptedRunner, hostproto.Request) {
	request := hostprototest.Request()
	ticket := hostprototest.Ticket(request, 0, 1)
	listing := hostproto.Listing{Items: []hostproto.ListingItem{{Ticket: ticket, Role: request.Role, View: request.ViewFor(ticket)}}}
	return &scriptedRunner{listing: listing}, request
}

func invoke(t *testing.T, runner *scriptedRunner, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	factory := func(string) hostprototest.ModuleRunner { return runner }
	code := run(args, &stdout, &stderr, factory, func(time.Duration) {})
	return code, stdout.String(), stderr.String()
}

func TestFakehostCommand(t *testing.T) {
	t.Run("the personas a command line can play are listed [verifies SP-142~2]", func(t *testing.T) {
		code, stdout, _ := invoke(t, &scriptedRunner{}, "-list")
		if code != exitOK || strings.Fields(stdout)[0] != hostprototest.PersonaHonest || len(strings.Fields(stdout)) != len(hostprototest.CLIPersonaNames()) {
			t.Fatalf("code %d, stdout %q", code, stdout)
		}
	})

	t.Run("a persona answers the pending tickets and each submission is reported [verifies SP-142~2]", func(t *testing.T) {
		runner, _ := newScripted()
		code, stdout, stderr := invoke(t, runner, "-module-bin", "module", "-persona", "honest")
		if code != exitOK {
			t.Fatalf("code %d, stderr %q", code, stderr)
		}
		var result hostprototest.CLIResult
		if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &result); err != nil || !result.Submitted || result.Persona != "honest" {
			t.Fatalf("stdout %q: %+v, %v", stdout, result, err)
		}
		if string(runner.stdin[len(runner.stdin)-1]) != `{"validity":"VALID"}` {
			t.Fatalf("submitted %q", runner.stdin[len(runner.stdin)-1])
		}
	})

	t.Run("recorded answers are read from the recordings directory [verifies SP-142~2]", func(t *testing.T) {
		runner, request := newScripted()
		directory := t.TempDir()
		recorded := `{"validity":"EQUIVALENT"}`
		if err := os.WriteFile(filepath.Join(directory, string(hostprototest.MustID(request))+".json"), []byte(recorded), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "README.txt"), []byte("ignored"), 0o644); err != nil {
			t.Fatal(err)
		}
		code, _, stderr := invoke(t, runner, "-module-bin", "m", "-recordings", directory)
		if code != exitOK || string(runner.stdin[len(runner.stdin)-1]) != recorded {
			t.Fatalf("code %d, stderr %q, submitted %q", code, stderr, runner.stdin[len(runner.stdin)-1])
		}
	})

	t.Run("a request without a recording is answered by the oracle when one is named [verifies SP-142~2]", func(t *testing.T) {
		runner, request := newScripted()
		view := request.ViewFor(hostprototest.Ticket(request, 0, 1))
		directory := t.TempDir()
		bare, err := answerSource(directory, "", "m")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := bare.Answer("honest", view); !errors.Is(err, hostprototest.ErrNoRecording) {
			t.Fatalf("without an oracle: %v", err)
		}
		general, err := answerSource(directory, oracleParcel, "m")
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := general.(withFallback).fallback.(parcel.Oracle); !ok {
			t.Fatalf("a module is answered by %T", general.(withFallback).fallback)
		}
		alignment, err := answerSource(directory, oracleParcel, alignmentBinary)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := alignment.(withFallback).fallback.(parcel.AlignmentOracle); !ok {
			t.Fatalf("alignment is answered by %T", alignment.(withFallback).fallback)
		}
		if code, _, _ := invoke(t, runner, "-module-bin", "m", "-oracle", "saboteur"); code != exitFailure {
			t.Fatalf("an unknown oracle: code %d", code)
		}
	})

	t.Run("a missing binary, an unknown flag, an unreadable directory and a persona that needs hooks are refused [verifies SP-142~2]", func(t *testing.T) {
		runner, _ := newScripted()
		cases := []struct {
			name string
			args []string
			want int
		}{
			{"no binary", []string{"-persona", "honest"}, exitUsage},
			{"an unknown flag", []string{"-bogus"}, exitUsage},
			{"an unreadable directory", []string{"-module-bin", "m", "-recordings", filepath.Join(t.TempDir(), "absent")}, exitFailure},
			{"a persona that needs hooks", []string{"-module-bin", "m", "-persona", "leaker"}, exitUsage},
			{"an unknown persona", []string{"-module-bin", "m", "-persona", "saboteur"}, exitUsage},
		}
		for _, c := range cases {
			if code, _, _ := invoke(t, runner, c.args...); code != c.want {
				t.Errorf("%s: code %d, want %d", c.name, code, c.want)
			}
		}
	})

	t.Run("a failing module makes the command fail [verifies SP-142~2]", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		failing := func(string) hostprototest.ModuleRunner { return failingRunner{} }
		if code := run([]string{"-module-bin", "m"}, &stdout, &stderr, failing, func(time.Duration) {}); code != exitFailure {
			t.Fatalf("code %d", code)
		}
	})

	t.Run("the process runner reports the standard error of a failing binary [verifies SP-142~2]", func(t *testing.T) {
		runner := execRunner("false")
		if _, err := runner.Run(nil, nil); err == nil {
			t.Fatal("a failing binary succeeded")
		}
		if _, err := execRunner(filepath.Join(t.TempDir(), "absent")).Run(nil, nil); err == nil {
			t.Fatal("an absent binary succeeded")
		}
		out, err := execRunner("cat").Run(nil, []byte("echo"))
		if err != nil || string(out) != "echo" {
			t.Fatalf("cat printed %q, %v", out, err)
		}
	})
}

type failingRunner struct{}

func (failingRunner) Run([]string, []byte) ([]byte, error) { return nil, errors.New("boom") }
