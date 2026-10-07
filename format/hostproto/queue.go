package hostproto

// QueueItem is the queue-only metadata of a request: which criteria it serves,
// in which wave it was first emitted and what for. An executor never sees it.
type QueueItem struct {
	RequestID RequestID `json:"requestId"`
	Criteria  []string  `json:"criteria"`
	Wave      int       `json:"wave"`
	Purpose   string    `json:"purpose"`
}

// Queue is the module-owned queue state next to the host store.
type Queue interface {
	// Record adds the item, or adds its criteria to the item already stored for
	// the request.
	Record(item QueueItem) error
	// Items lists every item ordered by request id.
	Items() ([]QueueItem, error)
	// Wave returns the number of the current wave, zero before the first.
	Wave() (int, error)
	// AdvanceWave starts the next wave and returns its number.
	AdvanceWave() (int, error)
}

// HostStore is the host directory of a module: the shared protocol store, the
// discard of a response a reissued ticket replaces, and the queue.
type HostStore interface {
	Store
	ResponseDiscarder
	Queue
}
