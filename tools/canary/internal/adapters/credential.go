package adapters

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/launch"
)

const (
	securityBinary = "security"
	// securityItemNotFound is the exit code of `security` for an absent item.
	securityItemNotFound = 44
	// securityTimeout bounds a keychain query, which may wait on a locked keychain.
	securityTimeout = 30 * time.Second
)

// EnvironmentCredential reads the token from the environment of the canary.
type EnvironmentCredential struct{ Environ []string }

// Source implements app.ClaudeCredential.
func (EnvironmentCredential) Source() string { return app.SourceEnvironment }

// Present implements app.ClaudeCredential.
func (c EnvironmentCredential) Present(context.Context) (bool, error) {
	return c.value() != "", nil
}

// Token implements app.ClaudeCredential.
func (c EnvironmentCredential) Token(context.Context) (string, error) {
	if value := c.value(); value != "" {
		return value, nil
	}
	return "", app.ErrCredentialAbsent
}

func (c EnvironmentCredential) value() string {
	value := ""
	for _, pair := range c.Environ {
		if found, ok := strings.CutPrefix(pair, launch.OAuthTokenVariable+"="); ok {
			value = found
		}
	}
	return value
}

// KeychainCredential reads the token from the macOS keychain through `security`.
// Nothing it prints or fails with carries the value or the output of `security`.
type KeychainCredential struct {
	// Service and Account select the keychain item.
	Service, Account string
}

// NewKeychainCredential selects the item the owner stores: the service of the
// canary under the account of the macOS login.
func NewKeychainCredential() KeychainCredential {
	account := ""
	if current, err := user.Current(); err == nil {
		account = current.Username
	}
	return KeychainCredential{Service: app.KeychainService, Account: account}
}

// Source implements app.ClaudeCredential.
func (KeychainCredential) Source() string { return app.SourceKeychain }

// Present implements app.ClaudeCredential. It queries the item without asking for
// its password.
func (c KeychainCredential) Present(ctx context.Context) (bool, error) {
	_, found, err := c.query(ctx, false)
	return found, err
}

// Token implements app.ClaudeCredential.
func (c KeychainCredential) Token(ctx context.Context) (string, error) {
	value, found, err := c.query(ctx, true)
	switch {
	case err != nil:
		return "", err
	case !found || value == "":
		return "", app.ErrCredentialAbsent
	default:
		return value, nil
	}
}

func (c KeychainCredential) query(ctx context.Context, withPassword bool) (value string, found bool, err error) {
	if c.Account == "" {
		return "", false, errors.New("the login name is unknown")
	}
	ctx, cancel := context.WithTimeout(ctx, securityTimeout)
	defer cancel()
	argv := []string{"find-generic-password", "-s", c.Service, "-a", c.Account}
	if withPassword {
		argv = append(argv, "-w")
	}
	command := exec.CommandContext(ctx, securityBinary, argv...)
	command.Env = launch.WithoutOAuthToken(os.Environ())
	command.Stderr = io.Discard
	var output []byte
	if withPassword {
		output, err = command.Output()
	} else {
		command.Stdout = io.Discard
		err = command.Run()
	}
	var exit *exec.ExitError
	switch {
	case err == nil:
		return strings.TrimRight(string(output), "\r\n"), true, nil
	case errors.As(err, &exit) && exit.ExitCode() == securityItemNotFound:
		return "", false, nil
	case errors.As(err, &exit):
		return "", false, fmt.Errorf("%s exited with code %d", securityBinary, exit.ExitCode())
	case ctx.Err() != nil:
		return "", false, fmt.Errorf("%s timed out", securityBinary)
	default:
		return "", false, errors.New(securityBinary + " could not be started")
	}
}
