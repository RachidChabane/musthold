package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/rawlog"
	"io"
	"strings"

	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/responsecheck"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/cliconv"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/requests"
)

// Flags of `request` and `systemone`.
const (
	flagJSON         = "json"
	flagNonce        = "nonce"
	flagCap          = "cap"
	flagSource       = "source"
	flagModel        = "model"
	flagVoters       = "voters"
	flagSessionModel = "session-model"
	flagSession      = "session"
	// Verbs of `systemone`.
	verbExport       = "export"
	verbImport       = "import"
	verbClassifyPlan = "classify-plan"
)

// Usage of the flags of `systemone classify-plan`.
const (
	usageClassifyModel        = "the model that answers all five classifier calls"
	usageClassifyVoters       = "the models that answer the five calls, as <model>*<n>[+<model>*<n>]"
	usageClassifySessionModel = "the model of the session, which an admission entry names; defaults to --model"
)

// usageSession is the usage of `--session` of `request list` and `systemone export`.
const usageSession = "list the tickets of this session only, prove or scan:<key> (default: every session)"

var errMissingTicket = refusal{errors.New("a ticket identifier is required")}

// sessionFilterOf reads `--session`: no session selects the tickets of every
// session, and a text that names no session is refused.
func sessionFilterOf(text string) (hostproto.SessionFilter, error) {
	filter, err := hostproto.ParseSessionFilter(text)
	if err != nil {
		return filter, refusal{err}
	}
	return filter, nil
}

func (e Env) requestService(root string) (requests.Service, error) {
	store, err := e.hostStore(root)
	if err != nil {
		return requests.Service{}, err
	}
	return requests.Service{Plugin: pluginName, Store: store, Validator: responsecheck.New(), Clock: e.Now}, nil
}

// requestVerbs are the verbs of `request`.
var requestVerbs = []string{hostproto.SubcommandList, hostproto.SubcommandShow, hostproto.SubcommandSubmit}

func (e Env) request(args []string) outcome {
	if len(args) == 0 {
		return outcome{err: refusal{errors.New(cliconv.NeedsVerb(commandRequest, requestVerbs))}}
	}
	verb, rest := args[0], args[1:]
	switch {
	case cliconv.IsHelpArgument(verb) || verb == cliconv.HelpCommand:
		return outcome{err: cliconv.HelpRequest{Text: cliconv.VerbsUsage(e.Binary, commandRequest, requestVerbs)}}
	case verb == hostproto.SubcommandList:
		return e.requestList(rest)
	case verb == hostproto.SubcommandShow:
		return e.requestShow(rest)
	case verb == hostproto.SubcommandSubmit:
		return e.requestSubmit(rest)
	}
	return outcome{err: refusal{fmt.Errorf("unknown request verb %q", verb)}}
}

func (e Env) requestList(args []string) outcome {
	set := e.newFlags(commandRequest+" "+hostproto.SubcommandList, "")
	asJSON := set.Bool(flagJSON, false, "print the listing as JSON")
	session := set.String(flagSession, "", usageSession)
	root := set.String(flagRoot, "", usageRoot)
	rest, err := parseInterleaved(set, args)
	switch {
	case err != nil:
		return outcome{err: err}
	case len(rest) > 0:
		return outcome{err: errUnexpectedParams}
	}
	filter, err := sessionFilterOf(*session)
	if err != nil {
		return outcome{err: err}
	}
	service, err := e.requestService(mustRoot(e, *root))
	if err != nil {
		return outcome{err: err}
	}
	return e.list(service, *asJSON, filter)
}

func (e Env) requestShow(args []string) outcome {
	set := e.newFlags(commandRequest+" "+hostproto.SubcommandShow, argumentTicket)
	capability := set.String(flagCap, "", usageCap)
	root := set.String(flagRoot, "", usageRoot)
	positionals, err := parseInterleaved(set, args)
	if err != nil {
		return outcome{err: err}
	}
	service, err := e.requestService(mustRoot(e, *root))
	if err != nil {
		return outcome{err: err}
	}
	return e.show(service, positionals, *capability)
}

func (e Env) requestSubmit(args []string) outcome {
	set := e.newFlags(commandRequest+" "+hostproto.SubcommandSubmit, argumentTicket)
	nonce := set.String(flagNonce, "", usageNonce)
	capability := set.String(flagCap, "", usageCap)
	root := set.String(flagRoot, "", usageRoot)
	positionals, err := parseInterleaved(set, args)
	if err != nil {
		return outcome{err: err}
	}
	service, err := e.requestService(mustRoot(e, *root))
	if err != nil {
		return outcome{err: err}
	}
	return e.submit(service, positionals, *nonce, *capability)
}

// mustRoot resolves the root; a failure leaves the working directory, which the
// store opening then reports on.
func mustRoot(e Env, given string) string {
	root, err := e.rootOf(given)
	if err != nil {
		return e.WorkDir
	}
	return root
}

func (e Env) list(service requests.Service, asJSON bool, filter hostproto.SessionFilter) outcome {
	listing, err := service.List(filter)
	if err != nil {
		return outcome{err: err}
	}
	if asJSON {
		return outcome{err: json.NewEncoder(e.Stdout).Encode(listing)}
	}
	for _, item := range listing.Items {
		fmt.Fprintf(e.Stdout, "%s %s\n", item.Ticket.TicketID, item.Role)
	}
	return outcome{code: ExitClean}
}

func parseTicket(positionals []string) (hostproto.TicketID, error) {
	if len(positionals) != 1 {
		return hostproto.TicketID{}, errMissingTicket
	}
	ticket, err := hostproto.ParseTicketID(positionals[0])
	if err != nil {
		return hostproto.TicketID{}, refusal{err}
	}
	return ticket, nil
}

func (e Env) show(service requests.Service, positionals []string, capability string) outcome {
	ticket, err := parseTicket(positionals)
	if err != nil {
		return outcome{err: err}
	}
	view, err := service.Show(ticket, capability)
	if err != nil {
		return outcome{err: asRefusal(err)}
	}
	return outcome{err: json.NewEncoder(e.Stdout).Encode(view)}
}

func (e Env) submit(service requests.Service, positionals []string, nonce, capability string) outcome {
	ticket, err := parseTicket(positionals)
	if err != nil {
		return outcome{err: err}
	}
	output, err := io.ReadAll(io.LimitReader(e.Stdin, hostproto.MaxOutputBytes+1))
	if err != nil {
		return outcome{err: fmt.Errorf("read the output: %w", err)}
	}
	receipt, err := service.Submit(requests.Submission{Ticket: ticket, Nonce: nonce, Cap: capability, Output: output, Origin: requests.OriginCLI})
	if err != nil {
		if receipt.CorrectionAllowed {
			fmt.Fprintln(e.Stderr, "the output may be corrected once")
		}
		return outcome{err: asRefusal(err)}
	}
	fmt.Fprintln(e.Stdout, receipt.Text)
	return outcome{code: ExitClean}
}

// asRefusal marks the errors the protocol and the response schema produce as
// refusals: the caller's answer was wrong, the tool did not fail.
func asRefusal(err error) error {
	protocol := []error{
		hostproto.ErrNotFound, hostproto.ErrBadCap, hostproto.ErrCapMissing, hostproto.ErrAgentNotStarted,
		hostproto.ErrTicketMismatch, hostproto.ErrNonceMismatch, hostproto.ErrSchemaDigestMismatch,
		hostproto.ErrNotSubmittable, hostproto.ErrResubmissionConflict, hostproto.ErrCapBoundElsewhere,
		hostproto.ErrIllegalTransition, hostproto.ErrSchemaViolation,
	}
	for _, known := range protocol {
		if errors.Is(err, known) {
			return refusal{err}
		}
	}
	return err
}

// systemOneVerbs are the verbs of `systemone`.
var systemOneVerbs = []string{verbExport, verbImport, verbClassifyPlan}

func (e Env) systemOne(args []string) outcome {
	if len(args) == 0 {
		return outcome{err: refusal{errors.New(cliconv.NeedsVerb(commandSystemOne, systemOneVerbs))}}
	}
	verb, rest := args[0], args[1:]
	switch {
	case cliconv.IsHelpArgument(verb) || verb == cliconv.HelpCommand:
		return outcome{err: cliconv.HelpRequest{Text: cliconv.VerbsUsage(e.Binary, commandSystemOne, systemOneVerbs)}}
	case verb == verbExport:
		return e.systemOneExport(rest)
	case verb == verbImport:
		return e.systemOneImport(rest)
	case verb == verbClassifyPlan:
		return e.systemOneClassifyPlan(rest)
	}
	return outcome{err: refusal{fmt.Errorf("unknown systemone verb %q", verb)}}
}

func (e Env) systemOneExport(args []string) outcome {
	set := e.newFlags(commandSystemOne+" "+verbExport, "")
	session := set.String(flagSession, "", usageSession)
	root := set.String(flagRoot, "", usageRoot)
	rest, err := parseInterleaved(set, args)
	switch {
	case err != nil:
		return outcome{err: err}
	case len(rest) > 0:
		return outcome{err: errUnexpectedParams}
	}
	filter, err := sessionFilterOf(*session)
	if err != nil {
		return outcome{err: err}
	}
	service, err := e.requestService(mustRoot(e, *root))
	if err != nil {
		return outcome{err: err}
	}
	return e.exportClosed(service, filter)
}

func (e Env) systemOneImport(args []string) outcome {
	set := e.newFlags(commandSystemOne+" "+verbImport, argumentTicket)
	nonce := set.String(flagNonce, "", usageNonce)
	source := set.String(flagSource, "", usageSource)
	root := set.String(flagRoot, "", usageRoot)
	positionals, err := parseInterleaved(set, args)
	if err != nil {
		return outcome{err: err}
	}
	service, err := e.requestService(mustRoot(e, *root))
	if err != nil {
		return outcome{err: err}
	}
	return e.importClosed(service, mustRoot(e, *root), positionals, *nonce, *source)
}

func (e Env) exportClosed(service requests.Service, filter hostproto.SessionFilter) outcome {
	listing, err := service.List(filter)
	if err != nil {
		return outcome{err: err}
	}
	return outcome{err: json.NewEncoder(e.Stdout).Encode(hostproto.ExportClosed(listing))}
}

func (e Env) importClosed(service requests.Service, root string, positionals []string, nonce, source string) outcome {
	if strings.HasPrefix(source, classifySourcePrefix) {
		return e.importClassified(root, positionals, nonce, source)
	}
	ticket, err := parseTicket(positionals)
	if err != nil {
		return outcome{err: err}
	}
	if source == "" {
		return outcome{err: refusal{errors.New("--source names the model and endpoint the answers come from")}}
	}
	confidenceSource, err := format.ParseSystemOneSource(source)
	if err != nil {
		return outcome{err: refusal{err}}
	}
	output, err := io.ReadAll(io.LimitReader(e.Stdin, hostproto.MaxOutputBytes+1))
	if err != nil {
		return outcome{err: fmt.Errorf("read the answers: %w", err)}
	}
	view, err := service.Show(ticket, "")
	if err != nil {
		return outcome{err: asRefusal(err)}
	}
	output, err = hostproto.AnswersFromSystemOneFor(view, output)
	if err != nil {
		return outcome{err: refusal{err}}
	}
	profile := requests.RelayProfile(confidenceSource)
	receipt, err := service.Submit(requests.Submission{Ticket: ticket, Nonce: nonce, Output: output, Origin: requests.OriginRelay, Executor: &profile})
	if err != nil {
		return outcome{err: asRefusal(err)}
	}
	fmt.Fprintln(e.Stdout, receipt.Text)
	return outcome{code: ExitClean}
}

func (e Env) systemOneClassifyPlan(args []string) outcome {
	set := e.newFlags(commandSystemOne+" "+verbClassifyPlan, "")
	model := set.String(flagModel, "", usageClassifyModel)
	voters := set.String(flagVoters, "", usageClassifyVoters)
	sessionModel := set.String(flagSessionModel, "", usageClassifySessionModel)
	root := set.String(flagRoot, "", usageRoot)
	rest, err := parseInterleaved(set, args)
	switch {
	case err != nil:
		return outcome{err: err}
	case len(rest) > 0:
		return outcome{err: errUnexpectedParams}
	}
	commands, err := e.classifyCommands(mustRoot(e, *root))
	if err != nil {
		return outcome{err: err}
	}
	return classifyOutcome(commands.Plan(*model, *voters, *sessionModel))
}

// classifyCommands wires the classification verbs over the host store, the
// settings of the project and the admission table.
func (e Env) classifyCommands(root string) (requests.ClassifyCommands, error) {
	service, err := e.requestService(root)
	if err != nil {
		return requests.ClassifyCommands{}, err
	}
	settings, err := hostSettingsOf(root)
	if err != nil {
		return requests.ClassifyCommands{}, refusal{err}
	}
	admission, err := e.classifyAdmission()
	if err != nil {
		return requests.ClassifyCommands{}, err
	}
	return requests.ClassifyCommands{
		Service: service, Kind: format.KindAlignment, Settings: settings, Admission: admission, Stdin: e.Stdin, Stdout: e.Stdout,
	}, nil
}

// classifyAdmission is the admission table the binary answers with: the shipped
// one, unless the environment of a test brings its own.
func (e Env) classifyAdmission() (hostproto.ClassifyAdmission, error) {
	if e.ClassifyAdmission != nil {
		return *e.ClassifyAdmission, nil
	}
	return hostproto.ShippedClassifyAdmission()
}

// classifyOutcome turns the error of a classification command into an outcome:
// a refusal of the protocol is the caller's, anything else a tool failure.
func classifyOutcome(err error) outcome {
	switch {
	case err == nil:
		return outcome{code: ExitClean}
	case requests.IsClassifyRefusal(err):
		return outcome{err: refusal{err}}
	}
	return outcome{err: err}
}

// classifySourcePrefix starts the source of an import made of classifier calls.
const classifySourcePrefix = string(format.SourceHostClassify) + "("

func (e Env) importClassified(root string, positionals []string, nonce, source string) outcome {
	ticket, err := parseTicket(positionals)
	if err != nil {
		return outcome{err: err}
	}
	commands, err := e.classifyCommands(root)
	if err != nil {
		return outcome{err: err}
	}
	return classifyOutcome(commands.Import(ticket.String(), nonce, source))
}

// witnessClassifications checks the classifications that answered a request
// against the raw API log of the user's host, when the project opted into the
// check with `host.classify.witness_dir`.
func (e Env) witnessClassifications(root string) error {
	settings, err := hostSettingsOf(root)
	if err != nil {
		return refusal{err}
	}
	if settings.ClassifyWitnessDir == "" {
		return nil
	}
	service, err := e.requestService(root)
	if err != nil {
		return err
	}
	dir := settings.ClassifyWitnessDir
	_, err = service.WitnessClassifications(hostproto.FileLog{Files: rawlog.Directory{Root: dir}, Root: dir})
	return err
}
