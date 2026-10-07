package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/bundlefs"
	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/ghrun"
	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/hostgit"
	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/process"
	"github.com/RachidChabane/spec-proof/completeness/internal/version"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/cliconv"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostbundle"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostsync"
)

// Verbs and flags of `host`.
const (
	commandHost = "host"
	verbPush    = "push"
	verbPull    = "pull"

	flagRemote   = "remote"
	flagCIRun    = "ci-run"
	flagRepo     = "repo"
	flagArtifact = "artifact"

	scratchPattern = "spec-proof-pull-"

	// hostModule names the module in the refs and the artifacts of host push and pull.
	hostModule = "completeness"
)

var (
	errHostVerb    = errors.New("host needs one of push or pull")
	errNoCIRun     = errors.New("--ci-run names the CI run whose requests to pull")
	errNothingSent = errors.New("the host store holds no answered request to push")
)

func (e Env) module() format.Module {
	return format.Module{Name: moduleName, Version: version.Version}
}

// hostVerbs are the verbs of `host`.
var hostVerbs = []string{verbPush, verbPull}

func (e Env) host(args []string) outcome {
	if len(args) == 0 {
		return outcome{err: refusal{errHostVerb}}
	}
	verb, rest := args[0], args[1:]
	switch {
	case cliconv.IsHelpArgument(verb) || verb == cliconv.HelpCommand:
		return outcome{err: cliconv.HelpRequest{Text: cliconv.VerbsUsage(e.Binary, commandHost, hostVerbs)}}
	case verb == verbPush:
		return e.hostPush(rest)
	case verb == verbPull:
		return e.hostPull(rest)
	}
	return outcome{err: refusal{errHostVerb}}
}

func (e Env) hostPush(args []string) outcome {
	set := e.newFlags(commandHost+" "+verbPush, "")
	root := set.String(flagRoot, "", usageRoot)
	remote := set.String(flagRemote, "", "also publish the ref to this git remote")
	resolved, err := e.hostRootOf(set, root, args)
	if err != nil {
		return outcome{err: err}
	}
	return e.push(resolved, *remote)
}

func (e Env) hostPull(args []string) outcome {
	set := e.newFlags(commandHost+" "+verbPull, "")
	root := set.String(flagRoot, "", usageRoot)
	run := set.String(flagCIRun, "", "the CI run to pull the issued requests of")
	repository := set.String(flagRepo, "", "the repository of the run, as owner/name")
	artifact := set.String(flagArtifact, hostbundle.IssuedArtifact(hostModule), "the artifact the run uploaded its requests as")
	resolved, err := e.hostRootOf(set, root, args)
	if err != nil {
		return outcome{err: err}
	}
	return e.pull(resolved, *run, *repository, *artifact)
}

// hostRootOf parses the flags of a `host` verb, which takes no argument, and
// resolves the project root.
func (e Env) hostRootOf(set *cliconv.FlagSet, root *string, args []string) (string, error) {
	positionals, err := parseInterleaved(set, args)
	switch {
	case err != nil:
		return "", err
	case len(positionals) > 0:
		return "", errUnexpectedParams
	}
	return e.rootOf(*root)
}

// push publishes the answers of the host store under the ref of the commit the
// working tree is at.
func (e Env) push(root, remote string) outcome {
	ctx := context.Background()
	repository := hostgit.Repository{Process: process.New(), Root: root}
	head, err := repository.HeadSHA(ctx)
	if err != nil {
		return outcome{err: err}
	}
	if dirty, err := repository.HasUncommittedChanges(ctx); err != nil {
		return outcome{err: err}
	} else if dirty {
		fmt.Fprintf(e.Stderr, "%s: the working tree differs from %s: CI recomputes the requests from that commit and will not find the answers of the ones that changed\n", e.Binary, head)
	}
	store, err := e.hostStore(root)
	if err != nil {
		return outcome{err: err}
	}
	bundle, err := hostsync.Collect(store, e.module(), head)
	if err != nil {
		return outcome{err: err}
	}
	if len(bundle.Responses) == 0 {
		return outcome{err: refusal{errNothingSent}}
	}
	files, err := bundle.Files()
	if err != nil {
		return outcome{err: err}
	}
	ref := hostbundle.RefName(hostModule, head)
	if _, err := repository.Publish(ctx, ref, files); err != nil {
		return outcome{err: err}
	}
	if remote != "" {
		if err := repository.PushRef(ctx, remote, ref); err != nil {
			return outcome{err: err}
		}
	}
	fmt.Fprintf(e.Stdout, "pushed %s %d responses\n", ref, len(bundle.Responses))
	return outcome{code: ExitClean}
}

// pull downloads the requests a CI run issued and makes tickets of them.
func (e Env) pull(root, run, repository, artifact string) outcome {
	if run == "" {
		return outcome{err: refusal{errNoCIRun}}
	}
	scratch, err := os.MkdirTemp("", scratchPattern)
	if err != nil {
		return outcome{err: err}
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	client := ghrun.Client{Process: process.New(), Dir: root}
	if err := client.Download(context.Background(), run, artifact, scratch, repository); err != nil {
		return outcome{err: err}
	}
	files, err := bundlefs.Read(scratch)
	if err != nil {
		return outcome{err: err}
	}
	issued, err := hostbundle.DecodeIssued(files)
	if err != nil {
		return outcome{err: refusal{err}}
	}
	store, err := e.hostStore(root)
	if err != nil {
		return outcome{err: err}
	}
	imported, err := hostsync.ImportIssued(store, issued, e.Now, e.Entropy)
	if err != nil {
		return outcome{err: err}
	}
	fmt.Fprintf(e.Stdout, "imported %d requests\n", imported)
	return outcome{code: ExitClean}
}
