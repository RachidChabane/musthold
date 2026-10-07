package hostproto

// The `request` command of every module, as scripts, relays and fake hosts drive
// it: `request list --json` prints a Listing, and `request submit <ticket> --nonce
// <nonce>` reads the output document from standard input.
const (
	CommandRequest   = "request"
	SubcommandList   = "list"
	SubcommandShow   = "show"
	SubcommandSubmit = "submit"
	FlagJSON         = "--json"
	FlagNonce        = "--nonce"
	FlagCap          = "--cap"
)

// ListArgs returns the arguments of `request list --json`.
func ListArgs() []string { return []string{CommandRequest, SubcommandList, FlagJSON} }

// ShowArgs returns the arguments of `request show <ticket>`.
func ShowArgs(ticket TicketID) []string {
	return []string{CommandRequest, SubcommandShow, ticket.String()}
}

// SubmitArgs returns the arguments of `request submit <ticket> --nonce <nonce>`.
func SubmitArgs(ticket TicketID, nonce string) []string {
	return []string{CommandRequest, SubcommandSubmit, ticket.String(), FlagNonce, nonce}
}

// ListingItem is one pending ticket as `request list --json` prints it: the
// ticket with its nonce, the role that serves it and what its executor sees.
type ListingItem struct {
	Ticket Ticket       `json:"ticket"`
	Role   string       `json:"role"`
	View   ExecutorView `json:"view"`
}

// Listing is the document `request list --json` prints, the contract between a
// module's command line and the scripts, relays and fake hosts that drive it.
type Listing struct {
	Items []ListingItem `json:"items"`
}
