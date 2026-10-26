package hostinstall_test

import (
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/format/hostproto/harness"
	"github.com/RachidChabane/musthold/format/hostproto/pluginfiles"
)

const (
	codexConfigTag = "[verifies SP-721~1]"

	userTablesBefore = "model = \"gpt-5\"\n\n[projects.\"/work/parcel\"]\ntrust_level = \"trusted\"\n"
	userTablesAfter  = "[mcp_servers.docs]\ncommand = \"docs-server\"\n"
)

// codexBlock is the module's table as init writes it.
func codexBlock(t *testing.T, s sample) string {
	t.Helper()
	files, err := s.module.Files.For(s.harness(harness.FlagCodex))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.Path == pluginfiles.CodexConfigPath {
			return string(file.Content)
		}
	}
	t.Fatal("the Codex files hold no config.toml")
	return ""
}

func initCodex(t *testing.T, s sample, files *memFiles) {
	t.Helper()
	apply(t, files)(initPlan(s, files, s.harness(harness.FlagCodex)))
}

func TestCodexConfigBlock(t *testing.T) {
	eachModule(t, "init creates config.toml holding only the module's table when the file is absent", codexConfigTag, func(t *testing.T, s sample) {
		files := repository(t)
		initCodex(t, s, files)
		if got, want := files.text(pluginfiles.CodexConfigPath), codexBlock(t, s); got != want {
			t.Fatalf("config.toml = %q, want %q", got, want)
		}
	})

	eachModule(t, "init appends the table to a config.toml with the user's own tables before and after it, byte for byte", codexConfigTag, func(t *testing.T, s sample) {
		files := repository(t)
		files.put(t, pluginfiles.CodexConfigPath, userTablesBefore)
		initCodex(t, s, files)
		want := userTablesBefore + "\n" + codexBlock(t, s)
		if got := files.text(pluginfiles.CodexConfigPath); got != want {
			t.Fatalf("config.toml = %q, want %q", got, want)
		}
		files.put(t, pluginfiles.CodexConfigPath, strings.Replace(want, codexBlock(t, s), codexBlock(t, s)+"\n"+userTablesAfter, 1))
		before := files.text(pluginfiles.CodexConfigPath)
		initCodex(t, s, files)
		if got := files.text(pluginfiles.CodexConfigPath); got != before {
			t.Fatalf("a table between the user's tables changed the file: %q", got)
		}
	})

	eachModule(t, "init replaces a stale copy of the table in place and plans no change on a second run", codexConfigTag, func(t *testing.T, s sample) {
		files := repository(t)
		stale := userTablesBefore + "\n" + s.module.Files.CodexConfigBegin() + "\n[mcp_servers.old]\ncommand = \"old\"\n" + s.module.Files.CodexConfigEnd() + "\n\n" + userTablesAfter
		files.put(t, pluginfiles.CodexConfigPath, stale)
		initCodex(t, s, files)
		want := userTablesBefore + "\n" + codexBlock(t, s) + "\n" + userTablesAfter
		if got := files.text(pluginfiles.CodexConfigPath); got != want {
			t.Fatalf("config.toml = %q, want %q", got, want)
		}
		plan, err := initPlan(s, files, s.harness(harness.FlagCodex))
		if err != nil {
			t.Fatal(err)
		}
		if action, _ := actionFor(plan, pluginfiles.CodexConfigPath); action.Change.String() != "unchanged" {
			t.Errorf("second run = %s", action.Change)
		}
	})

	eachModule(t, "init refuses a config.toml whose markers are unbalanced", codexConfigTag, func(t *testing.T, s sample) {
		files := repository(t)
		files.put(t, pluginfiles.CodexConfigPath, s.module.Files.CodexConfigBegin()+"\n")
		if _, err := initPlan(s, files, s.harness(harness.FlagCodex)); err == nil {
			t.Fatal("unbalanced markers were accepted")
		}
	})

	eachModule(t, "disable removes the table and keeps the user's tables, and removes a file the table alone filled", codexConfigTag, func(t *testing.T, s sample) {
		files := repository(t)
		files.put(t, pluginfiles.CodexConfigPath, userTablesBefore+"\n"+userTablesAfter)
		initCodex(t, s, files)
		apply(t, files)(s.module.BuildDisable(files))
		if got, want := files.text(pluginfiles.CodexConfigPath), userTablesBefore+"\n"+userTablesAfter; got != want {
			t.Fatalf("config.toml = %q, want %q", got, want)
		}

		alone := repository(t)
		initCodex(t, s, alone)
		apply(t, alone)(s.module.BuildDisable(alone))
		if got := alone.text(pluginfiles.CodexConfigPath); got != absent {
			t.Fatalf("config.toml = %q, want it removed", got)
		}
	})

	eachModule(t, "enable restores the table of a project that runs the module's Codex hooks", codexConfigTag, func(t *testing.T, s sample) {
		files := prepared(t, s, traceabilityOnly)
		initCodex(t, s, files)
		apply(t, files)(s.module.BuildDisable(files))
		apply(t, files)(s.module.BuildEnable(files))
		if got, want := files.text(pluginfiles.CodexConfigPath), codexBlock(t, s); got != want {
			t.Fatalf("config.toml = %q, want %q", got, want)
		}
	})

	eachModule(t, "enable writes no config.toml in a project without the module's Codex hooks", codexConfigTag, func(t *testing.T, s sample) {
		files := prepared(t, s, traceabilityOnly)
		apply(t, files)(s.module.BuildEnable(files))
		if got := files.text(pluginfiles.CodexConfigPath); got != absent {
			t.Fatalf("config.toml = %q", got)
		}
	})
}
