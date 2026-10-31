package hostproto

import (
	"fmt"
	"strconv"
)

// The `request` command of every module, as scripts, relays, hooks and fake hosts
// drive it: `request list --json --session <session>` prints a Listing of the
// session's tickets, `request submit <ticket> --nonce <nonce>` reads the output
// document from standard input, and `request progress --json --session <session>
// --since <cursor>` prints the Progress of the session. Without `--session`,
// `request list` and `request progress` read every session.
const (
	CommandRequest     = "request"
	SubcommandList     = "list"
	SubcommandShow     = "show"
	SubcommandSubmit   = "submit"
	SubcommandProgress = "progress"
	FlagJSON           = "--json"
	FlagNonce          = "--nonce"
	FlagCap            = "--cap"
	FlagSession        = "--session"
	FlagSince          = "--since"
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

// ProgressArgs returns the arguments of `request progress --json --since
// <cursor>` for the sessions the filter selects.
func ProgressArgs(filter SessionFilter, since int) []string {
	return append(append([]string{CommandRequest, SubcommandProgress, FlagJSON}, filter.Args()...), FlagSince, strconv.Itoa(since))
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
// Items are the launchable tickets; InFlight and Orphaned count the fetched ones
// that wait for an answer, which no executor should be started for.
type Listing struct {
	Items    []ListingItem `json:"items"`
	InFlight int           `json:"inFlight"`
	Orphaned int           `json:"orphaned"`
}

// progressLine is the text form of a Progress, one line.
const progressLine = "cursor %d since %d answered %d launchable %d in_flight %d orphaned %d"

// Line is the text form `request progress` prints without `--json`.
func (p Progress) Line() string {
	return fmt.Sprintf(progressLine, p.Cursor, p.Since, p.Answered, p.Launchable, p.InFlight, p.Orphaned)
}

// Progress is the document `request progress --json` prints: where the journal
// stands, how many answers it accepted since a cursor for the selected sessions,
// and what those sessions wait for. A caller keeps Cursor and passes it as
// `--since` on its next read to learn whether an answer arrived in between.
type Progress struct {
	// Cursor is the sequence number the next journal record will carry.
	Cursor int `json:"cursor"`
	// Since is the cursor the answers are counted from.
	Since int `json:"since"`
	// Answered counts the answers accepted from Since on.
	Answered int `json:"answered"`
	// Launchable, InFlight and Orphaned count the waiting tickets as the listing
	// sorts them.
	Launchable int `json:"launchable"`
	InFlight   int `json:"inFlight"`
	Orphaned   int `json:"orphaned"`
}
