package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/completeness/internal/cli"
)

const (
	verifiesHelp       = " [verifies SP-1630~1]"
	helpBinary         = "spec-proof-completeness"
	helpRequestedError = "help requested"
	undefinedFlagError = "flag provided but not defined: -zz-undefined"
)

// commandLines are every command and verb of the binary, as a user types it.
var commandLines = [][]string{
	{"version"}, {"prove"}, {"doctor"}, {"init"}, {"enable"}, {"disable"}, {"mcp"}, {"hook"},
	{"request", "list"}, {"request", "show"}, {"request", "submit"},
	{"systemone", "export"}, {"systemone", "import"}, {"systemone", "classify-plan"},
	{"host", "push"}, {"host", "pull"},
}

func runHelp(arguments ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = cli.Run(cli.Env{Binary: helpBinary, Stdout: &out, Stderr: &errOut}, arguments)
	return code, out.String(), errOut.String()
}

func TestHelpOfTheBinary(t *testing.T) {
	for _, request := range []string{"-h", "--help", "help"} {
		t.Run(request+" prints the commands and succeeds"+verifiesHelp, func(t *testing.T) {
			code, stdout, stderr := runHelp(request)
			if code != cli.ExitClean || stderr != "" || !strings.HasPrefix(stdout, "usage: "+helpBinary+" <version|prove|") {
				t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
			}
		})
	}
	t.Run("help of a command is the help the command gives itself"+verifiesHelp, func(t *testing.T) {
		_, viaHelp, _ := runHelp("help", "prove")
		_, direct, _ := runHelp("prove", "-h")
		if viaHelp != direct || !strings.Contains(direct, "--changed") {
			t.Fatalf("help prove:\n%s\nprove -h:\n%s", viaHelp, direct)
		}
	})
}

func TestHelpOfACommand(t *testing.T) {
	for _, line := range commandLines {
		for _, flag := range []string{"-h", "--help"} {
			name := strings.Join(line, " ") + " " + flag
			t.Run(name+" prints the flags of the command and succeeds"+verifiesHelp, func(t *testing.T) {
				code, stdout, stderr := runHelp(append(append([]string{}, line...), flag)...)
				if code != cli.ExitClean || stderr != "" {
					t.Fatalf("exit %d, stderr %q", code, stderr)
				}
				if !strings.HasPrefix(stdout, "usage: "+helpBinary+" "+strings.Join(line, " ")) || strings.Contains(stdout, helpRequestedError) {
					t.Fatalf("stdout %q", stdout)
				}
			})
		}
	}
}

func TestHelpListsOnlyTheFlagsOfTheCommand(t *testing.T) {
	cases := []struct {
		line          []string
		want, refused []string
	}{
		{[]string{"request", "show"}, []string{"--cap", "--root", "<ticket>"}, []string{"--nonce", "--json", "--source"}},
		{[]string{"request", "list"}, []string{"--json", "--root"}, []string{"--nonce", "--cap"}},
		{[]string{"request", "submit"}, []string{"--nonce", "--cap", "--root"}, []string{"--json", "--source"}},
		{[]string{"host", "push"}, []string{"--remote", "--root"}, []string{"--ci-run", "--repo", "--artifact"}},
		{[]string{"host", "pull"}, []string{"--ci-run", "--repo", "--artifact", "--root"}, []string{"--remote"}},
		{[]string{"systemone", "export"}, []string{"--root"}, []string{"--nonce", "--source"}},
		{[]string{"systemone", "import"}, []string{"--nonce", "--source", "--root"}, []string{"--cap"}},
		{[]string{"systemone", "classify-plan"}, []string{"--model", "--voters", "--root"}, []string{"--nonce", "--source"}},
		{[]string{"prove"}, []string{"--changed", "--all", "--resume", "--record", "--replay", "--ci", "--import-host", "--root"}, []string{"--dir"}},
	}
	for _, c := range cases {
		t.Run(strings.Join(c.line, " ")+" -h lists its own flags and no other"+verifiesHelp, func(t *testing.T) {
			_, stdout, _ := runHelp(append(append([]string{}, c.line...), "-h")...)
			for _, want := range c.want {
				if !strings.Contains(stdout, want) {
					t.Errorf("usage lacks %s:\n%s", want, stdout)
				}
			}
			for _, refused := range c.refused {
				if strings.Contains(stdout, refused) {
					t.Errorf("usage lists %s, which the command does not define:\n%s", refused, stdout)
				}
			}
		})
	}
}

func TestHelpOfACommandWithVerbs(t *testing.T) {
	for _, command := range []string{"request", "systemone", "host"} {
		for _, request := range []string{"-h", "--help", "help"} {
			t.Run(command+" "+request+" lists the verbs"+verifiesHelp, func(t *testing.T) {
				code, stdout, stderr := runHelp(command, request)
				if code != cli.ExitClean || stderr != "" || !strings.HasPrefix(stdout, "usage: "+helpBinary+" "+command+" <") {
					t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
				}
			})
		}
	}
}

func TestCommandLineErrors(t *testing.T) {
	t.Run("an undefined flag is a usage error that names the flag and the usage of its command"+verifiesHelp, func(t *testing.T) {
		code, stdout, stderr := runHelp("request", "show", "--zz-undefined")
		if code != cli.ExitToolFailure || stdout != "" || !strings.Contains(stderr, undefinedFlagError) || !strings.Contains(stderr, "usage: "+helpBinary+" request show") {
			t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
	})
	t.Run("a flag another verb owns is refused by this one"+verifiesHelp, func(t *testing.T) {
		code, _, stderr := runHelp("request", "show", "--nonce", "x")
		if code != cli.ExitToolFailure || !strings.Contains(stderr, "flag provided but not defined: -nonce") {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
	})
	t.Run("an unknown command still prints the commands on the error stream"+verifiesHelp, func(t *testing.T) {
		code, stdout, stderr := runHelp("frobnicate")
		if code != cli.ExitToolFailure || stdout != "" || !strings.Contains(stderr, "usage: "+helpBinary+" <") {
			t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
	})
}
