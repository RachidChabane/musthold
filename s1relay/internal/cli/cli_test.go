package cli_test

import (
	"bytes"
	"testing"

	"github.com/RachidChabane/musthold/s1relay/internal/cli"
	"github.com/RachidChabane/musthold/s1relay/internal/version"
)

const binary = "musthold-s1"

func TestMain(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
	}{
		{"musthold-s1 prints its version", []string{"version"}, cli.ExitClean, binary + " " + version.Version + "\n"},
		{"no argument is a tool failure", nil, cli.ExitToolFailure, ""},
		{"an unknown argument is a tool failure", []string{"frobnicate"}, cli.ExitToolFailure, ""},
	}
	for _, c := range cases {
		t.Run(c.name+" [verifies SP-105~1]", func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := cli.Main(binary, c.args, &stdout, &stderr)
			if code != c.wantCode || stdout.String() != c.wantStdout {
				t.Fatalf("code %d stdout %q, want %d %q", code, stdout.String(), c.wantCode, c.wantStdout)
			}
			if c.wantCode != cli.ExitClean && stderr.Len() == 0 {
				t.Fatal("a failure explains itself on stderr")
			}
		})
	}
}
