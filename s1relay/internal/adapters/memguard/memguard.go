// Package memguard is the memory rule of the repository for heavy local work, as
// the relay applies it before it wakes a local model: refuse when the system-wide
// free memory percentage reported by `memory_pressure` is below the threshold. It
// mirrors script/mem-guard, whose threshold variable it honours; on a system
// without `memory_pressure` the rule is skipped and says so.
package memguard

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"

	"github.com/RachidChabane/spec-proof/s1relay/internal/app/ports"
)

// Rule constants shared with script/mem-guard.
const (
	// DefaultMinFreePercent is the lowest free memory percentage that allows a batch.
	DefaultMinFreePercent = 40
	// MinFreeVariable overrides the threshold, as in script/mem-guard.
	MinFreeVariable = "MEM_GUARD_MIN_FREE_PCT"

	// OSDarwin is the system whose `memory_pressure` the rule reads.
	OSDarwin = "darwin"

	memoryPressureCommand = "memory_pressure"
	percentBase           = 10
	percentBits           = 32
	maxPercent            = 100
)

var freePattern = regexp.MustCompile(`System-wide memory free percentage:\s*(\d+)%`)

// Reader returns the output of `memory_pressure`.
type Reader func(ctx context.Context) ([]byte, error)

var _ ports.MemoryGuard = Guard{}

// Guard applies the rule.
type Guard struct {
	// OS is the operating system, as runtime.GOOS names it.
	OS             string
	MinFreePercent int
	Read           Reader
}

// New builds the guard of this machine with the given threshold.
func New(minFreePercent int) Guard {
	return Guard{OS: runtime.GOOS, MinFreePercent: minFreePercent, Read: readMemoryPressure}
}

func readMemoryPressure(ctx context.Context) ([]byte, error) {
	return exec.CommandContext(ctx, memoryPressureCommand).Output()
}

// ParseFreePercent reads the free memory percentage from the output of
// `memory_pressure`.
func ParseFreePercent(output []byte) (int, error) {
	match := freePattern.FindSubmatch(output)
	if match == nil {
		return 0, fmt.Errorf("no free memory percentage in the %s output", memoryPressureCommand)
	}
	value, err := strconv.ParseInt(string(match[1]), percentBase, percentBits)
	if err != nil || value > maxPercent {
		return 0, fmt.Errorf("the free memory percentage %q is not a percentage", match[1])
	}
	return int(value), nil
}

// Check applies the rule. On a system other than darwin it is skipped with the
// reason. When the percentage cannot be read it returns an error, which the use
// case treats as a refusal: the rule fails closed.
func (g Guard) Check(ctx context.Context) (ports.MemoryVerdict, error) {
	if g.OS != OSDarwin {
		return ports.MemoryVerdict{Skipped: true, Reason: fmt.Sprintf("%s is not available on %s", memoryPressureCommand, g.OS)}, nil
	}
	output, err := g.Read(ctx)
	if err != nil {
		return ports.MemoryVerdict{}, fmt.Errorf("run %s: %w", memoryPressureCommand, err)
	}
	free, err := ParseFreePercent(output)
	if err != nil {
		return ports.MemoryVerdict{}, err
	}
	if free < g.MinFreePercent {
		return ports.MemoryVerdict{Reason: fmt.Sprintf("%d%% of memory is free, the rule needs %d%%", free, g.MinFreePercent)}, nil
	}
	return ports.MemoryVerdict{Allowed: true, Reason: fmt.Sprintf("%d%% of memory is free", free)}, nil
}
