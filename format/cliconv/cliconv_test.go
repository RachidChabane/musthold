package cliconv_test

import (
	"bytes"
	"errors"
	"flag"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/format/cliconv"
)

const (
	verifiesHelp = " [verifies SP-1600~1]"
	verifiesLine = " [verifies SP-1601~1]"
)

func provedSet() *cliconv.FlagSet {
	set := cliconv.NewFlagSet("tool prove", "")
	set.Bool("all", false, "prove everything")
	set.String("root", "", "project root")
	set.Int("count", 3, "how many")
	return set
}

func TestHelpRequests(t *testing.T) {
	for _, argument := range []string{"-h", "--h", "-help", "--help"} {
		t.Run("the flag "+argument+" asks for help"+verifiesHelp, func(t *testing.T) {
			if !cliconv.IsHelpArgument(argument) || !cliconv.IsHelpRequest([]string{argument}) {
				t.Fatal("not recognised as a request for help")
			}
		})
	}
	t.Run("the word help asks for help in the place of a command"+verifiesHelp, func(t *testing.T) {
		if !cliconv.IsHelpRequest([]string{"help"}) {
			t.Fatal("not recognised")
		}
	})
	t.Run("a command, an empty command line and a flag value ask for none"+verifiesHelp, func(t *testing.T) {
		for _, arguments := range [][]string{nil, {"prove"}, {"prove", "-h"}, {"--root", "x"}} {
			if cliconv.IsHelpRequest(arguments) {
				t.Errorf("%v taken for a request for help", arguments)
			}
		}
	})
}

func TestFlagSetUsage(t *testing.T) {
	t.Run("the usage names the synopsis and lists exactly the flags of the command"+verifiesHelp, func(t *testing.T) {
		usage := provedSet().Usage()
		for _, want := range []string{"usage: tool prove [flags]", "--all", "prove everything", "--root string", "project root", "--count int", "how many (default 3)"} {
			if !strings.Contains(usage, want) {
				t.Errorf("usage lacks %q:\n%s", want, usage)
			}
		}
		if strings.Contains(usage, "--nonce") {
			t.Errorf("usage lists a flag the command does not define:\n%s", usage)
		}
	})
	t.Run("the usage shows the positional arguments of the command"+verifiesHelp, func(t *testing.T) {
		set := cliconv.NewFlagSet("tool request show", "<ticket>")
		set.String("cap", "", "capability")
		if got := strings.SplitN(set.Usage(), "\n", 2)[0]; got != "usage: tool request show <ticket> [flags]" {
			t.Errorf("synopsis = %q", got)
		}
	})
	t.Run("a command with several forms spells them out in its synopsis"+verifiesHelp, func(t *testing.T) {
		set := cliconv.NewFlagSet("tool", "").WithSynopsis("usage:\n  tool version\n  tool --out DIR\n")
		set.String("out", "", "output directory")
		want := "usage:\n  tool version\n  tool --out DIR\n\nflags:\n  --out string  output directory\n"
		if got := set.Usage(); got != want {
			t.Errorf("usage = %q, want %q", got, want)
		}
	})
	t.Run("a command without flags prints its synopsis only"+verifiesHelp, func(t *testing.T) {
		if got := cliconv.NewFlagSet("tool version", "").Usage(); got != "usage: tool version\n" {
			t.Errorf("usage = %q", got)
		}
	})
	t.Run("the commands and the verbs are listed in order"+verifiesHelp, func(t *testing.T) {
		if got := cliconv.CommandsUsage("tool", []string{"a", "b"}); got != "usage: tool <a|b>\n" {
			t.Errorf("commands = %q", got)
		}
		if got := cliconv.VerbsUsage("tool", "request", []string{"list", "show"}); got != "usage: tool request <list|show>\n" {
			t.Errorf("verbs = %q", got)
		}
	})
}

func TestNeedsVerb(t *testing.T) {
	t.Run("the refusal lists the verbs the command takes"+verifiesHelp, func(t *testing.T) {
		if got := cliconv.NeedsVerb("request", []string{"list", "show", "submit"}); got != "request needs one of list, show or submit" {
			t.Errorf("text = %q", got)
		}
		if got := cliconv.NeedsVerb("host", []string{"push", "pull"}); got != "host needs one of push or pull" {
			t.Errorf("text = %q", got)
		}
		if got := cliconv.NeedsVerb("solo", []string{"only"}); got != "solo needs one of only" {
			t.Errorf("text = %q", got)
		}
	})
}

func TestParse(t *testing.T) {
	t.Run("-h and --help come back as a help request holding the usage of the command"+verifiesHelp, func(t *testing.T) {
		for _, argument := range []string{"-h", "--help", "--all", "-help"} {
			set := provedSet()
			_, err := set.ParseArguments([]string{argument})
			var help cliconv.HelpRequest
			wantHelp := argument != "--all"
			if errors.As(err, &help) != wantHelp {
				t.Fatalf("%s: err = %v", argument, err)
			}
			if wantHelp && help.Text != set.Usage() {
				t.Errorf("%s: text = %q", argument, help.Text)
			}
		}
	})
	t.Run("help after interleaved positionals is still a help request"+verifiesHelp, func(t *testing.T) {
		_, err := provedSet().ParseInterleaved([]string{"x", "-h"})
		var help cliconv.HelpRequest
		if !errors.As(err, &help) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("an undefined flag is a usage error that keeps the flag package's wording"+verifiesHelp, func(t *testing.T) {
		_, err := provedSet().ParseArguments([]string{"--zz-undefined"})
		var usage cliconv.UsageError
		if !errors.As(err, &usage) || !strings.Contains(err.Error(), "flag provided but not defined: -zz-undefined") || usage.Text == "" {
			t.Fatalf("err = %v", err)
		}
		if errors.Is(err, flag.ErrHelp) {
			t.Fatal("a usage error is not a help request")
		}
	})
	t.Run("flags and positionals interleave"+verifiesHelp, func(t *testing.T) {
		set := provedSet()
		positionals, err := set.ParseInterleaved([]string{"a", "--root", "r", "b"})
		if err != nil || strings.Join(positionals, ",") != "a,b" || set.Lookup("root").Value.String() != "r" {
			t.Fatalf("positionals = %v, err = %v", positionals, err)
		}
	})
}

func TestHandle(t *testing.T) {
	t.Run("a help request prints its text on the output stream and succeeds"+verifiesHelp, func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code, handled := cliconv.Handle(cliconv.HelpRequest{Text: "usage: x\n"}, "tool", &stdout, &stderr)
		if !handled || code != cliconv.ExitHelp || stdout.String() != "usage: x\n" || stderr.Len() != 0 {
			t.Fatalf("handled=%v code=%d stdout=%q stderr=%q", handled, code, stdout.String(), stderr.String())
		}
	})
	t.Run("a usage error prints the failure and the usage on the error stream and exits 2"+verifiesHelp, func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code, handled := cliconv.Handle(cliconv.UsageError{Err: errors.New("bad flag"), Text: "usage: x\n"}, "tool", &stdout, &stderr)
		if !handled || code != cliconv.ExitUsage || stdout.Len() != 0 || stderr.String() != "tool: bad flag\nusage: x\n" {
			t.Fatalf("handled=%v code=%d stdout=%q stderr=%q", handled, code, stdout.String(), stderr.String())
		}
	})
	t.Run("another error is left to the caller"+verifiesHelp, func(t *testing.T) {
		if _, handled := cliconv.Handle(errors.New("boom"), "tool", &bytes.Buffer{}, &bytes.Buffer{}); handled {
			t.Fatal("handled")
		}
	})
}

func TestLine(t *testing.T) {
	t.Run("the non-empty fields are joined by one space"+verifiesLine, func(t *testing.T) {
		if got := cliconv.Line("PAR-1~1", "awaiting_host", ""); got != "PAR-1~1 awaiting_host" {
			t.Errorf("line = %q", got)
		}
		if got := cliconv.Line("", "a", "", "b"); got != "a b" {
			t.Errorf("line = %q", got)
		}
	})
	t.Run("a line never starts or ends with a space nor holds two in a row"+verifiesLine, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			fields := rapid.SliceOf(rapid.StringMatching(`[a-z~0-9_]{0,6}`)).Draw(t, "fields")
			line := cliconv.Line(fields...)
			if strings.HasPrefix(line, " ") || strings.HasSuffix(line, " ") || strings.Contains(line, "  ") {
				t.Fatalf("line = %q", line)
			}
		})
	})
}
