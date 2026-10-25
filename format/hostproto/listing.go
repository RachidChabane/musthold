package hostproto

// The `request` command of every module, as scripts, relays and fake hosts drive
// it: `request list --json --session <session>` prints a Listing of the session's
// tickets, and `request submit <ticket> --nonce <nonce>` reads the output
// document from standard input. Without `--session`, `request list` lists the
// tickets of every session.
const (
	CommandRequest   = "request"
	SubcommandList   = "list"
	SubcommandShow   = "show"
	SubcommandSubmit = "submit"
	FlagJSON         = "--json"
	FlagNonce        = "--nonce"
	FlagCap          = "--cap"
	FlagSession      = "--session"
)

// ProveListCommand is `request list --session prove`, as the skills that drain
// the prove session run it: one `<ticket> <role>` line per ticket of the session.
const ProveListCommand = CommandRequest + " " + SubcommandList + " " + FlagSession + " " + string(SessionProve)

// ListArgs returns the arguments of `request list --json --session <session>`.
func ListArgs(session Session) []string {
	return append([]string{CommandRequest, SubcommandList, FlagJSON}, OnlySession(session).Args()...)
}

// ShowArgs returns the arguments of `request show <ticket>`.
func ShowArgs(ticket TicketID) []string {
	return []string{CommandRequest, SubcommandShow, ticket.String()}
}

// SubmitArgs returns the arguments of `request submit <ticket> --nonce <nonce>`.
func SubmitArgs(ticket TicketID, nonce string) []string {
	return []string{CommandRequest, SubcommandSubmit, ticket.String(), FlagNonce, nonce}
}

// ListingItem is one pending ticket as `request list --json` prints it: the
// ticket with its nonce, the role that serves it, the sessions that want its
// request and what its executor sees.
type ListingItem struct {
	Ticket   Ticket       `json:"ticket"`
	Role     string       `json:"role"`
	Sessions []Session    `json:"sessions"`
	View     ExecutorView `json:"view"`
}

// Listing is the document `request list --json` prints, the contract between a
// module's command line and the scripts, relays and fake hosts that drive it.
type Listing struct {
	Items []ListingItem `json:"items"`
}
