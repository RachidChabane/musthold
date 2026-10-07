package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/s1relay/internal/cli"
)

const verifiesHelp = " [verifies SP-1660~1]"

func runHelp(arguments ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = cli.Main(binary, arguments, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestHelp(t *testing.T) {
	for _, request := range []string{"-h", "--help", "help"} {
		t.Run(request+" prints the commands and succeeds"+verifiesHelp, func(t *testing.T) {
			code, stdout, stderr := runHelp(request)
			if code != cli.ExitClean || stderr != "" || !strings.HasPrefix(stdout, "usage: "+binary+" <version|drain") {
				t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
			}
		})
	}

	for _, flag := range []string{"-h", "--help"} {
		t.Run("drain "+flag+" prints the flags of drain and succeeds"+verifiesHelp, func(t *testing.T) {
			code, stdout, stderr := runHelp("drain", flag)
			if code != cli.ExitClean || stderr != "" || strings.Contains(stdout, "help requested") {
				t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
			}
			for _, want := range []string{"usage: " + binary + " drain", "--module", "--endpoint", "--model", "--timeout", "--max-requests", "--root", "--mem-guard", "--no-unload"} {
				if !strings.Contains(stdout, want) {
					t.Errorf("usage lacks %q:\n%s", want, stdout)
				}
			}
		})
	}

	t.Run("help drain is the help drain gives itself"+verifiesHelp, func(t *testing.T) {
		_, viaHelp, _ := runHelp("help", "drain")
		_, direct, _ := runHelp("drain", "-h")
		if viaHelp != direct {
			t.Fatalf("help drain:\n%s\ndrain -h:\n%s", viaHelp, direct)
		}
	})

	t.Run("an unknown flag prints the failure and the usage of drain on the error stream and exits 2"+verifiesHelp, func(t *testing.T) {
		code, stdout, stderr := runHelp("drain", "--zz-undefined")
		if code != cli.ExitToolFailure || stdout != "" || !strings.Contains(stderr, "flag provided but not defined: -zz-undefined") || !strings.Contains(stderr, "usage: "+binary+" drain") {
			t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
	})
}
