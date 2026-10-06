package cli_test

import (
	"bytes"
	"testing"

	"github.com/RachidChabane/spec-proof/internal/cli"
	"github.com/RachidChabane/spec-proof/internal/version"
)

func TestMain(t *testing.T) {
	cases := []struct {
		name       string
		binary     string
		args       []string
		wantCode   int
		wantStdout string
	}{
		{"spec-proof prints its version", "spec-proof", []string{"version"}, cli.ExitClean, "spec-proof " + version.Version + "\n"},
		{"spec-proof-verify prints its version", "spec-proof-verify", []string{"version"}, cli.ExitClean, "spec-proof-verify " + version.Version + "\n"},
		{"no argument is a tool failure", "spec-proof", nil, cli.ExitToolFailure, ""},
		{"an unknown argument is a tool failure", "spec-proof", []string{"frobnicate"}, cli.ExitToolFailure, ""},
	}
	for _, c := range cases {
		t.Run(c.name+" [verifies SP-40~1]", func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := cli.Main(c.binary, c.args, &stdout, &stderr)
			if code != c.wantCode || stdout.String() != c.wantStdout {
				t.Fatalf("code %d stdout %q, want %d %q", code, stdout.String(), c.wantCode, c.wantStdout)
			}
			if c.wantCode != cli.ExitClean && stderr.Len() == 0 {
				t.Fatal("a failure explains itself on stderr")
			}
		})
	}
}
