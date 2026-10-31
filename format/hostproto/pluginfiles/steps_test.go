package pluginfiles_test

import (
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/harness"
)

const verifiesSteps = " [verifies SP-2669~1]"

func TestFulfilSkillSteps(t *testing.T) {
	for _, s := range samples() {
		module := s.module
		for _, host := range []harness.Harness{harness.Claude(module.Identity), harness.Codex(module.Identity)} {
			t.Run(s.name+" "+host.Flag+": the loop runs bounded steps, continues an unfinished one and renews an abandonment once"+verifiesSteps, func(t *testing.T) {
				_, body := frontMatter(t, generated(t, module, host)[skillPath(host.Flag, module)])
				for _, want := range []string{
					module.Binary + " prove --resume` in the foreground", "within 90 seconds", "exit code 3",
					"`unfinished budget_spent`", "`unfinished session_busy`", "`in_flight <n> tickets`",
					"wait for the executors you started", module.Binary + " prove --changed` once", "a second `host_unresponsive` finding ends the loop",
				} {
					if !strings.Contains(body, want) {
						t.Errorf("the skill lacks %q", want)
					}
				}
				if strings.Contains(body, "or a `host_unresponsive` finding, report it") {
					t.Error("the skill still stops the loop at the first host_unresponsive finding")
				}
			})
		}
	}

	t.Run("a resume is accepted for a session that holds a request or left a wave open, refused otherwise"+verifiesSteps, func(t *testing.T) {
		prove := hostproto.SessionProve
		held := []hostproto.QueueItem{{RequestID: "a", Sessions: []hostproto.Session{prove}}}
		cases := []struct {
			name   string
			states []hostproto.SessionState
			items  []hostproto.QueueItem
			want   bool
		}{
			{"a new store", nil, nil, false},
			{"a held request", nil, held, true},
			{"a first step cut before it asked", []hostproto.SessionState{{Session: prove, Wave: 1}}, nil, true},
			{"an ended session", []hostproto.SessionState{{Session: prove, Wave: 3, Ended: true}}, nil, false},
		}
		for _, c := range cases {
			if got := hostproto.Resumable(c.states, c.items, prove); got != c.want {
				t.Errorf("%s: resumable %v", c.name, got)
			}
		}
	})
}
