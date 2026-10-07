package app

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
	"github.com/RachidChabane/spec-proof/format/hostproto/pluginfiles"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/launch"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/modules"
)

// copilotUserDir is the directory of the user's Copilot CLI state in their home.
const copilotUserDir = ".copilot"

const (
	skillManifestName = "SKILL.md"
	notRun            = -1
)

// copilotCredentialSource says where a Copilot CLI session finds its login. The
// canary passes the token variables through unchanged and never prints a value.
func (r Runner) copilotCredentialSource() string {
	for _, name := range launch.CopilotTokenVariables {
		if variable(r.Env, name) != "" {
			return "the " + name + " environment variable, passed through unchanged"
		}
	}
	return "the Copilot CLI login in the system keychain (no token variable is set: " + strings.Join(launch.CopilotTokenVariables, ", ") + ")"
}

// copilotListing runs the no-call listings of the plugins and skills the session
// mounts, with the Copilot CLI environment of the run.
func (r Runner) copilotListing(ctx context.Context, l layout, env []string) *evidence.CopilotListing {
	listing := evidence.CopilotListing{Attempted: true}
	run := func(subject string) (string, int) {
		output, err := r.Processes.Run(ctx, CommandSpec{Argv: launch.CopilotListArgv(l.repo, l.copilotLogs, subject), Dir: l.project, Env: env, Timeout: supportTimeout})
		if err != nil {
			return err.Error(), notRun
		}
		return output.Stdout, output.ExitCode
	}
	listing.PluginsOutput, listing.PluginsExit = run(copilotListSubjects[0])
	listing.SkillsOutput, listing.SkillsExit = run(copilotListSubjects[1])
	return &listing
}

// inspectTrees reads the generated portable tree of each module. A tree that
// cannot be read is left out, so that its check concludes INCONCLUSIVE.
func (r Runner) inspectTrees(l layout) []evidence.PluginTree {
	var trees []evidence.PluginTree
	for _, module := range modules.All() {
		dir := filepath.Join(l.repo, module.PluginDir(harness.FlagPortable))
		if files, readable := r.readTree(dir); readable {
			trees = append(trees, evidence.InspectPluginTree(dir, files))
		}
	}
	return trees
}

func (r Runner) readTree(dir string) (evidence.TreeFiles, bool) {
	var files evidence.TreeFiles
	var err error
	if files.Manifest, err = r.Files.Read(filepath.Join(dir, pluginfiles.PortableManifestPath)); err != nil {
		return files, false
	}
	if files.MCP, err = r.Files.Read(filepath.Join(dir, pluginfiles.PortableMCPPath)); err != nil {
		return files, false
	}
	skills, err := r.Files.Glob(filepath.Join(dir, pluginfiles.PortableSkillsDirectory, "*", skillManifestName))
	if err != nil {
		return files, false
	}
	files.Skills = map[string][]byte{}
	for _, path := range skills {
		content, err := r.Files.Read(path)
		if err != nil {
			return files, false
		}
		files.Skills[filepath.Base(filepath.Dir(path))] = content
	}
	namespaced, err := r.Files.Glob(filepath.Join(dir, pluginfiles.CopilotNamespace, "*", "*"))
	if err != nil {
		return files, false
	}
	for _, path := range namespaced {
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return files, false
		}
		files.Namespace = append(files.Namespace, filepath.ToSlash(relative))
	}
	return files, true
}
