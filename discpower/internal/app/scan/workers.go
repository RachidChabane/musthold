package scan

import (
	"context"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
)

// Bounds of the workers a scan chooses from the capacity of the machine.
const (
	// MaxAutoWorkers is the most workers a scan chooses by itself.
	MaxAutoWorkers = 4
	// ProcessorsPerWorker is how many processors each chosen worker needs: a
	// run of tests often starts a build or an interpreter beside the tests.
	ProcessorsPerWorker = 2
	// MemoryPerWorker is the memory each chosen worker needs.
	MemoryPerWorker = 1 << 30
	// GuardFreePercent is the share of the memory the memory guard keeps free:
	// workers are chosen in the memory above it.
	GuardFreePercent = 40
	// percentBase turns a percentage into a share.
	percentBase = 100
)

// workers is the requested number of workers, or, when none is requested, as
// many as the processors and the memory above the guard's free share allow,
// between one and MaxAutoWorkers; DefaultWorkers when the capacity cannot be
// read.
func (s *Service) workers(ctx context.Context, requested int) int {
	if requested > 0 {
		return requested
	}
	if s.deps.Capacity == nil {
		return DefaultWorkers
	}
	capacity, err := s.deps.Capacity.Capacity(ctx)
	if err != nil {
		return DefaultWorkers
	}
	return AutoWorkers(capacity)
}

// AutoWorkers is how many workers the capacity allows: the processors over
// ProcessorsPerWorker and the memory above the guard's free share over
// MemoryPerWorker, the fewer of the two, between one and MaxAutoWorkers.
func AutoWorkers(capacity ports.Capacity) int {
	headroom := 0
	if capacity.FreePercent > GuardFreePercent {
		headroom = int(capacity.TotalMemory * uint64(capacity.FreePercent-GuardFreePercent) / percentBase / MemoryPerWorker)
	}
	return max(1, min(MaxAutoWorkers, capacity.Processors/ProcessorsPerWorker, headroom))
}
