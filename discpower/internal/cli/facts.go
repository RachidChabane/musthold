package cli

import (
	"errors"
	"fmt"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/factsrecord"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/xdg"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
	"github.com/RachidChabane/musthold/format/cliconv"
)

// The `facts` command prints the facts record of a criterion's last Statement.
const (
	commandFacts      = "facts"
	verbShow          = "show"
	argumentCriterion = "ID~REV"
)

// factsVerbs are the verbs of `facts`.
var factsVerbs = []string{verbShow}

func (e Env) facts(args []string) outcome {
	if len(args) == 0 {
		return outcome{err: refusal{errors.New(cliconv.NeedsVerb(commandFacts, factsVerbs))}}
	}
	verb, rest := args[0], args[1:]
	switch {
	case cliconv.IsHelpArgument(verb) || verb == cliconv.HelpCommand:
		return outcome{err: cliconv.HelpRequest{Text: cliconv.VerbsUsage(e.Binary, commandFacts, factsVerbs)}}
	case verb == verbShow:
		return e.factsShow(rest)
	}
	return outcome{err: refusal{fmt.Errorf("unknown facts verb %q", verb)}}
}

func (e Env) factsShow(args []string) outcome {
	set := e.newFlags(commandFacts+" "+verbShow, argumentCriterion)
	root := set.String(flagRoot, "", usageRoot)
	positionals, err := parseInterleaved(set, args)
	switch {
	case err != nil:
		return outcome{err: err}
	case len(positionals) != 1:
		return outcome{err: errUnexpectedParams}
	}
	var criterion spec.Ref
	if err := criterion.UnmarshalText([]byte(positionals[0])); err != nil {
		return outcome{err: refusal{err}}
	}
	resolved, err := e.rootOf(*root)
	if err != nil {
		return outcome{err: err}
	}
	record, err := factsrecord.New(xdg.RecordsDir(e.lookupEnv, e.HomeDir, resolved)).Latest(criterion)
	if errors.Is(err, factsrecord.ErrNoRecord) {
		return outcome{err: refusal{err}}
	}
	if err != nil {
		return outcome{err: err}
	}
	fmt.Fprintln(e.Stdout, string(record))
	return outcome{code: ExitClean}
}
