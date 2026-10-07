package hostinstall_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostinstall"
	"github.com/RachidChabane/spec-proof/format/hostproto/pluginfiles"
)

const (
	initTag   = "[verifies SP-717~2]"
	enableTag = "[verifies SP-718~2]"

	configPath    = ".spec-proof/config.yaml"
	shallnotPath  = "shallnot.yaml"
	agentsPath    = "AGENTS.md"
	gitignorePath = ".gitignore"
	settingsPath  = ".claude/settings.json"
	codexHooks    = ".codex/hooks.json"

	// otherProof is a proof kind no sample module owns.
	otherProof = "other-proof"

	advisoryBlockSentence = "The result is advisory"

	traceabilityOnly = `# Shallnot configuration of the parcel service
version: 1
specs:
  - specs

tests:
  - tests   # the test roots
results:
  - build/test-results/*.xml
`
	shallnotAgents     = "<!-- shallnot:begin -->\n## Tracing requirements\n\nRun the gate.\n<!-- shallnot:end -->\n"
	claudeSettingsFile = `{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "command": "shallnot hook claude-stop",
            "timeout": 600,
            "type": "command"
          }
        ]
      }
    ]
  }
}
`
)

// sample is one module the installer is exercised with.
type sample struct {
	name   string
	module hostinstall.Module
}

func (s sample) kind() string   { return s.module.ProofKind }
func (s sample) plugin() string { return s.module.Files.Plugin() }

func (s sample) pluginKey() string { return s.plugin() + "@" + hostinstall.Marketplace }

func (s sample) blockBegin() string { return "<!-- " + s.plugin() + ":begin -->" }
func (s sample) blockEnd() string   { return "<!-- " + s.plugin() + ":end -->" }

// seed is the configuration a new project gets: every proof off.
func (s sample) seed() string {
	return "proofs:\n  " + otherProof + ": false\n  " + s.kind() + ": false\necosystems:\n  - pytest\n"
}

// placeholders replaces {kind} and {other} in a layout.
func (s sample) placeholders(text string) string {
	return strings.NewReplacer("{kind}", s.kind(), "{other}", otherProof).Replace(text)
}

func (s sample) hookCommand(kind hostproto.HookKind, flag string) string {
	selected, err := harness.ByFlag(s.module.Files.Identity, flag)
	if err != nil {
		panic(err)
	}
	return s.module.Files.HookCommand(kind, selected)
}

func (s sample) harness(flag string) harness.Harness {
	selected, err := harness.ByFlag(s.module.Files.Identity, flag)
	if err != nil {
		panic(err)
	}
	return selected
}

func (s sample) agentNames() []string {
	var names []string
	for _, agent := range s.module.Files.Identity.Agents {
		names = append(names, s.plugin()+"-"+string(agent))
	}
	return names
}

// gitignoreAfterInit is what init appends to a .gitignore holding "build/".
func (s sample) gitignoreAfterInit() string {
	text := "build/\n"
	for _, path := range hostinstall.LocalStatePaths {
		text += path + "\n"
	}
	return text
}

// seedWithExample is the configuration init writes: the seed, a blank line and the
// commented example of the operations block.
func (s sample) seedWithExample() string { return s.seed() + "\n" + hostinstall.OperationsExample }

func samples() []sample {
	pair := harness.Identity{Plugin: "spec-proof-pair", Agents: []hostproto.Agent{hostproto.AgentJudge, hostproto.AgentDecider}}
	solo := harness.Identity{Plugin: "spec-proof-solo", Agents: []hostproto.Agent{hostproto.AgentDecider}}
	full := harness.Identity{Plugin: "spec-proof-sample", Agents: hostproto.Agents()}
	return []sample{
		{name: "full", module: hostinstall.Module{
			Files: pluginfiles.Module{
				Identity: full, Binary: "spec-proof-sample", Version: "0.1.0",
				Subject: "sample proof", ProofPhrase: "the sample proof",
			},
			ProofKind: "discriminating-power", ConfigPath: configPath, Required: true,
		}},
		{name: "pair", module: hostinstall.Module{
			Files: pluginfiles.Module{
				Identity: pair, Binary: "spec-proof-pair", Version: "1.2.3",
				Subject: "pair proof", ProofPhrase: "the pair proof",
			},
			ProofKind: "alignment", ConfigPath: configPath, Required: true,
		}},
		{name: "advisory", module: hostinstall.Module{
			Files: pluginfiles.Module{
				Identity: solo, Binary: "spec-proof-solo", Version: "2.0.1",
				Subject: "solo proof", ProofPhrase: "the solo proof",
			},
			ProofKind: "completeness", ConfigPath: configPath, Required: false,
		}},
	}
}

// eachModule runs a subtest per sample module; the sample name precedes the
// traceability tag so the name still ends with it.
func eachModule(t *testing.T, title, tag string, body func(t *testing.T, s sample)) {
	t.Helper()
	for _, s := range samples() {
		t.Run(title+" ("+s.name+") "+tag, func(t *testing.T) { body(t, s) })
	}
}

// repository is a project Shallnot's `init` equipped: traceability only.
func repository(t *testing.T) *memFiles {
	t.Helper()
	files := newMemFiles()
	files.put(t, shallnotPath, traceabilityOnly)
	files.put(t, agentsPath, shallnotAgents)
	files.put(t, settingsPath, claudeSettingsFile)
	files.put(t, gitignorePath, "build/\n")
	return files
}

// apply returns a function that applies a plan the test just built.
func apply(t *testing.T, files *memFiles) func(hostinstall.Plan, error) {
	return func(plan hostinstall.Plan, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		if err := plan.Apply(files); err != nil {
			t.Fatal(err)
		}
	}
}

func initPlan(s sample, files *memFiles, harnesses ...harness.Harness) (hostinstall.Plan, error) {
	return s.module.BuildInit(files, hostinstall.InitOptions{Harnesses: harnesses, Seed: hostinstall.ConfigSeed(s.seed())})
}

// prepared is a repository holding the given shallnot.yaml where init ran for Claude Code.
func prepared(t *testing.T, s sample, shallnot string) *memFiles {
	t.Helper()
	files := repository(t)
	files.put(t, shallnotPath, shallnot)
	apply(t, files)(initPlan(s, files, s.harness(harness.FlagClaude)))
	return files
}

func actionFor(plan hostinstall.Plan, path string) (hostinstall.Action, bool) {
	for _, action := range plan.Actions {
		if action.Path == path {
			return action, true
		}
	}
	return hostinstall.Action{}, false
}

func TestInit(t *testing.T) {
	eachModule(t, "init leaves a traceability-only repository's Shallnot files and AGENTS.md as they were", initTag, func(t *testing.T, s sample) {
		files := repository(t)
		apply(t, files)(initPlan(s, files, s.harness(harness.FlagClaude)))
		if got := files.text(shallnotPath); got != traceabilityOnly {
			t.Errorf("shallnot.yaml changed:\n%s", got)
		}
		if got := files.text(agentsPath); got != shallnotAgents {
			t.Errorf("AGENTS.md changed:\n%s", got)
		}
		if got := files.text(configPath); got != s.seedWithExample() {
			t.Errorf("config.yaml = %q", got)
		}
		if got := files.text(gitignorePath); got != s.gitignoreAfterInit() {
			t.Errorf(".gitignore = %q, want %q", got, s.gitignoreAfterInit())
		}
	})

	eachModule(t, "init enables the plugin in the project settings and keeps Shallnot's stop hook", initTag, func(t *testing.T, s sample) {
		files := repository(t)
		apply(t, files)(initPlan(s, files, s.harness(harness.FlagClaude)))
		settings := files.text(settingsPath)
		if !strings.Contains(settings, `"`+s.pluginKey()+`": true`) || !strings.Contains(settings, "shallnot hook claude-stop") {
			t.Fatalf("settings = %s", settings)
		}
		if strings.Contains(settings, s.module.Files.HookCommandMarker()) {
			t.Fatal("init installed a module hook in the project settings, which the plugin carries")
		}
	})

	eachModule(t, "init enables the plugin in a project that has no settings file", initTag, func(t *testing.T, s sample) {
		files := newMemFiles()
		apply(t, files)(initPlan(s, files, s.harness(harness.FlagClaude)))
		if got := files.text(settingsPath); !strings.Contains(got, `"`+s.pluginKey()+`": true`) {
			t.Fatalf("settings = %s", got)
		}
	})

	eachModule(t, "a choice the user made about the plugin is kept", initTag, func(t *testing.T, s sample) {
		files := repository(t)
		chosen := `{"enabledPlugins":{"` + s.pluginKey() + `":false}}`
		files.put(t, settingsPath, chosen)
		apply(t, files)(initPlan(s, files, s.harness(harness.FlagClaude)))
		if got := files.text(settingsPath); got != chosen {
			t.Fatalf("settings = %s", got)
		}
	})

	eachModule(t, "init is idempotent and a second run plans no change", initTag, func(t *testing.T, s sample) {
		files := repository(t)
		all := harness.All(s.module.Files.Identity)
		apply(t, files)(initPlan(s, files, all...))
		again, err := initPlan(s, files, all...)
		if err != nil || again.Pending() {
			t.Fatalf("pending = %v, error = %v, actions = %+v", again.Pending(), err, again.Actions)
		}
	})

	eachModule(t, "Codex gets read-only agents, a skill and hook entries merged into its hooks file", initTag, func(t *testing.T, s sample) {
		files := repository(t)
		files.put(t, codexHooks, `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"shallnot hook stop","timeout":600}]}]}}`)
		plan, err := initPlan(s, files, s.harness(harness.FlagCodex))
		apply(t, files)(plan, err)
		hooks := files.text(codexHooks)
		wanted := []string{
			"shallnot hook stop",
			s.hookCommand(hostproto.HookPreTool, harness.FlagCodex),
			s.hookCommand(hostproto.HookSubagentStart, harness.FlagCodex),
			s.hookCommand(hostproto.HookSubagentStop, harness.FlagCodex),
		}
		for _, want := range wanted {
			if strings.Count(hooks, want) != 1 {
				t.Errorf("hooks file holds %q %d times", want, strings.Count(hooks, want))
			}
		}
		paths := []string{".agents/skills/" + s.module.Files.SkillName() + "/SKILL.md"}
		for _, name := range s.agentNames() {
			paths = append(paths, ".codex/agents/"+name+".toml")
		}
		for _, path := range paths {
			if files.text(path) == absent {
				t.Errorf("%s was not written", path)
			}
		}
		if !slices.ContainsFunc(plan.NextSteps, func(step string) bool { return strings.Contains(step, "hooks = true") }) {
			t.Errorf("next steps = %q", plan.NextSteps)
		}
	})

	eachModule(t, "Codex hook entries are created when the project has no hooks file", initTag, func(t *testing.T, s sample) {
		files := repository(t)
		apply(t, files)(initPlan(s, files, s.harness(harness.FlagCodex)))
		want := s.hookCommand(hostproto.HookPreTool, harness.FlagCodex)
		if got := files.text(codexHooks); !strings.Contains(got, want) {
			t.Fatalf("hooks = %s", got)
		}
	})

	eachModule(t, "Copilot gets its agents, its skill and a hooks file of its own beside Shallnot's", initTag, func(t *testing.T, s sample) {
		files := repository(t)
		files.put(t, ".github/hooks/shallnot.json", `{"version":1}`)
		apply(t, files)(initPlan(s, files, s.harness(harness.FlagCopilot)))
		paths := []string{".github/skills/" + s.module.Files.SkillName() + "/SKILL.md", ".github/hooks/" + s.plugin() + ".json"}
		for _, name := range s.agentNames() {
			paths = append(paths, ".github/agents/"+name+".agent.md")
		}
		for _, path := range paths {
			if files.text(path) == absent {
				t.Errorf("%s was not written", path)
			}
		}
		if got := files.text(".github/hooks/shallnot.json"); got != `{"version":1}` {
			t.Errorf("Shallnot's hooks file changed: %s", got)
		}
	})

	eachModule(t, "an existing configuration gets the proof's entry and nothing else changes", initTag, func(t *testing.T, s sample) {
		files := repository(t)
		files.put(t, configPath, "# kept\nproofs:\n  "+otherProof+": true\necosystems:\n  - go\nalignment:\n  anything: 1\n")
		apply(t, files)(initPlan(s, files))
		want := "# kept\nproofs:\n  " + otherProof + ": true\n  " + s.kind() + ": false\necosystems:\n  - go\nalignment:\n  anything: 1\n\n" + hostinstall.OperationsExample
		if got := files.text(configPath); got != want {
			t.Fatalf("config = %q", got)
		}
	})

	eachModule(t, "an existing configuration that already has the proof's entry is kept byte for byte", initTag, func(t *testing.T, s sample) {
		files := repository(t)
		original := "# kept\nproofs:\n  " + s.kind() + ": true\n"
		files.put(t, configPath, original)
		apply(t, files)(initPlan(s, files))
		if got := files.text(configPath); got != original {
			t.Fatalf("config = %q", got)
		}
	})

	eachModule(t, "hooks none installs no harness file", initTag, func(t *testing.T, s sample) {
		harnesses, err := s.module.SelectHarnesses(newMemFiles(), "none")
		if err != nil || len(harnesses) != 0 {
			t.Fatalf("harnesses = %v, %v", harnesses, err)
		}
		files := repository(t)
		apply(t, files)(initPlan(s, files))
		if files.text(settingsPath) != claudeSettingsFile {
			t.Fatal("the Claude Code settings changed")
		}
	})

	eachModule(t, "hooks auto selects Claude Code and the harnesses the project shows signs of", initTag, func(t *testing.T, s sample) {
		files := newMemFiles()
		names := func() string {
			harnesses, err := s.module.SelectHarnesses(files, "auto")
			if err != nil {
				t.Fatal(err)
			}
			var flags []string
			for _, selected := range harnesses {
				flags = append(flags, selected.Flag)
			}
			return strings.Join(flags, ",")
		}
		if got := names(); got != "claude" {
			t.Errorf("empty project: %s", got)
		}
		files.put(t, ".codex/config.toml", "")
		files.put(t, ".github/copilot-instructions.md", "")
		if got := names(); got != "claude,codex,copilot" {
			t.Errorf("project with signs: %s", got)
		}
	})

	eachModule(t, "hooks takes a list and refuses a harness the module has no files for", initTag, func(t *testing.T, s sample) {
		harnesses, err := s.module.SelectHarnesses(newMemFiles(), "codex, copilot")
		if err != nil || len(harnesses) != 2 {
			t.Fatalf("harnesses = %v, %v", harnesses, err)
		}
		if _, err := s.module.SelectHarnesses(newMemFiles(), "claude,cursor"); !errors.Is(err, hostinstall.ErrUnknownHookHarness) {
			t.Fatalf("error = %v", err)
		}
	})

	eachModule(t, "a hooks file that is not a JSON object is refused rather than rewritten", initTag, func(t *testing.T, s sample) {
		files := repository(t)
		files.put(t, codexHooks, "[1, 2")
		if _, err := initPlan(s, files, s.harness(harness.FlagCodex)); err == nil {
			t.Fatal("a broken hooks file was planned over")
		}
	})

	eachModule(t, "applying an init plan writes every changed file through the Files port", initTag, func(t *testing.T, s sample) {
		files := repository(t)
		plan, err := initPlan(s, files, s.harness(harness.FlagClaude), s.harness(harness.FlagCodex))
		if err != nil {
			t.Fatal(err)
		}
		if !plan.Pending() {
			t.Fatal("a fresh project plans no change")
		}
		if err := plan.Apply(files); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{configPath, settingsPath, codexHooks} {
			if !slices.Contains(files.writes, path) {
				t.Errorf("%s did not go through Write; writes = %v", path, files.writes)
			}
		}
		if got := files.text(configPath); got != s.seedWithExample() {
			t.Errorf("config.yaml read back = %q", got)
		}
		for _, action := range plan.Actions {
			if action.Change != hostinstall.ChangeNone && files.text(action.Path) != string(action.Content) {
				t.Errorf("%s read back = %q, planned %q", action.Path, files.text(action.Path), action.Content)
			}
		}
	})

	eachModule(t, "applying a plan stops at the first write the port refuses", initTag, func(t *testing.T, s sample) {
		files := repository(t)
		plan, err := initPlan(s, files, s.harness(harness.FlagClaude))
		if err != nil {
			t.Fatal(err)
		}
		refused := errors.New("disk full")
		files.failWrites = refused
		if err := plan.Apply(files); !errors.Is(err, refused) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestEnableAndDisable(t *testing.T) {
	enable := func(t *testing.T, s sample, files *memFiles) {
		t.Helper()
		apply(t, files)(s.module.BuildEnable(files))
	}
	disable := func(t *testing.T, s sample, files *memFiles) {
		t.Helper()
		apply(t, files)(s.module.BuildDisable(files))
	}

	eachModule(t, "enable adds a required proof to proofs.require at the end of a file that has no proofs block and an advisory proof writes nothing there", enableTag, func(t *testing.T, s sample) {
		files := prepared(t, s, traceabilityOnly)
		enable(t, s, files)
		want := traceabilityOnly
		if s.module.Required {
			want += "proofs:\n  require:\n    - " + s.kind() + "\n  trust: local\n"
		}
		if got := files.text(shallnotPath); got != want {
			t.Fatalf("shallnot.yaml = %q", got)
		}
	})

	eachModule(t, "enable turns the proof on in the configuration and writes its block of AGENTS.md beside Shallnot's", enableTag, func(t *testing.T, s sample) {
		files := prepared(t, s, traceabilityOnly)
		enable(t, s, files)
		if got := files.text(configPath); !strings.Contains(got, s.kind()+": true") || strings.Contains(got, otherProof+": true") {
			t.Errorf("config = %q", got)
		}
		agents := files.text(agentsPath)
		if !strings.HasPrefix(agents, shallnotAgents+"\n") || !strings.Contains(agents, s.module.Files.Binary+" prove --changed") ||
			strings.Count(agents, s.blockBegin()) != 1 || strings.Count(agents, s.blockEnd()) != 1 {
			t.Errorf("AGENTS.md = %q", agents)
		}
		if !strings.Contains(agents, "## "+s.plugin()+": "+s.module.Files.Subject) {
			t.Errorf("AGENTS.md lacks the block heading: %q", agents)
		}
	})

	eachModule(t, "the AGENTS.md block says the result is advisory only for a proof Shallnot cannot require", enableTag, func(t *testing.T, s sample) {
		files := prepared(t, s, traceabilityOnly)
		enable(t, s, files)
		agents := files.text(agentsPath)
		if s.module.Required {
			if strings.Contains(agents, "advisory") {
				t.Errorf("a required proof's block says advisory: %q", agents)
			}
			return
		}
		if !strings.Contains(agents, advisoryBlockSentence) {
			t.Errorf("an advisory proof's block lacks the advisory sentence: %q", agents)
		}
		block := agents[strings.Index(agents, s.blockBegin()):]
		if !strings.Contains(block, advisoryBlockSentence) {
			t.Errorf("the advisory sentence is outside the module's block: %q", agents)
		}
	})

	eachModule(t, "enable tells that the gate reads local proofs and CI signed ones for a required proof, and what is advisory for the other", enableTag, func(t *testing.T, s sample) {
		files := prepared(t, s, traceabilityOnly)
		plan, err := s.module.BuildEnable(files)
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(plan.NextSteps, "\n")
		if s.module.Required {
			if !strings.Contains(joined, "proofs.trust is local") || !strings.Contains(joined, "CI configuration of the gate sets proofs.trust to signed") {
				t.Errorf("next steps = %q", plan.NextSteps)
			}
			if !strings.Contains(joined, "no source root was detected for proofs.code") {
				t.Errorf("a project with no layout gets no note about proofs.code: %q", plan.NextSteps)
			}
			return
		}
		if !strings.Contains(joined, s.kind()+" is advisory") || !strings.Contains(joined, "enabling it writes no require entry") {
			t.Errorf("next steps = %q", plan.NextSteps)
		}
	})

	eachModule(t, "enabling an advisory proof plans no change to shallnot.yaml, even when the file is absent", enableTag, func(t *testing.T, s sample) {
		if s.module.Required {
			t.Skip("applies to an advisory proof")
		}
		files := newMemFiles()
		plan, err := s.module.BuildEnable(files)
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if _, planned := actionFor(plan, shallnotPath); planned {
			t.Errorf("shallnot.yaml is in the plan: %+v", plan.Actions)
		}
		apply(t, files)(plan, nil)
		if got := files.text(shallnotPath); got != absent {
			t.Errorf("shallnot.yaml = %q", got)
		}
		if got := files.text(configPath); !strings.Contains(got, s.kind()+": true") {
			t.Errorf("config = %q", got)
		}
	})

	eachModule(t, "disabling an advisory proof never touches shallnot.yaml", enableTag, func(t *testing.T, s sample) {
		if s.module.Required {
			t.Skip("applies to an advisory proof")
		}
		files := prepared(t, s, "proofs: [not a mapping\n")
		plan, err := s.module.BuildDisable(files)
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if _, planned := actionFor(plan, shallnotPath); planned {
			t.Errorf("shallnot.yaml is in the plan: %+v", plan.Actions)
		}
	})

	eachModule(t, "enable extends an existing proofs block in each layout and disable takes the require entry out and leaves what else the block holds", enableTag, func(t *testing.T, s sample) {
		type layout struct{ before, enabled, disabled string }
		layouts := map[string]layout{
			"a block without require": {
				before:  "version: 1 # keep\nproofs:\n  trust: local\n  code:\n    - parcel/**\ntests:\n  - tests\n",
				enabled: "version: 1 # keep\nproofs:\n  trust: local\n  code:\n    - parcel/**\n  require:\n    - {kind}\ntests:\n  - tests\n",
			},
			"a block list": {
				before:  "proofs:\n  require:\n    - {other} # first\n  trust: local\n",
				enabled: "proofs:\n  require:\n    - {other} # first\n    - {kind}\n  trust: local\n",
			},
			"a flow list": {
				before:  "proofs:\n  require: [{other}]\n  trust: local\n",
				enabled: "proofs:\n  require: [{other}, {kind}]\n  trust: local\n",
			},
			"an empty value": {
				before:   "proofs:\n  require:\n  trust: local\n",
				enabled:  "proofs:\n  require:\n    - {kind}\n  trust: local\n",
				disabled: "proofs:\n  trust: local\n",
			},
			"four-space indentation": {
				before:  "proofs:\n    trust: local\n",
				enabled: "proofs:\n    trust: local\n    require:\n      - {kind}\n",
			},
			"no trailing newline": {
				before:   "version: 1\nspecs: [specs]",
				enabled:  "version: 1\nspecs: [specs]\nproofs:\n  require:\n    - {kind}\n  trust: local\n",
				disabled: "version: 1\nspecs: [specs]\nproofs:\n  trust: local\n",
			},
		}
		for name, l := range layouts {
			before := s.placeholders(l.before)
			wantEnabled, wantDisabled := s.placeholders(l.enabled), before
			if l.disabled != "" {
				wantDisabled = s.placeholders(l.disabled)
			}
			if !s.module.Required {
				wantEnabled, wantDisabled = before, before
			}
			files := prepared(t, s, before)
			agentsBefore, configBefore := files.text(agentsPath), files.text(configPath)
			enable(t, s, files)
			if got := files.text(shallnotPath); got != wantEnabled {
				t.Errorf("%s: enabled = %q, want %q", name, got, wantEnabled)
			}
			disable(t, s, files)
			if got := files.text(shallnotPath); got != wantDisabled {
				t.Errorf("%s: disabled = %q, want %q", name, got, wantDisabled)
			}
			if got := files.text(agentsPath); got != agentsBefore {
				t.Errorf("%s: AGENTS.md = %q, want %q", name, got, agentsBefore)
			}
			if got := files.text(configPath); got != configBefore {
				t.Errorf("%s: config = %q, want %q", name, got, configBefore)
			}
		}
	})

	eachModule(t, "enable and disable are idempotent", enableTag, func(t *testing.T, s sample) {
		files := prepared(t, s, traceabilityOnly)
		enable(t, s, files)
		if again, err := s.module.BuildEnable(files); err != nil || again.Pending() {
			t.Fatalf("second enable pending = %v, %v", again.Pending(), err)
		}
		disable(t, s, files)
		if again, err := s.module.BuildDisable(files); err != nil || again.Pending() {
			t.Fatalf("second disable pending = %v, %v", again.Pending(), err)
		}
	})

	eachModule(t, "a project that has no shallnot.yaml cannot enable a required proof", enableTag, func(t *testing.T, s sample) {
		_, err := s.module.BuildEnable(newMemFiles())
		if s.module.Required {
			if !errors.Is(err, hostinstall.ErrNoShallnotConfig) {
				t.Fatalf("error = %v", err)
			}
			return
		}
		if err != nil {
			t.Fatalf("an advisory proof needs no shallnot.yaml: %v", err)
		}
	})

	eachModule(t, "disable in a project that never enabled anything changes nothing", enableTag, func(t *testing.T, s sample) {
		plan, err := s.module.BuildDisable(repository(t))
		if err != nil || plan.Pending() {
			t.Fatalf("pending = %v, %v", plan.Pending(), err)
		}
	})

	eachModule(t, "an AGENTS.md that held only the module's block is removed on disable", enableTag, func(t *testing.T, s sample) {
		files := newMemFiles()
		files.put(t, shallnotPath, traceabilityOnly)
		apply(t, files)(s.module.BuildEnable(files))
		if files.text(agentsPath) == absent {
			t.Fatal("AGENTS.md was not created")
		}
		apply(t, files)(s.module.BuildDisable(files))
		if got := files.text(agentsPath); got != absent {
			t.Fatalf("AGENTS.md = %q", got)
		}
		if !slices.Contains(files.removes, agentsPath) {
			t.Errorf("AGENTS.md was not removed through the port; removes = %v", files.removes)
		}
	})

	eachModule(t, "a file with other text keeps it when the module's block is removed", enableTag, func(t *testing.T, s sample) {
		files := prepared(t, s, traceabilityOnly)
		enable(t, s, files)
		disable(t, s, files)
		if got := files.text(agentsPath); got != shallnotAgents {
			t.Fatalf("AGENTS.md = %q", got)
		}
	})

	eachModule(t, "unbalanced markers are refused", enableTag, func(t *testing.T, s sample) {
		for name, text := range map[string]string{
			"only a begin marker": s.blockBegin() + "\ntext\n",
			"only an end marker":  "text\n" + s.blockEnd() + "\n",
		} {
			files := prepared(t, s, traceabilityOnly)
			files.put(t, agentsPath, text)
			if _, err := s.module.BuildEnable(files); err == nil {
				t.Errorf("%s: enable accepted unbalanced markers", name)
			}
			if _, err := s.module.BuildDisable(files); err == nil {
				t.Errorf("%s: disable accepted unbalanced markers", name)
			}
		}
	})

	eachModule(t, "a proofs block in flow style is refused where the module edits it", enableTag, func(t *testing.T, s sample) {
		files := prepared(t, s, "proofs: {trust: local}\n")
		_, err := s.module.BuildEnable(files)
		if s.module.Required {
			if !errors.Is(err, hostinstall.ErrUnsupportedYAML) {
				t.Errorf("flow mapping in shallnot.yaml: %v", err)
			}
		} else if err != nil {
			t.Errorf("an advisory proof never edits shallnot.yaml: %v", err)
		}
		files = prepared(t, s, traceabilityOnly)
		files.put(t, configPath, "proofs: {"+s.kind()+": false}\n")
		if _, err := s.module.BuildEnable(files); !errors.Is(err, hostinstall.ErrUnsupportedYAML) {
			t.Errorf("flow mapping in the configuration: %v", err)
		}
	})

	eachModule(t, "a non-boolean flag is refused", enableTag, func(t *testing.T, s sample) {
		files := prepared(t, s, traceabilityOnly)
		files.put(t, configPath, "proofs:\n  "+s.kind()+": maybe\n")
		if _, err := s.module.BuildEnable(files); !errors.Is(err, hostinstall.ErrUnsupportedYAML) {
			t.Errorf("non-boolean flag: %v", err)
		}
	})

	eachModule(t, "the replacement of the module's block keeps the text around it", enableTag, func(t *testing.T, s sample) {
		files := prepared(t, s, traceabilityOnly)
		files.put(t, agentsPath, "before\n\n"+s.blockBegin()+"\nold\n"+s.blockEnd()+"\nafter\n")
		enable(t, s, files)
		updated := files.text(agentsPath)
		if !strings.HasPrefix(updated, "before\n\n") || !strings.HasSuffix(updated, "\nafter\n") || strings.Contains(updated, "old") ||
			strings.Count(updated, s.blockBegin()) != 1 {
			t.Fatalf("updated = %q", updated)
		}
		disable(t, s, files)
		if got := files.text(agentsPath); got != "before\n\nafter\n" {
			t.Fatalf("removed = %q", got)
		}
	})

	eachModule(t, "applying an enable plan writes through the Files port and a disable plan removes through it", enableTag, func(t *testing.T, s sample) {
		files := newMemFiles()
		files.put(t, shallnotPath, traceabilityOnly)
		plan, err := s.module.BuildEnable(files)
		if err != nil {
			t.Fatal(err)
		}
		if err := plan.Apply(files); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{configPath, agentsPath} {
			if !slices.Contains(files.writes, path) {
				t.Errorf("%s did not go through Write; writes = %v", path, files.writes)
			}
			action, planned := actionFor(plan, path)
			if !planned || action.Change != hostinstall.ChangeCreate || files.text(path) != string(action.Content) {
				t.Errorf("%s: action = %+v, read back %q", path, action, files.text(path))
			}
		}
		disablePlan, err := s.module.BuildDisable(files)
		if err != nil {
			t.Fatal(err)
		}
		if action, planned := actionFor(disablePlan, agentsPath); !planned || action.Change != hostinstall.ChangeDelete {
			t.Errorf("AGENTS.md action = %+v", action)
		}
	})
}
