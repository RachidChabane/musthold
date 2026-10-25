package hostbridge

// noCap is the emission cap of a session that may emit without limit.
const noCap = 0

// budget is what remains of a session's emission cap while a request is settled:
// the emissions the journal records for the session, plus those made since.
type budget struct {
	cap  int
	used int
}

func newBudget(cap, used int) *budget { return &budget{cap: cap, used: used} }

// allows reports whether the session may make one more emission.
func (b *budget) allows() bool { return b.cap == noCap || b.used < b.cap }

// spend counts one emission.
func (b *budget) spend() { b.used++ }
