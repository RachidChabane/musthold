package hostinstall_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
)

func TestCodexHooksMatcherUpgrade(t *testing.T) {
	eachModule(t, "init replaces the module's pre-tool group that selects the tool names of an earlier spelling and keeps every other group", codexConfigTag, func(t *testing.T, s sample) {
		other := `{"matcher":"Bash","hooks":[{"type":"command","command":"other-tool hook pre","timeout":5}]}`
		stale := `{"matcher":"mcp__` + s.plugin() + `-exec__.*","hooks":[{"type":"command","command":"` +
			s.hookCommand(hostproto.HookPreTool, harness.FlagCodex) + `","timeout":30}]}`
		files := repository(t)
		files.put(t, codexHooks, `{"hooks":{"PreToolUse":[`+other+`,`+stale+`]}}`)
		initCodex(t, s, files)

		var parsed struct {
			Hooks map[string][]struct {
				Matcher string `json:"matcher"`
			} `json:"hooks"`
		}
		if err := json.Unmarshal([]byte(files.text(codexHooks)), &parsed); err != nil {
			t.Fatal(err)
		}
		groups := parsed.Hooks["PreToolUse"]
		if len(groups) != 2 || groups[0].Matcher != "Bash" || groups[1].Matcher != s.harness(harness.FlagCodex).PreToolMatcher() {
			t.Fatalf("PreToolUse groups = %+v", groups)
		}
		if !strings.Contains(files.text(codexHooks), "other-tool hook pre") {
			t.Error("another tool's group was dropped")
		}
	})
}
