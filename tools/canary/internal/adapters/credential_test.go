package adapters_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/adapters"
	"github.com/RachidChabane/musthold/tools/canary/internal/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/launch"
)

const (
	keychainToken = "sk-ant-oat01-FAKE-7be1d0a39c5f4482"
	keychainLogin = "someone"
)

// fakeSecurity puts a `security` on the PATH that answers like the macOS tool for
// an item that exists, that does not exist (exit 44) or that fails otherwise. It
// prints the token on its standard output and its error stream in every mode, so a
// test sees whether anything of them leaks. It returns the file that logs the
// arguments of each call.
func fakeSecurity(t *testing.T, mode string) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "` + log + `"
case "` + mode + `" in
present)
  case "$*" in *" -w"*) echo "` + keychainToken + `";; *) echo 'keychain: "login.keychain-db"';; esac
  exit 0;;
absent)
  echo "` + keychainToken + `" >&2
  exit 44;;
*)
  echo "` + keychainToken + `"
  echo "` + keychainToken + `" >&2
  exit 51;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "security"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func keychain() adapters.KeychainCredential {
	return adapters.KeychainCredential{Service: app.KeychainService, Account: keychainLogin}
}

func TestKeychainCredential(t *testing.T) {
	t.Run("a present item is found without its password being asked for, and read with it only on request [verifies SP-1523~1]", func(t *testing.T) {
		log := fakeSecurity(t, "present")
		present, err := keychain().Present(context.Background())
		if err != nil || !present {
			t.Fatalf("present %v, %v", present, err)
		}
		token, err := keychain().Token(context.Background())
		if err != nil || token != keychainToken {
			t.Fatalf("token %q, %v", token, err)
		}
		calls, _ := os.ReadFile(log)
		want := "find-generic-password -s musthold-claude-oauth -a someone\nfind-generic-password -s musthold-claude-oauth -a someone -w\n"
		if string(calls) != want {
			t.Errorf("calls:\n%s\nwant:\n%s", calls, want)
		}
	})

	t.Run("exit 44 is an absent item, not a failure [verifies SP-1523~1]", func(t *testing.T) {
		fakeSecurity(t, "absent")
		present, err := keychain().Present(context.Background())
		if err != nil || present {
			t.Fatalf("present %v, %v", present, err)
		}
		if _, err := keychain().Token(context.Background()); !errors.Is(err, app.ErrCredentialAbsent) {
			t.Errorf("error = %v", err)
		}
	})

	t.Run("any other failure is an error that carries nothing the tool printed [verifies SP-1523~1]", func(t *testing.T) {
		fakeSecurity(t, "failure")
		_, presentErr := keychain().Present(context.Background())
		_, tokenErr := keychain().Token(context.Background())
		for _, err := range []error{presentErr, tokenErr} {
			if err == nil || errors.Is(err, app.ErrCredentialAbsent) || strings.Contains(err.Error(), keychainToken) || !strings.Contains(err.Error(), "51") {
				t.Errorf("error = %v", err)
			}
		}
	})

	t.Run("a missing security tool is an error [verifies SP-1523~1]", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		if _, err := keychain().Present(context.Background()); err == nil {
			t.Error("a missing tool was read as an absent item")
		}
	})

	t.Run("an unknown login is an error, not an absent item [verifies SP-1523~1]", func(t *testing.T) {
		fakeSecurity(t, "present")
		if _, err := (adapters.KeychainCredential{Service: app.KeychainService}).Present(context.Background()); err == nil {
			t.Error("an empty account was queried")
		}
	})

	t.Run("the tool does not inherit the token of the canary's own environment [verifies SP-1523~1]", func(t *testing.T) {
		dir := t.TempDir()
		seen := filepath.Join(dir, "seen")
		script := "#!/bin/sh\nprintf '%s' \"${" + launch.OAuthTokenVariable + "-unset}\" > " + seen + "\nexit 44\n"
		if err := os.WriteFile(filepath.Join(dir, "security"), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
		t.Setenv(launch.OAuthTokenVariable, keychainToken)
		if _, err := keychain().Present(context.Background()); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(seen); string(got) != "unset" {
			t.Errorf("the tool saw %q", got)
		}
	})
}

func TestEnvironmentCredential(t *testing.T) {
	t.Run("the variable is the source when it is set and not empty [verifies SP-1523~1]", func(t *testing.T) {
		set := adapters.EnvironmentCredential{Environ: []string{"A=b", launch.OAuthTokenVariable + "=" + keychainToken}}
		if present, _ := set.Present(context.Background()); !present {
			t.Error("a set variable is absent")
		}
		if token, err := set.Token(context.Background()); err != nil || token != keychainToken {
			t.Errorf("token %q, %v", token, err)
		}
		for _, environ := range [][]string{nil, {launch.OAuthTokenVariable + "="}} {
			empty := adapters.EnvironmentCredential{Environ: environ}
			if present, _ := empty.Present(context.Background()); present {
				t.Errorf("%v is present", environ)
			}
			if _, err := empty.Token(context.Background()); !errors.Is(err, app.ErrCredentialAbsent) {
				t.Errorf("%v: error = %v", environ, err)
			}
		}
	})
}

func TestShimDropsTheToken(t *testing.T) {
	t.Run("a server the shim replaces by the real binary does not see the token either [verifies SP-1523~1]", func(t *testing.T) {
		command := exec.Command(os.Args[0])
		command.Env = append(os.Environ(), helperVariable+"="+helperShimEnv, recordVariable+"="+filepath.Join(t.TempDir(), "hooks.jsonl"), launch.OAuthTokenVariable+"="+keychainToken)
		output, err := command.Output()
		if err != nil || string(output) != "unset" {
			t.Fatalf("output %q, %v", output, err)
		}
	})

	t.Run("a hook the shim runs does not see the token of the session that called it [verifies SP-1523~1]", func(t *testing.T) {
		t.Setenv(launch.OAuthTokenVariable, keychainToken)
		real := fakeBinary(t, `cat >/dev/null; printf '%s' "${`+launch.OAuthTokenVariable+`-unset}"`)
		record := filepath.Join(t.TempDir(), "hooks.jsonl")
		var stdout strings.Builder
		adapters.RunShim(real, record, []string{"hook", "pre-tool"}, strings.NewReader("{}"), &stdout, &strings.Builder{})
		if stdout.String() != "unset" {
			t.Errorf("the hook saw %q", stdout.String())
		}
	})
}
