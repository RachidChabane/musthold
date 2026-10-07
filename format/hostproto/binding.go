package hostproto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/RachidChabane/spec-proof/format"
)

// capSeparator separates the agent identifier from the MAC in a capability.
const capSeparator = "."

// Errors of the capability and binding checks.
var (
	// ErrBadCap is returned for a capability that is malformed or whose MAC is wrong.
	ErrBadCap = errors.New("invalid capability")
	// ErrCapMissing is returned when attestation is required and no capability came.
	ErrCapMissing = errors.New("capability missing")
	// ErrAgentNotStarted is returned when the capability's agent is not a started executor of the role.
	ErrAgentNotStarted = errors.New("the agent was not started as an executor of this role")
	// ErrTicketMismatch is returned when the submission names another ticket than the entry.
	ErrTicketMismatch = errors.New("ticket mismatch")
	// ErrNonceMismatch is returned when the nonce is not the one of the issue.
	ErrNonceMismatch = errors.New("nonce mismatch")
	// ErrSchemaDigestMismatch is returned when the stored request's schema changed since emission.
	ErrSchemaDigestMismatch = errors.New("schema digest mismatch")
	// ErrNotSubmittable is returned when the entry is not in a state that takes an answer.
	ErrNotSubmittable = errors.New("the ticket takes no answer in its state")
)

// FormatCap builds the capability of an agent: its identifier and the HMAC-SHA256
// of that identifier under the per-install secret.
func FormatCap(secret []byte, agentID string) (string, error) {
	if agentID == "" || strings.Contains(agentID, capSeparator) {
		return "", fmt.Errorf("%w: agent identifier %q", ErrBadCap, agentID)
	}
	return agentID + capSeparator + hex.EncodeToString(capMAC(secret, agentID)), nil
}

func capMAC(secret []byte, agentID string) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(agentID))
	return mac.Sum(nil)
}

// VerifyCap checks a capability under the secret and returns the agent it names.
// The comparison is constant-time.
func VerifyCap(secret []byte, capability string) (string, error) {
	cut := strings.LastIndex(capability, capSeparator)
	if cut <= 0 {
		return "", fmt.Errorf("%w: malformed", ErrBadCap)
	}
	agentID, presented := capability[:cut], capability[cut+len(capSeparator):]
	expected := hex.EncodeToString(capMAC(secret, agentID))
	if !hmac.Equal([]byte(presented), []byte(expected)) {
		return "", fmt.Errorf("%w: wrong MAC", ErrBadCap)
	}
	return agentID, nil
}

// CapDigest is the digest recorded as the capability's attestation.
func CapDigest(capability string) string { return format.SHA256Hex([]byte(capability)) }

// ExecutorRoster tells which agents the host started as executors of a role. The
// journal of SubagentStart events backs it.
type ExecutorRoster interface {
	Started(agentID, role string) bool
}

// CheckFetch ties a fetch to its entry: the ticket, the state and the capability.
// A fetch carries no nonce; the nonce is what the fetch returns.
func CheckFetch(entry Entry, ticket TicketID, capability string, verifier CapVerifier) (Binding, error) {
	if ticket != entry.Ticket.TicketID {
		return Binding{}, ErrTicketMismatch
	}
	if entry.State != StateEmitted && entry.State != StateFetched {
		return Binding{}, fmt.Errorf("%w: %s", ErrNotSubmittable, entry.State)
	}
	return checkCapBinding(entry, capability, verifier)
}

// CapBindings tells which ticket a capability was first used with, so that one
// capability serves one ticket. The entries of the store back it.
type CapBindings interface {
	TicketOf(capDigest string) (Key, bool)
}

// EntryCapBindings derives the bindings from stored entries: a capability is
// bound to the ticket of the entry that records its digest.
type EntryCapBindings []Entry

// TicketOf implements CapBindings.
func (e EntryCapBindings) TicketOf(capDigest string) (Key, bool) {
	for _, entry := range e {
		if entry.CapDigest != "" && sameSecret(entry.CapDigest, capDigest) {
			return entry.Ticket.Key(), true
		}
	}
	return Key{}, false
}

// CapVerifier bundles what verifying a capability needs. Without Bindings no
// capability is known to be bound to any ticket.
type CapVerifier struct {
	Secret             []byte
	Roster             ExecutorRoster
	Bindings           CapBindings
	RequireAttestation bool
}

// Submission is what an executor presents with an answer.
type Submission struct {
	Ticket TicketID
	Nonce  string
	Cap    string
}

// Binding is the result of a successful binding check.
type Binding struct {
	// AgentID is the agent the capability names, empty when attestation was waived.
	AgentID string
	// CapDigest is the digest of the presented capability, empty when waived.
	CapDigest string
	// Attested is true when a valid capability of a started executor was presented.
	Attested bool
}

// CheckBinding ties a submission to its entry: the ticket, the nonce, the state,
// the schema digest of the stored request and the capability. Any mismatch
// refuses the submission with the matching error. A capability that is present is
// always verified; an absent one is accepted only when the verifier does not
// require attestation.
func CheckBinding(entry Entry, currentSchemaDigest string, submission Submission, verifier CapVerifier) (Binding, error) {
	if submission.Ticket != entry.Ticket.TicketID {
		return Binding{}, ErrTicketMismatch
	}
	if !sameSecret(submission.Nonce, entry.Ticket.Nonce) {
		return Binding{}, ErrNonceMismatch
	}
	if entry.State != StateFetched && entry.State != StateAccepted {
		return Binding{}, fmt.Errorf("%w: %s", ErrNotSubmittable, entry.State)
	}
	if !sameSecret(currentSchemaDigest, entry.SchemaDigest) {
		return Binding{}, ErrSchemaDigestMismatch
	}
	return checkCapBinding(entry, submission.Cap, verifier)
}

func checkCapBinding(entry Entry, capability string, verifier CapVerifier) (Binding, error) {
	if capability == "" {
		if verifier.RequireAttestation {
			return Binding{}, ErrCapMissing
		}
		return Binding{}, nil
	}
	agentID, err := VerifyCap(verifier.Secret, capability)
	if err != nil {
		return Binding{}, err
	}
	if verifier.Roster == nil || !verifier.Roster.Started(agentID, entry.Role) {
		return Binding{}, ErrAgentNotStarted
	}
	digest := CapDigest(capability)
	if entry.CapDigest != "" && !sameSecret(entry.CapDigest, digest) {
		return Binding{}, ErrCapBoundElsewhere
	}
	if verifier.Bindings != nil {
		if bound, found := verifier.Bindings.TicketOf(digest); found && bound != entry.Ticket.Key() {
			return Binding{}, ErrCapBoundElsewhere
		}
	}
	return Binding{AgentID: agentID, CapDigest: digest, Attested: true}, nil
}
