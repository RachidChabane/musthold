package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/responsecheck"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/cliconv"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/requests"
)

// Flags of `request` and `systemone`.
const (
	flagJSON   = "json"
	flagNonce  = "nonce"
	flagCap    = "cap"
	flagSource = "source"
	// Verbs of `systemone`.
	verbExport = "export"
	verbImport = "import"
)

var errMissingTicket = refusal{errors.New("a ticket identifier is required")}

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
	root := set.String(flagRoot, "", usageRoot)
	rest, err := parseInterleaved(set, args)
	switch {
	case err != nil:
		return outcome{err: err}
	case len(rest) > 0:
		return outcome{err: errUnexpectedParams}
	}
	service, err := e.requestService(mustRoot(e, *root))
	if err != nil {
		return outcome{err: err}
	}
	return e.list(service, *asJSON)
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

func (e Env) list(service requests.Service, asJSON bool) outcome {
	listing, err := service.List()
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
var systemOneVerbs = []string{verbExport, verbImport}

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
	}
	return outcome{err: refusal{fmt.Errorf("unknown systemone verb %q", verb)}}
}

func (e Env) systemOneExport(args []string) outcome {
	set := e.newFlags(commandSystemOne+" "+verbExport, "")
	root := set.String(flagRoot, "", usageRoot)
	rest, err := parseInterleaved(set, args)
	switch {
	case err != nil:
		return outcome{err: err}
	case len(rest) > 0:
		return outcome{err: errUnexpectedParams}
	}
	service, err := e.requestService(mustRoot(e, *root))
	if err != nil {
		return outcome{err: err}
	}
	return e.exportClosed(service)
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
	return e.importClosed(service, positionals, *nonce, *source)
}

// closedExport is one closed request as a relay posts it to a System One endpoint.
type closedExport struct {
	Ticket string          `json:"ticket"`
	Nonce  string          `json:"nonce"`
	Closed json.RawMessage `json:"closed"`
}

func (e Env) exportClosed(service requests.Service) outcome {
	listing, err := service.List()
	if err != nil {
		return outcome{err: err}
	}
	exported := []closedExport{}
	for _, item := range listing.Items {
		if len(item.View.Closed) > 0 && item.Ticket.Vote == 0 {
			exported = append(exported, closedExport{Ticket: item.Ticket.TicketID.String(), Nonce: item.Ticket.Nonce, Closed: item.View.Closed})
		}
	}
	return outcome{err: json.NewEncoder(e.Stdout).Encode(exported)}
}

func (e Env) importClosed(service requests.Service, positionals []string, nonce, source string) outcome {
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
	if _, err := service.Show(ticket, ""); err != nil {
		return outcome{err: asRefusal(err)}
	}
	profile := requests.RelayProfile(confidenceSource)
	receipt, err := service.Submit(requests.Submission{Ticket: ticket, Nonce: nonce, Output: output, Origin: requests.OriginRelay, Executor: &profile})
	if err != nil {
		return outcome{err: asRefusal(err)}
	}
	fmt.Fprintln(e.Stdout, receipt.Text)
	return outcome{code: ExitClean}
}
