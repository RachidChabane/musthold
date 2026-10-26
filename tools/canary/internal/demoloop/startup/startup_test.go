package startup_test

import (
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/startup"
)

const (
	initAll       = `{"type":"system","subtype":"init","plugins":[{"name":"musthold-dp"},{"name":"shallnot"},{"name":"musthold"}],"plugin_errors":[]}`
	initUmbrella  = `{"type":"system","subtype":"init","plugins":[{"name":"musthold-dp"}],"plugin_errors":[{"plugin":"musthold@inline","type":"dependency-unsatisfied","message":"Dependency \"shallnot\" is not installed"}]}`
	retryNoServer = `{"type":"system","subtype":"api_retry","attempt":1,"max_retries":10,"error_status":null,"error":"unknown"}`
	retryOverload = `{"type":"system","subtype":"api_retry","attempt":1,"max_retries":10,"error_status":529,"error":"overloaded"}`
	answer        = `{"type":"assistant","message":{"model":"claude-sonnet-5-5","content":[{"type":"text","text":"ready"}]}}`
	synthetic     = `{"type":"assistant","message":{"model":"<synthetic>","content":[{"type":"text","text":"API Error"}]}}`
)

var expected = []string{"musthold-dp", "shallnot", "musthold"}

func feed(check *startup.Check, lines ...string) string {
	for _, line := range lines {
		if reason := check.Line([]byte(line)); reason != "" {
			return reason
		}
	}
	return ""
}

func TestStartup(t *testing.T) {
	t.Run("a start with every plugin loaded and an answer from the API goes on [verifies SP-2419~1]", func(t *testing.T) {
		check := startup.New(expected)
		if reason := feed(check, initAll, answer, retryNoServer, answer); reason != "" {
			t.Errorf("reason %q: %s", reason, check.Problem())
		}
	})

	t.Run("a plugin error or a missing plugin in the init event ends the session, naming them [verifies SP-2419~1]", func(t *testing.T) {
		check := startup.New(expected)
		if reason := feed(check, initUmbrella); reason != startup.ReasonPlugins {
			t.Fatalf("reason %q", reason)
		}
		for _, wanted := range []string{"musthold@inline: dependency-unsatisfied", "plugin shallnot is not loaded", "plugin musthold is not loaded"} {
			if !strings.Contains(check.Problem(), wanted) {
				t.Errorf("the problem lacks %q: %s", wanted, check.Problem())
			}
		}
		if !startup.Failed(startup.ReasonPlugins) || startup.Failed("turn-cap") {
			t.Error("Failed does not tell a failed start")
		}
	})

	t.Run("a retry that reached no server before any answer ends the session at its first attempt [verifies SP-2419~1]", func(t *testing.T) {
		check := startup.New(expected)
		if reason := feed(check, initAll, synthetic, retryNoServer); reason != startup.ReasonUnreachable {
			t.Fatalf("reason %q", reason)
		}
		if !strings.Contains(check.Problem(), "attempt 1") || !strings.Contains(check.Problem(), "no HTTP status") {
			t.Errorf("problem %s", check.Problem())
		}
	})

	t.Run("a retry with an HTTP status reached the API, and the session goes on [verifies SP-2419~1]", func(t *testing.T) {
		check := startup.New(expected)
		if reason := feed(check, initAll, retryOverload, retryNoServer); reason != "" {
			t.Errorf("reason %q: %s", reason, check.Problem())
		}
	})
}
