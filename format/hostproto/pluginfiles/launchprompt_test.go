package pluginfiles_test

import (
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/pluginfiles"
)

func TestSkillsSpellTheVerifiedLaunchPrompt(t *testing.T) {
	t.Run("the portable skill tells the main agent to pass the prompt the stop hook accepts as bare [verifies SP-1702~1]", func(t *testing.T) {
		prompt := "`" + hostproto.ExecutorPromptOf(hostproto.TicketPlaceholder) + "`"
		for _, sample := range samples() {
			body := portableOf(t, sample.module)[pluginfiles.PortableSkillPath(sample.module.SkillName())]
			if !strings.Contains(body, "with "+prompt+" as the whole prompt") {
				t.Errorf("%s: the skill does not spell the prompt %s:\n%s", sample.name, prompt, body)
			}
		}
	})
}
