package app_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/app"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/launch"
)

// fakeToken is a value no fixture, flag or path of a run can contain by chance.
const fakeToken = "sk-ant-oat01-FAKE-0f3a9c71d2b84e56a1c0ffee"

// fakeCredential is a source of the token that counts what is asked of it.
type fakeCredential struct {
	source       string
	token        string
	checkErr     error
	readErr      error
	presentCalls int
	tokenCalls   int
}

func (f *fakeCredential) Source() string { return f.source }

func (f *fakeCredential) Present(context.Context) (bool, error) {
	f.presentCalls++
	return f.token != "" && f.checkErr == nil, f.checkErr
}

func (f *fakeCredential) Token(context.Context) (string, error) {
	f.tokenCalls++
	switch {
	case f.readErr != nil:
		return "", f.readErr
	case f.token == "":
		return "", app.ErrCredentialAbsent
	default:
		return f.token, nil
	}
}

func sources(list ...*fakeCredential) []app.ClaudeCredential {
	converted := make([]app.ClaudeCredential, len(list))
	for index, source := range list {
		converted[index] = source
	}
	return converted
}

func TestClaudeCredentialSources(t *testing.T) {
	t.Run("the environment is read before the keychain and the keychain is not asked when it holds the token [verifies SP-1523~1]", func(t *testing.T) {
		w := newWorld(t, "claude", night)
		environment := &fakeCredential{source: app.SourceEnvironment, token: fakeToken}
		keychain := &fakeCredential{source: app.SourceKeychain, token: "other"}
		w.runner.Credentials = sources(environment, keychain)
		if _, err := w.runner.Run(context.Background(), w.options); err != nil {
			t.Fatal(err)
		}
		if got := environmentOf(w.session.runs[0].Env)[launch.OAuthTokenVariable]; got != fakeToken {
			t.Errorf("the session token = %q", got)
		}
		if keychain.presentCalls != 0 || keychain.tokenCalls != 0 {
			t.Errorf("the keychain was asked: %d checks, %d reads", keychain.presentCalls, keychain.tokenCalls)
		}
	})

	t.Run("the keychain is the source when the environment holds no token [verifies SP-1523~1]", func(t *testing.T) {
		w := newWorld(t, "claude", night)
		w.runner.Credentials = sources(&fakeCredential{source: app.SourceEnvironment}, &fakeCredential{source: app.SourceKeychain, token: fakeToken})
		if _, err := w.runner.Run(context.Background(), w.options); err != nil {
			t.Fatal(err)
		}
		environment := environmentOf(w.session.runs[0].Env)
		if environment[launch.OAuthTokenVariable] != fakeToken || environment["CLAUDE_CONFIG_DIR"] == "" {
			t.Errorf("token %q, config dir %q", environment[launch.OAuthTokenVariable], environment["CLAUDE_CONFIG_DIR"])
		}
	})

	t.Run("a source that fails to answer ends the run before anything starts, and the error carries no token [verifies SP-1523~1]", func(t *testing.T) {
		w := newWorld(t, "claude", night)
		w.runner.Credentials = sources(&fakeCredential{source: app.SourceKeychain, token: fakeToken, checkErr: errors.New("security exited with code 1")})
		_, err := w.runner.Run(context.Background(), w.options)
		if err == nil || !strings.Contains(err.Error(), "security exited with code 1") || strings.Contains(err.Error(), fakeToken) || len(w.processes.calls) != 0 {
			t.Fatalf("error %v, calls %v", err, w.processes.calls)
		}
	})

	t.Run("a token that cannot be read ends the run before the session [verifies SP-1523~1]", func(t *testing.T) {
		w := newWorld(t, "claude", night)
		w.runner.Credentials = sources(&fakeCredential{source: app.SourceKeychain, token: fakeToken, readErr: errors.New("security exited with code 51")})
		_, err := w.runner.Run(context.Background(), w.options)
		if err == nil || !strings.Contains(err.Error(), "code 51") || len(w.session.runs) != 0 {
			t.Fatalf("error %v, sessions %d", err, len(w.session.runs))
		}
	})

	t.Run("the shared home asks no source for anything [verifies SP-1523~1]", func(t *testing.T) {
		w := newWorld(t, "claude", night)
		environment := &fakeCredential{source: app.SourceEnvironment, token: fakeToken}
		w.runner.Credentials = sources(environment)
		w.options.SharedHome = true
		if _, err := w.runner.Run(context.Background(), w.options); err != nil {
			t.Fatal(err)
		}
		if environment.presentCalls != 0 || environment.tokenCalls != 0 {
			t.Errorf("the source was asked: %d checks, %d reads", environment.presentCalls, environment.tokenCalls)
		}
		if _, set := environmentOf(w.session.runs[0].Env)[launch.OAuthTokenVariable]; set {
			t.Error("the shared-home session received the token")
		}
	})

	t.Run("a harness other than Claude Code never asks a source [verifies SP-1523~1]", func(t *testing.T) {
		for _, flag := range []string{"codex", "copilot"} {
			w := newWorld(t, flag, night)
			environment := &fakeCredential{source: app.SourceEnvironment, token: fakeToken}
			w.runner.Credentials = sources(environment)
			if _, err := w.runner.Run(context.Background(), w.options); err != nil {
				t.Fatalf("%s: %v", flag, err)
			}
			if environment.presentCalls != 0 || environment.tokenCalls != 0 {
				t.Errorf("%s asked the source", flag)
			}
		}
	})
}

func TestClaudeTokenReachesTheSessionOnly(t *testing.T) {
	t.Run("only the Claude Code session receives the token, whatever the canary inherited [verifies SP-1523~1]", func(t *testing.T) {
		w := newWorld(t, "claude", night)
		w.runner.Env = append(w.runner.Env, launch.OAuthTokenVariable+"="+fakeToken)
		w.runner.Credentials = sources(&fakeCredential{source: app.SourceKeychain, token: fakeToken})
		if _, err := w.runner.Run(context.Background(), w.options); err != nil {
			t.Fatal(err)
		}
		if environmentOf(w.session.runs[0].Env)[launch.OAuthTokenVariable] != fakeToken {
			t.Fatal("the session lacks the token")
		}
		if len(w.processes.envs) == 0 || len(w.prober.handshakes) == 0 || len(w.prober.specs) == 0 {
			t.Fatal("no support command or probe ran")
		}
		for index, env := range w.processes.envs {
			if _, set := environmentOf(env)[launch.OAuthTokenVariable]; set {
				t.Errorf("the command %v received the token", w.processes.calls[index])
			}
		}
		for _, spec := range append(slices.Clone(w.prober.handshakes), w.prober.specs...) {
			if _, set := environmentOf(spec.Env)[launch.OAuthTokenVariable]; set {
				t.Errorf("the server probe of %s received the token", spec.Binary)
			}
		}
	})

	t.Run("no record, shim, setting, plan or output of an isolated run holds the value of the token [verifies SP-1523~1]", func(t *testing.T) {
		w := newWorld(t, "claude", night)
		w.runner.Env = append(w.runner.Env, launch.OAuthTokenVariable+"="+fakeToken)
		w.runner.Credentials = sources(&fakeCredential{source: app.SourceKeychain, token: fakeToken})
		outcome, err := w.runner.Run(context.Background(), w.options)
		if err != nil {
			t.Fatal(err)
		}
		scanned := 0
		walkErr := filepath.WalkDir(outcome.RunDir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			scanned++
			if bytes.Contains(data, []byte(fakeToken)) {
				t.Errorf("%s holds the token", path)
			}
			return nil
		})
		if walkErr != nil || scanned == 0 {
			t.Fatalf("scanned %d files, %v", scanned, walkErr)
		}
		if strings.Contains(w.stdout.String(), fakeToken) {
			t.Error("the output holds the token")
		}

		dry := newWorld(t, "claude", night)
		dry.runner.Env = append(dry.runner.Env, launch.OAuthTokenVariable+"="+fakeToken)
		dry.runner.Credentials = sources(&fakeCredential{source: app.SourceEnvironment, token: fakeToken})
		dry.options.DryRun = true
		if _, err := dry.runner.Run(context.Background(), dry.options); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(dry.stdout.String(), fakeToken) {
			t.Errorf("the dry run holds the token:\n%s", dry.stdout.String())
		}
	})
}

func TestDryRunReportsTheCredentialSource(t *testing.T) {
	for _, tc := range []struct {
		name    string
		sources []*fakeCredential
		want    string
	}{
		{"environment", []*fakeCredential{{source: app.SourceEnvironment, token: fakeToken}, {source: app.SourceKeychain, token: fakeToken}}, "credentials: source environment (CLAUDE_CODE_OAUTH_TOKEN=<set>)"},
		{"keychain", []*fakeCredential{{source: app.SourceEnvironment}, {source: app.SourceKeychain, token: fakeToken}}, "credentials: source keychain (CLAUDE_CODE_OAUTH_TOKEN=<from keychain>)"},
		{"absent", []*fakeCredential{{source: app.SourceEnvironment}, {source: app.SourceKeychain}}, "credentials: source absent"},
	} {
		t.Run("a dry run names the "+tc.name+" source without reading the token [verifies SP-1524~1]", func(t *testing.T) {
			w := newWorld(t, "claude", night)
			w.runner.Credentials = sources(tc.sources...)
			w.options.DryRun = true
			if _, err := w.runner.Run(context.Background(), w.options); err != nil {
				t.Fatal(err)
			}
			out := w.stdout.String()
			if !strings.Contains(out, tc.want) {
				t.Errorf("the plan lacks %q:\n%s", tc.want, out)
			}
			for _, source := range tc.sources {
				if source.tokenCalls != 0 {
					t.Errorf("the %s token was read", source.source)
				}
			}
			sessionLine := ""
			for _, line := range strings.Split(out, "\n") {
				if strings.Contains(line, "session environment: ") {
					sessionLine = line
				}
			}
			if hasPlaceholder := strings.Contains(sessionLine, launch.OAuthTokenVariable+"=<"); hasPlaceholder == (tc.name == "absent") {
				t.Errorf("the session environment line %q", sessionLine)
			}
		})
	}

	t.Run("a dry run of the shared home says the session uses the credentials of ~/.claude [verifies SP-1524~1]", func(t *testing.T) {
		w := newWorld(t, "claude", night)
		w.options.DryRun, w.options.SharedHome = true, true
		if _, err := w.runner.Run(context.Background(), w.options); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(w.stdout.String(), "credentials: none, the session shares the credentials of ~/.claude") {
			t.Errorf("plan:\n%s", w.stdout.String())
		}
	})
}
