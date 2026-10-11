package ports

import "context"

// Capacity is what the machine has for a scan's runs: its processors, its
// memory and the share of that memory that is free.
type Capacity struct {
	Processors  int
	TotalMemory uint64
	// FreePercent is the share of the memory that is free, from 0 to 100.
	FreePercent int
}

// CapacityReader reads the capacity of the machine.
type CapacityReader interface {
	Capacity(ctx context.Context) (Capacity, error)
}
