package hostinstall_test

import (
	"testing"

	"github.com/RachidChabane/musthold/format/hostproto/hostinstall"
)

func TestWithAgentsBlocks(t *testing.T) {
	t.Run("the AGENTS.md rendered for several modules is the one enabling each of them in order leaves [verifies SP-1941~1]", func(t *testing.T) {
		files := repository(t)
		var modules []hostinstall.Module
		for _, s := range samples() {
			apply(t, files)(s.module.BuildEnable(files))
			modules = append(modules, s.module)
		}
		rendered, err := hostinstall.WithAgentsBlocks([]byte(shallnotAgents), modules...)
		if err != nil {
			t.Fatal(err)
		}
		if string(rendered) != files.text(hostinstall.AgentsPath) {
			t.Fatalf("rendered:\n%s\nenabled:\n%s", rendered, files.text(hostinstall.AgentsPath))
		}
	})

	t.Run("rendering again over the rendered file changes nothing, and an empty file takes the blocks alone [verifies SP-1941~1]", func(t *testing.T) {
		var modules []hostinstall.Module
		for _, s := range samples() {
			modules = append(modules, s.module)
		}
		once, err := hostinstall.WithAgentsBlocks(nil, modules...)
		if err != nil {
			t.Fatal(err)
		}
		twice, err := hostinstall.WithAgentsBlocks(once, modules...)
		if err != nil || string(twice) != string(once) || len(once) == 0 {
			t.Fatalf("once %q, twice %q, %v", once, twice, err)
		}
	})

	t.Run("a file whose block of a module is unbalanced is refused [verifies SP-1941~1]", func(t *testing.T) {
		module := samples()[0].module
		broken := "<!-- " + module.Files.Plugin() + ":begin -->\nhalf a block\n"
		if _, err := hostinstall.WithAgentsBlocks([]byte(broken), module); err == nil {
			t.Fatal("an unbalanced block was accepted")
		}
	})
}
