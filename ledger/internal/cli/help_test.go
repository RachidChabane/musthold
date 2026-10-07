package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/ledger/internal/cli"
)

const verifiesHelp = " [verifies SP-1650~1]"

// commandLines are every command and subcommand of the binary.
var commandLines = [][]string{
	{"sign"},
	{"ledger", "append"}, {"ledger", "export"}, {"ledger", "audit"}, {"ledger", "seed"}, {"ledger", "fetch-trust"},
}

func runHelp(arguments ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = cli.Main(binary, arguments, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestHelp(t *testing.T) {
	for _, request := range []string{"-h", "--help", "help"} {
		t.Run(request+" prints the forms of the binary and succeeds"+verifiesHelp, func(t *testing.T) {
			code, stdout, stderr := runHelp(request)
			if code != cli.ExitClean || stderr != "" || !strings.Contains(stdout, binary+" ledger append") || !strings.Contains(stdout, binary+" sign") {
				t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
			}
		})
	}

	for _, line := range commandLines {
		for _, flag := range []string{"-h", "--help"} {
			t.Run(strings.Join(line, " ")+" "+flag+" prints the flags of the command and succeeds"+verifiesHelp, func(t *testing.T) {
				code, stdout, stderr := runHelp(append(append([]string{}, line...), flag)...)
				if code != cli.ExitClean || stderr != "" || strings.Contains(stdout, "help requested") {
					t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
				}
				if !strings.HasPrefix(stdout, "usage: "+binary+" "+strings.Join(line, " ")) {
					t.Fatalf("stdout %q", stdout)
				}
			})
		}
	}

	t.Run("a command lists only its own flags"+verifiesHelp, func(t *testing.T) {
		_, stdout, _ := runHelp("ledger", "export", "-h")
		if !strings.Contains(stdout, "--to") || !strings.Contains(stdout, "--repo") || strings.Contains(stdout, "--signed") || strings.Contains(stdout, "--identity") {
			t.Fatalf("stdout %q", stdout)
		}
	})

	t.Run("ledger lists its subcommands and help names a command"+verifiesHelp, func(t *testing.T) {
		_, stdout, _ := runHelp("ledger", "-h")
		if !strings.HasPrefix(stdout, "usage: "+binary+" ledger <append|export|audit|seed|fetch-trust>") {
			t.Fatalf("stdout %q", stdout)
		}
		_, viaHelp, _ := runHelp("help", "sign")
		_, direct, _ := runHelp("sign", "-h")
		if viaHelp != direct {
			t.Fatalf("help sign:\n%s\nsign -h:\n%s", viaHelp, direct)
		}
	})

	t.Run("an unknown flag prints the failure and the usage of its command on the error stream and exits 2"+verifiesHelp, func(t *testing.T) {
		code, stdout, stderr := runHelp("ledger", "audit", "--zz-undefined")
		if code != cli.ExitToolFailure || stdout != "" || !strings.Contains(stderr, "flag provided but not defined: -zz-undefined") || !strings.Contains(stderr, "usage: "+binary+" ledger audit") {
			t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
	})
}
