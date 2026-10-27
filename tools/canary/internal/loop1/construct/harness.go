package construct

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// CallTimeout is how long one call to the model may run before it counts as a
// failure of the harness.
const CallTimeout = 30 * time.Minute

// ErrHarnessFailure is matched by every failure that comes from the machinery
// around the model (sign-in, quota, network, time limit, the program itself)
// and not from the model's answer. Such a failure uses no repair call and drops
// no domain.
var ErrHarnessFailure = errors.New("the harness failed, not the model")

// HarnessKind names why a call failed on the harness side.
type HarnessKind string

// The kinds of harness failure.
const (
	HarnessUnavailable    HarnessKind = "codex-unavailable"
	HarnessTimeout        HarnessKind = "timeout"
	HarnessNoMessage      HarnessKind = "no-final-message"
	HarnessAuthentication HarnessKind = "authentication"
	HarnessUsageLimit     HarnessKind = "usage-limit"
	HarnessNetwork        HarnessKind = "network"
)

// HarnessFailure is a call that failed on the harness side.
type HarnessFailure struct {
	Kind   HarnessKind
	Detail string
}

func (f *HarnessFailure) Error() string {
	if f.Detail == "" {
		return fmt.Sprintf("harness failure (%s)", f.Kind)
	}
	return fmt.Sprintf("harness failure (%s): %s", f.Kind, f.Detail)
}

// Is makes a HarnessFailure match ErrHarnessFailure.
func (f *HarnessFailure) Is(target error) bool { return target == ErrHarnessFailure }

// harnessTextRules maps the text a failed call prints to the kind of failure it
// shows, in the order the rules apply.
var harnessTextRules = []struct {
	kind    HarnessKind
	pattern *regexp.Regexp
}{
	{HarnessAuthentication, regexp.MustCompile(`not logged in|unauthori[sz]ed|\b401\b|please (log|sign) ?in|invalid (api key|token)`)},
	{HarnessUsageLimit, regexp.MustCompile(`usage limit|rate limit|\b429\b|try again|too many requests`)},
	{HarnessNetwork, regexp.MustCompile(`network|connection (refused|reset|closed|error)|no such host|dial tcp|name resolution|could not resolve|tls handshake|i/o timeout|unreachable|econnreset|etimedout`)},
}

// ClassifyOutcome decides whether a finished `codex exec` call failed on the
// harness side. A call that exited with status zero and wrote a final message
// is the model's answer, whatever its output text says. Any other call is a
// harness failure when its output text shows a sign-in, quota or network
// problem, or when it left no final message after a non-zero exit.
func ClassifyOutcome(result ProcessResult, message []byte) *HarnessFailure {
	empty := len(strings.TrimSpace(string(message))) == 0
	if result.ExitCode == 0 && !empty {
		return nil
	}
	text := strings.ToLower(string(result.Stderr) + "\n" + string(result.Stdout))
	for _, rule := range harnessTextRules {
		if rule.pattern.MatchString(text) {
			return &HarnessFailure{Kind: rule.kind, Detail: fmt.Sprintf("exit %d", result.ExitCode)}
		}
	}
	if result.ExitCode != 0 && empty {
		return &HarnessFailure{Kind: HarnessNoMessage, Detail: fmt.Sprintf("exit %d", result.ExitCode)}
	}
	return nil
}

// interruption says why a call ended before its process did: the caller's own
// cancellation, or the call's time limit, which is a harness failure. It is nil
// when neither happened.
func interruption(parent, call context.Context, limit time.Duration) error {
	switch {
	case parent.Err() != nil:
		return parent.Err()
	case errors.Is(call.Err(), context.DeadlineExceeded):
		return &HarnessFailure{Kind: HarnessTimeout, Detail: fmt.Sprintf("no answer within %s", limit)}
	}
	return nil
}
